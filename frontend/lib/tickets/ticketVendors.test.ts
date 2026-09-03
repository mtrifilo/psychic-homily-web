import { describe, it, expect, afterEach } from 'vitest'
import {
  TICKET_VENDORS_BY_DOMAIN,
  affiliatePartnerIds,
  carriesOurAffiliateTag,
  repairTicketUrl,
  resolveTicketVendor,
  ticketLink,
  ticketOffer,
  ticketVendorLabel,
} from './ticketVendors'
import type { AffiliatePartnerIds } from './ticketVendors'

/** The state the site ships in until the affiliate application is approved. */
const NO_PARTNERS: AffiliatePartnerIds = {}
const IMPACT: AffiliatePartnerIds = { impact: '1234567' }

// Every test that sets the variable is freed from restoring it, so no test can
// leak a configured partner ID into the pass-through cases that must not see one.
afterEach(() => {
  delete process.env.NEXT_PUBLIC_IMPACT_PARTNER_ID
})

/**
 * A real domain in the table that carries no affiliate program.
 *
 * Throws rather than degrading: with `?.[0]` this silently became the string
 * "undefined" once every vendor had a program, and the case it exists to cover
 * (a recognized vendor is never tagged) stopped being tested while the suite
 * stayed green.
 */
const unaffiliatedDomain = (() => {
  const found = Object.entries(TICKET_VENDORS_BY_DOMAIN).find(
    ([, vendor]) => !vendor.affiliate
  )?.[0]
  if (!found) {
    throw new Error(
      'Every vendor now has an affiliate entry: pick a new NEVER_TAGGABLE fixture.'
    )
  }
  return found
})()

/**
 * Values that must survive UNTOUCHED whatever is configured: no vendor, no
 * affiliate program, or nothing the builder could rewrite without inventing a
 * scheme. Asserted under both partner-ID states, so "config changes nothing
 * here" is a claim the suite makes rather than two lists that can drift.
 */
const NEVER_TAGGABLE = [
  `https://${unaffiliatedDomain}/event/abc`,
  'https://tix.some-venue.example/e/1',
  'https://ticketweb.com.evil.test/e/2',
  'ticketweb.com/event/2',
  '//ticketweb.com/event/2',
  '/relative/path',
  'not a url',
  '',
]

function expectPassthrough(url: string, partnerIds: AffiliatePartnerIds) {
  expect(ticketLink(url, partnerIds)).toEqual({
    href: url,
    sponsored: false,
    plantedTag: null,
  })
}

describe('resolveTicketVendor', () => {
  it.each([
    ['https://dice.fm/event/abc', 'DICE'],
    ['https://www.eventbrite.com/e/123', 'Eventbrite'],
    ['ticketmaster.com/event/1', 'Ticketmaster'],
    ['https://www.ticketweb.com/event/2', 'TicketWeb'],
    ['https://seetickets.us/event/3', 'See Tickets'],
    ['https://event.etix.com/ticket/4', 'Etix'],
    ['HTTPS://WWW.TICKETWEB.COM/event/5', 'TicketWeb'],
    // A trailing dot is the fully-qualified spelling of the same host, and
    // reaches the real vendor.
    ['https://ticketweb.com./event/6', 'TicketWeb'],
  ])('names the vendor behind %s', (url, expected) => {
    expect(resolveTicketVendor(url)?.name).toBe(expected)
  })

  // Host-anchored, never substring: a lookalike must not borrow a real
  // vendor's name in structured data or its affiliate tag on an outbound link.
  it.each([
    ['a lookalike prefix', 'https://evil-dice.fm/event/abc'],
    ['a lookalike suffix', 'https://dice.fm.evil.test/event/abc'],
    ['the domain in a path', 'https://evil.test/dice.fm/event/abc'],
    ['the domain in a query', 'https://evil.test/e?to=ticketweb.com'],
    ['an unknown vendor', 'https://tix.some-venue.example/e/1'],
    ['an unparseable value', 'not a url'],
    ['an empty value', ''],
    ['whitespace only', '   '],
    ['a nullish value', undefined],
  ])('resolves no vendor for %s', (_label, url) => {
    expect(resolveTicketVendor(url)).toBeUndefined()
  })
})

describe('repairTicketUrl', () => {
  it.each([
    ['https://tix.example/1', 'https://tix.example/1'],
    ['HTTPS://tix.example/1', 'HTTPS://tix.example/1'],
    ['  https://tix.example/1  ', 'https://tix.example/1'],
    ['tix.example/1', 'https://tix.example/1'],
    ['//tix.example/1', 'https://tix.example/1'],
    // A prefix-passing non-scheme would otherwise ship as a relative href.
    ['httpfoo.example/1', 'https://httpfoo.example/1'],
  ])('repairs %s', (input, expected) => {
    expect(repairTicketUrl(input)).toBe(expected)
  })

  it.each<string | null | undefined>(['', '   ', null, undefined])(
    'returns null for %s',
    input => {
      expect(repairTicketUrl(input)).toBeNull()
    }
  )

  // The festival anchor carries its own http(s) floor and must never be handed
  // a script-bearing scheme. The repair defusing these by prefixing is a
  // navigation fix, so this pins the SAFETY property separately.
  it.each([
    'javascript:alert(1)',
    'JaVaScRiPt:alert(1)',
    'data:text/html,<script>alert(1)</script>',
    'vbscript:msgbox(1)',
    '  javascript:alert(1)  ',
  ])('never yields a script-bearing scheme for %s', input => {
    const repaired = repairTicketUrl(input)
    expect(repaired).not.toMatch(/^\s*(javascript|data|vbscript):/i)
    expect(repaired).toMatch(/^https:\/\//i)
  })
})

describe('affiliatePartnerIds', () => {
  it('is empty when the environment carries no partner ID', () => {
    delete process.env.NEXT_PUBLIC_IMPACT_PARTNER_ID
    expect(affiliatePartnerIds()).toEqual({})
  })

  it('is empty for a blank partner ID rather than tagging with one', () => {
    process.env.NEXT_PUBLIC_IMPACT_PARTNER_ID = '   '
    expect(affiliatePartnerIds()).toEqual({})
  })

  it('reads the Impact partner ID from the environment', () => {
    process.env.NEXT_PUBLIC_IMPACT_PARTNER_ID = ' 1234567 '
    expect(affiliatePartnerIds()).toEqual({ impact: '1234567' })
  })
})

describe('ticketLink with no partner IDs configured', () => {
  // The shipping default. Every one of these must come back byte-identical:
  // turning affiliate links on has to be a config flip, so today's rendered
  // href is exactly the stored string.
  it.each([
    ...NEVER_TAGGABLE,
    // The configured vendors, which an ID would tag — the whole point.
    'https://www.ticketweb.com/event/2',
    'https://www.ticketmaster.com/event/1',
    // Shapes URL normalization would silently rewrite if the builder ever
    // round-tripped through `new URL()` on this branch.
    'https://www.ticketweb.com/event/2?utm_source=venue#tickets',
    'https://www.ticketweb.com',
    '   https://www.ticketweb.com/event/2   ',
  ])('passes %s through byte-identically', url => {
    expectPassthrough(url, NO_PARTNERS)
  })

  it('leaves the whole table untagged', () => {
    for (const domain of Object.keys(TICKET_VENDORS_BY_DOMAIN)) {
      expectPassthrough(`https://${domain}/event/1`, NO_PARTNERS)
    }
  })
})

describe('ticketLink with an Impact partner ID configured', () => {
  // DERIVED from the table, not a hand-kept list: a vendor given an affiliate
  // entry gets its tagged shape checked because it was added, not because
  // somebody remembered to add a case. The shape asserted is the contract —
  // the vendor's OWN host (never a network redirect domain, which is
  // Disallow: / to Googlebot) carrying the partner ID as a plain query param.
  const configuredVendors = Object.entries(TICKET_VENDORS_BY_DOMAIN).filter(
    ([, vendor]) => vendor.affiliate
  )

  it('has at least one configured vendor to check', () => {
    expect(configuredVendors.length).toBeGreaterThan(0)
  })

  it.each(configuredVendors)('tags %s on its own host', (domain, vendor) => {
    const { param } = vendor.affiliate!
    expect(ticketLink(`https://www.${domain}/event/1`, IMPACT)).toEqual({
      href: `https://www.${domain}/event/1?${param}=1234567`,
      sponsored: true,
      plantedTag: null,
    })
  })

  it('keeps existing query params and appends the tag', () => {
    expect(ticketLink('https://www.ticketweb.com/e/2?a=1&b=2', IMPACT).href).toBe(
      'https://www.ticketweb.com/e/2?a=1&b=2&irmp=1234567'
    )
  })

  it('keeps the fragment behind the query', () => {
    expect(ticketLink('https://www.ticketweb.com/e/2#seats', IMPACT).href).toBe(
      'https://www.ticketweb.com/e/2?irmp=1234567#seats'
    )
  })

  it('tags a subdomain of a configured vendor', () => {
    expect(ticketLink('https://tickets.ticketweb.com/e/1', IMPACT).href).toBe(
      'https://tickets.ticketweb.com/e/1?irmp=1234567'
    )
  })

  // Ticketmaster is in the table as a seller NAME only: whether it has Impact
  // direct-domain tracking enabled is unverified, so it must not be tagged.
  it('does not tag a recognized vendor that has no affiliate entry', () => {
    const url = 'https://www.ticketmaster.com/event/1'
    expect(ticketLink(url, IMPACT)).toEqual({
      href: url,
      sponsored: false,
      plantedTag: null,
    })
    expect(resolveTicketVendor(url)?.name).toBe('Ticketmaster')
  })

  // href and sponsored are idempotent. `plantedTag` deliberately is NOT part
  // of that claim: it describes the INPUT, and re-application feeds it a value
  // that does carry a tag. Nothing re-applies in practice (the stored value is
  // what gets passed), and a report that fired on our own output would be a
  // false positive, so this pins which fields the invariant covers.
  it('is idempotent under re-application', () => {
    const once = ticketLink('https://www.ticketweb.com/e/2', IMPACT)
    const twice = ticketLink(once.href, IMPACT)
    expect(twice.href).toBe(once.href)
    expect(twice.sponsored).toBe(once.sponsored)
  })

  // The write side percent-encodes the ID; a read side that compared the raw
  // ID against the stored bytes would report our own link as somebody else's
  // and render it unqualified.
  it('is idempotent for a partner ID carrying reserved characters', () => {
    const partners = { impact: 'a b&c' }
    const once = ticketLink('https://www.ticketweb.com/e/2', partners)
    expect(once).toEqual({
      href: 'https://www.ticketweb.com/e/2?irmp=a%20b%26c',
      sponsored: true,
      plantedTag: null,
    })
    const again = ticketLink(once.href, partners)
    expect(again.href).toBe(once.href)
    expect(again.sponsored).toBe(once.sponsored)
  })

  // `show.ticket_url` is open contribution that publishes without review, so a
  // planted tag is a real input. It is neither rewritten (that would redirect
  // their commission to us) nor left unqualified (that would publish an
  // affiliate link on an indexed page with no rel="sponsored") nor left
  // silent. These are the spellings a server really does resolve to `irmp`, so
  // ours is withheld: two ids would compete.
  it.each([
    // `mine` is the last field: our OWN rendered link copied back in is the
    // likeliest source once the config is flipped, and is benign.
    ['our own id', 'https://www.ticketweb.com/e/2?irmp=1234567', true],
    ["another partner's id", 'https://www.ticketweb.com/e/2?irmp=9999999', false],
    ['a percent-encoded key', 'https://www.ticketweb.com/e/2?%69rmp=9999999', false],
    [
      'a tag among other params',
      'https://www.ticketweb.com/e/2?a=1&irmp=9999999&b=2',
      false,
    ],
    // Hidden behind a `;`, which some servers split on. A `&`-only scan would
    // miss it and append ours, delivering two ids.
    [
      'a tag behind a semicolon',
      'https://www.ticketweb.com/e/2?a=1;irmp=9999999',
      false,
    ],
  ])('leaves %s untouched, qualifies and reports it', (_label, url, mine) => {
    expect(ticketLink(url, IMPACT)).toEqual({
      href: url,
      sponsored: true,
      plantedTag: {
        param: 'irmp',
        host: 'www.ticketweb.com',
        matchesConfiguredPartner: mine,
      },
    })
  })

  // Query keys are case-sensitive and are not trimmed by any mainstream
  // parser, so these reach the advertiser as `IRMP`, `irmp ` and ` irmp` and
  // credit NOBODY. Treating them as a competing tag would hand a contributor a
  // one-character lever to suppress ours forever.
  it.each([
    ['an uppercase key', 'https://www.ticketweb.com/e/2?IRMP=9999999'],
    ['a space-padded key', 'https://www.ticketweb.com/e/2?irmp%20=9999999'],
    ['a plus-padded key', 'https://www.ticketweb.com/e/2?+irmp=9999999'],
  ])('still tags past %s, which credits nobody', (_label, url) => {
    const result = ticketLink(url, IMPACT)
    expect(result.href).toBe(`${url}&irmp=1234567`)
    expect(result.sponsored).toBe(true)
  })

  // Qualification is about the LINK, so it is scoped neither to this build's
  // configuration nor to vendors we have onboarded. A planted tag on a vendor
  // with no affiliate entry, or on a host not in the table at all, is still a
  // monetized link on an indexed page.
  it.each([
    [
      'no partner ID configured',
      'https://www.ticketweb.com/e/2?irmp=1234567',
      NO_PARTNERS,
      'www.ticketweb.com',
      // No configured ID means we have no tag of our own for this to be.
      false,
    ],
    [
      'a vendor with no affiliate entry',
      'https://www.ticketmaster.com/e/1?irmp=9999999',
      IMPACT,
      'www.ticketmaster.com',
      false,
    ],
    [
      'an unaffiliated table vendor',
      'https://dice.fm/e/1?irmp=9999999',
      IMPACT,
      'dice.fm',
      false,
    ],
    [
      'a host not in the table',
      'https://tix.some-venue.example/e/1?irmp=9999999',
      IMPACT,
      'tix.some-venue.example',
      false,
    ],
  ])('qualifies and reports a planted tag with %s', (_l, url, partners, host, mine) => {
    expect(ticketLink(url, partners)).toEqual({
      href: url,
      sponsored: true,
      plantedTag: { param: 'irmp', host, matchesConfiguredPartner: mine },
    })
  })

  // A valueless tag hidden inside a `;`-bearing segment can be neither removed
  // (removal claims whole `&`-segments, so it would take `x=1` with it) nor
  // appended beside (a `;`-splitting server — the only kind the `;` handling
  // exists for — would read `irmp=""` first and credit nobody while the page
  // claimed a paid placement). Left exactly as stored.
  it.each([
    'https://www.ticketweb.com/e/2?irmp=;x=1',
    'https://www.ticketweb.com/e/2?a=1;irmp=',
  ])('leaves %s untouched rather than appending beside a hidden tag', url => {
    expectPassthrough(url, IMPACT)
  })

  // Detection must not depend on every caller having repaired first: the
  // module's own docblock tells callers they need not, and the next surface
  // that renders a stored ticket_url would otherwise publish an unqualified
  // affiliate link with no warning.
  it.each([
    ['a scheme-less value', 'ticketweb.com/e/2?irmp=9999999', 'ticketweb.com'],
    [
      'a protocol-relative value',
      '//www.ticketweb.com/e/2?irmp=9999999',
      'www.ticketweb.com',
    ],
    [
      'a whitespace-padded value',
      '  https://www.ticketweb.com/e/2?irmp=9999999  ',
      'www.ticketweb.com',
    ],
  ])('qualifies and reports a planted tag on %s', (_label, url, host) => {
    expect(ticketLink(url, IMPACT)).toEqual({
      href: url,
      sponsored: true,
      plantedTag: { param: 'irmp', host, matchesConfiguredPartner: false },
    })
  })

  // A browser and `new URL` both strip these before the request goes out, so
  // the vendor reads `irmp`. A textual scan that kept them would call it an
  // unknown parameter and append ours beside it.
  it.each([
    'https://www.ticketweb.com/e/2?ir\tmp=9999999',
    'https://www.ticketweb.com/e/2?ir\nmp=9999999',
    'https://www.ticketweb.com/e/2?ir\rmp=9999999',
  ])('sees through a control character in the key of %j', url => {
    const result = ticketLink(url, IMPACT)
    expect(result.href).toBe(url)
    expect(result.sponsored).toBe(true)
    expect(result.plantedTag?.param).toBe('irmp')
  })

  // The reported host is spelled the way the classifier spells it, so one
  // vendor cannot split into two identities that each evade a host-scoped
  // alert and take their own dedupe slot.
  it('reports a trailing-dot host under its normalized spelling', () => {
    expect(
      ticketLink('https://www.ticketweb.com./e/2?irmp=9999999', IMPACT).plantedTag
    ).toEqual({
      param: 'irmp',
      host: 'www.ticketweb.com',
      matchesConfiguredPartner: false,
    })
  })

  // A valueless occurrence credits nobody: it is a truncated paste, and
  // treating it as a competing partner would forfeit the tag forever.
  it.each([
    ['https://www.ticketweb.com/e/2?irmp', 'https://www.ticketweb.com/e/2?irmp=1234567'],
    ['https://www.ticketweb.com/e/2?irmp=', 'https://www.ticketweb.com/e/2?irmp=1234567'],
    [
      'https://www.ticketweb.com/e/2?a=1&irmp=',
      'https://www.ticketweb.com/e/2?a=1&irmp=1234567',
    ],
  ])('replaces the valueless tag in %s', (input, expected) => {
    expect(ticketLink(input, IMPACT)).toEqual({
      href: expected,
      sponsored: true,
      plantedTag: null,
    })
  })

  // THE contract of the tagging branch: the stored query survives byte for
  // byte and one parameter is appended. Round-tripping through URLSearchParams
  // silently form-encodes all of these, which would break the destination on
  // the very day the config is flipped.
  it.each([
    // A vendor redirect target whose own query would collapse into one value.
    [
      'https://www.ticketweb.com/e/2?next=/a/b?c=d',
      'https://www.ticketweb.com/e/2?next=/a/b?c=d&irmp=1234567',
    ],
    // Base64 padding and reserved characters inside a signature.
    [
      'https://www.ticketweb.com/e/2?sig=aGVsbG8+d29ybGQ/PQ==',
      'https://www.ticketweb.com/e/2?sig=aGVsbG8+d29ybGQ/PQ==&irmp=1234567',
    ],
    // A percent-encoded space, which form encoding rewrites to "+".
    [
      'https://www.ticketweb.com/e/2?q=a%20b',
      'https://www.ticketweb.com/e/2?q=a%20b&irmp=1234567',
    ],
    // A valueless flag that is NOT our param keeps its spelling.
    [
      'https://www.ticketweb.com/e/2?flag&x=1',
      'https://www.ticketweb.com/e/2?flag&x=1&irmp=1234567',
    ],
    // Semicolon separators, which some servers split on themselves.
    [
      'https://www.ticketweb.com/e/2?a=1;b=2',
      'https://www.ticketweb.com/e/2?a=1;b=2&irmp=1234567',
    ],
    // An empty segment from a doubled ampersand.
    [
      'https://www.ticketweb.com/e/2?a=1&&b=2',
      'https://www.ticketweb.com/e/2?a=1&&b=2&irmp=1234567',
    ],
    // Uppercase host and scheme: absolute already, so not ours to restyle.
    [
      'HTTPS://WWW.TICKETWEB.COM/e/2',
      'HTTPS://WWW.TICKETWEB.COM/e/2?irmp=1234567',
    ],
    // A bare "?" keeps the path intact.
    ['https://www.ticketweb.com/e/2?', 'https://www.ticketweb.com/e/2?irmp=1234567'],
  ])('appends to %s without re-encoding the stored query', (input, expected) => {
    expect(ticketLink(input, IMPACT).href).toBe(expected)
  })

  it('encodes a partner ID that carries reserved characters', () => {
    expect(ticketLink('https://www.ticketweb.com/e/2', { impact: 'a b&c' }).href).toBe(
      'https://www.ticketweb.com/e/2?irmp=a%20b%26c'
    )
  })

  // The one difference between the branches: pass-through is byte-identical
  // including padding, tagging trims. Pinned so it stays a stated property
  // rather than an accident.
  it('trims surrounding whitespace when it tags', () => {
    expect(ticketLink('  https://www.ticketweb.com/e/2  ', IMPACT).href).toBe(
      'https://www.ticketweb.com/e/2?irmp=1234567'
    )
  })

  // The same list the no-config suite walks: an unknown vendor, a lookalike
  // host, and anything the builder would have to invent a scheme for stay
  // untouched whether or not a partner ID exists.
  it.each(NEVER_TAGGABLE)('passes %s through untagged', url => {
    expectPassthrough(url, IMPACT)
  })

  it('reads the ambient environment when no partner IDs are passed', () => {
    process.env.NEXT_PUBLIC_IMPACT_PARTNER_ID = '1234567'
    expect(ticketLink('https://www.ticketweb.com/e/2').href).toBe(
      'https://www.ticketweb.com/e/2?irmp=1234567'
    )
  })

  it('never throws on any input', () => {
    const hostile = [
      'javascript:alert(1)',
      'https://',
      'http://[',
      '%%%',
      'https://www.ticketweb.com/e/2?irmp',
    ]
    for (const url of hostile) {
      expect(() => ticketLink(url, IMPACT)).not.toThrow()
    }
  })
})

describe('plantedTag.matchesConfiguredPartner', () => {
  // A partner ID rides in public URLs, so anyone can append ours to any host.
  // The flag drives an operator filter, so a bare value match would hide the
  // reports worth reading behind the benign one.
  it('is false when our id sits on a host we do not tag', () => {
    for (const raw of [
      'https://evil.example/x?irmp=1234567',
      'https://tix.example/e/1?irmp=1234567',
      // A vendor in the table, but with no affiliate entry of its own.
      'https://www.eventbrite.com/e/1?irmp=1234567',
    ]) {
      expect(ticketLink(raw, IMPACT).plantedTag).toMatchObject({
        matchesConfiguredPartner: false,
      })
    }
  })

  it('is true for our own tagged link copied back in', () => {
    expect(
      ticketLink('https://www.ticketweb.com/event/2?irmp=1234567', IMPACT)
        .plantedTag
    ).toMatchObject({ matchesConfiguredPartner: true })
  })

  it('is false for somebody else on a vendor we do tag', () => {
    expect(
      ticketLink('https://www.ticketweb.com/event/2?irmp=9999999', IMPACT)
        .plantedTag
    ).toMatchObject({ matchesConfiguredPartner: false })
  })
})

describe('ticketVendorLabel', () => {
  it('prints the written-down name for a known vendor', () => {
    expect(ticketVendorLabel('https://www.ticketweb.com/event/1')).toBe(
      'TicketWeb'
    )
    expect(ticketVendorLabel('https://dice.fm/event/1')).toBe('DICE')
  })

  // A host we have not written a name for still has to be nameable on a
  // surface that no longer links to it, and its own hostname is the only fact
  // available. `www.` is not part of how a reader names a site.
  it('falls back to the hostname for an unknown vendor', () => {
    expect(ticketVendorLabel('https://www.tix.example/1')).toBe('tix.example')
    expect(ticketVendorLabel('https://box-office.venue.example/e/2')).toBe(
      'box-office.venue.example'
    )
  })

  it('resolves a scheme-less value, matching classification', () => {
    expect(ticketVendorLabel('ticketweb.com/e/1')).toBe('TicketWeb')
    expect(ticketVendorLabel('tix.example/1')).toBe('tix.example')
  })

  it('is null when there is no host to name', () => {
    expect(ticketVendorLabel(null)).toBeNull()
    expect(ticketVendorLabel('   ')).toBeNull()
    expect(ticketVendorLabel('http://')).toBeNull()
  })
})

describe('carriesOurAffiliateTag', () => {
  const TICKETWEB = 'https://www.ticketweb.com/event/2'

  it('is false for every link on a build with no partner ID', () => {
    expect(carriesOurAffiliateTag(ticketLink(TICKETWEB, NO_PARTNERS))).toBe(
      false
    )
    expect(
      carriesOurAffiliateTag(ticketLink('https://tix.example/1', NO_PARTNERS))
    ).toBe(false)
  })

  it('is true only for a link this build tagged itself', () => {
    expect(carriesOurAffiliateTag(ticketLink(TICKETWEB, IMPACT))).toBe(true)
    // A vendor outside every program can never be tagged, however the build is
    // configured.
    expect(
      carriesOurAffiliateTag(ticketLink('https://tix.example/1', IMPACT))
    ).toBe(false)
  })

  // The whole point of the separate derivation: a planted tag makes the link
  // SPONSORED (it is a paid link, and Google's policy is about the link) while
  // crediting somebody else, so it is not one we are paid for.
  it('is false for a sponsored link carrying somebody else\'s tag', () => {
    const planted = ticketLink(`${TICKETWEB}?irmp=9999999`, IMPACT)
    expect(planted.sponsored).toBe(true)
    expect(carriesOurAffiliateTag(planted)).toBe(false)
  })
})

describe('ticketOffer', () => {
  const TICKETWEB = 'https://www.ticketweb.com/event/2'
  const UNKNOWN = 'https://box-office.example/e/1'
  const PLANTED = `${TICKETWEB}?irmp=9999999`

  // THE paid-referral rule. Read with no partner ID configured, which is the
  // state the site ships in.
  it('names the vendor and withholds the link when nobody pays us', () => {
    const known = ticketOffer(TICKETWEB)
    expect(known.linked).toBe(false)
    expect(known.vendorName).toBe('TicketWeb')

    const unknown = ticketOffer(UNKNOWN)
    expect(unknown.linked).toBe(false)
    expect(unknown.vendorName).toBe('box-office.example')
  })

  it('links a vendor this build tagged, and only that vendor', () => {
    const known = ticketOffer(TICKETWEB, { partnerIds: IMPACT })
    expect(known.linked).toBe(true)
    if (!known.linked) throw new Error('unreachable')
    expect(known.href).toBe(`${TICKETWEB}?irmp=1234567`)
    expect(known.sponsored).toBe(true)
    expect(known.ugc).toBe(false)
    expect(ticketOffer(UNKNOWN, { partnerIds: IMPACT }).linked).toBe(false)
  })

  // A tag somebody else planted makes the link sponsored without making it
  // ours, so it is not a click we are paid for.
  it('withholds the link for a planted tag on a configured vendor', () => {
    const planted = ticketOffer(PLANTED, { partnerIds: IMPACT })
    expect(planted.linked).toBe(false)
    expect(planted.plantedTag).not.toBeNull()
    expect(planted.vendorName).toBe('TicketWeb')
  })

  // The exemption is an INPUT because only the caller knows whether admission
  // is free; festivals record no price and never pass it.
  it('links any vendor when the caller says admission is free', () => {
    const offer = ticketOffer(UNKNOWN, { freeAdmission: true })
    expect(offer.linked).toBe(true)
    if (!offer.linked) throw new Error('unreachable')
    expect(offer.href).toBe(UNKNOWN)
    expect(offer.sponsored).toBe(false)
    // Contributor-chosen destination that earns the site nothing.
    expect(offer.ugc).toBe(true)
  })

  // THE REGRESSION THIS PAIR EXISTS FOR. A zero price and a ticket URL are
  // both contributor-writable on the same unreviewed form, so an exemption
  // that ignored the planted tag would be a two-field switch for publishing
  // any stranger's affiliate link as a live anchor.
  it('refuses the free-admission exemption for a planted tag', () => {
    expect(ticketOffer(PLANTED, { freeAdmission: true }).linked).toBe(false)
    expect(
      ticketOffer(PLANTED, { freeAdmission: true, partnerIds: IMPACT }).linked
    ).toBe(false)
  })

  // Our OWN tag still wins on a free show: the click is paid for, so it is
  // qualified as sponsored rather than as user-generated.
  it('prefers our own tag over the exemption on a free show', () => {
    const offer = ticketOffer(TICKETWEB, {
      freeAdmission: true,
      partnerIds: IMPACT,
    })
    expect(offer.linked).toBe(true)
    if (!offer.linked) throw new Error('unreachable')
    expect(offer.href).toBe(`${TICKETWEB}?irmp=1234567`)
    expect(offer.sponsored).toBe(true)
    expect(offer.ugc).toBe(false)
  })

  // The report is about the STORED value, so it survives the refusal to link.
  it('carries the planted tag even when the anchor is withheld', () => {
    const offer = ticketOffer(PLANTED)
    expect(offer.linked).toBe(false)
    expect(offer.plantedTag).toEqual({
      param: 'irmp',
      host: 'www.ticketweb.com',
      matchesConfiguredPartner: false,
    })
  })

  // AN IMPACT PARTNER ID IS PUBLIC: it rides in every tagged href the site
  // renders and is inlined into the browser bundle. Accepting a stored tag
  // that merely carries it would let any URL buy a `rel="sponsored"` anchor to
  // an arbitrary host that pays us nothing.
  it('refuses a stored tag carrying our own public partner id', () => {
    for (const raw of [
      'https://evil.example/anything?irmp=1234567',
      'https://tix.example/x?irmp=1234567',
      // Repeated parameter: which value the vendor reads is a guess, so the
      // presence of ours in one segment settles nothing.
      `${TICKETWEB}?irmp=1234567&irmp=9999999`,
    ]) {
      expect(ticketOffer(raw, { partnerIds: IMPACT }).linked).toBe(false)
    }
  })

  // The href a render site receives is the value the floor tested.
  it('links a trimmed href', () => {
    const offer = ticketOffer('   https://box-office.example/e/1  ', {
      freeAdmission: true,
    })
    expect(offer.linked).toBe(true)
    if (!offer.linked) throw new Error('unreachable')
    expect(offer.href).toBe('https://box-office.example/e/1')
  })

  // A value naming no host has nothing to link and nothing to name.
  it('has neither a link nor a name for a value that names no host', () => {
    for (const raw of ['https://', 'https:///', 'https://javascript:alert(1)']) {
      const offer = ticketOffer(raw, { freeAdmission: true })
      expect(offer.linked).toBe(false)
      expect(offer.vendorName).toBeNull()
    }
  })
})
