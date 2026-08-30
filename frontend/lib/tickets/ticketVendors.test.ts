import { describe, it, expect, afterEach } from 'vitest'
import {
  TICKET_VENDORS_BY_DOMAIN,
  affiliatePartnerIds,
  resolveTicketVendor,
  ticketLink,
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
 * Values that must survive UNTOUCHED whatever is configured: no vendor, no
 * affiliate program, or nothing the builder could rewrite without inventing a
 * scheme. Asserted under both partner-ID states, so "config changes nothing
 * here" is a claim the suite makes rather than two lists that can drift.
 */
const NEVER_TAGGABLE = [
  'https://dice.fm/event/abc',
  'https://tix.some-venue.example/e/1',
  'https://ticketweb.com.evil.test/e/2',
  'ticketweb.com/event/2',
  '//ticketweb.com/event/2',
  '/relative/path',
  'not a url',
  '',
]

function expectPassthrough(url: string, partnerIds: AffiliatePartnerIds) {
  expect(ticketLink(url, partnerIds)).toEqual({ href: url, sponsored: false })
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
  // One case per configured vendor: the tagged URL keeps the vendor's own host
  // (never a network redirect domain, which is Disallow: / to Googlebot) and
  // carries the partner ID as a plain query parameter.
  it.each([
    [
      'https://www.ticketweb.com/event/2',
      'https://www.ticketweb.com/event/2?irmp=1234567',
    ],
    [
      'https://www.ticketmaster.com/event/1',
      'https://www.ticketmaster.com/event/1?irmp=1234567',
    ],
  ])('tags %s', (input, expected) => {
    expect(ticketLink(input, IMPACT)).toEqual({ href: expected, sponsored: true })
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
    expect(ticketLink('https://tickets.ticketmaster.com/e/1', IMPACT).href).toBe(
      'https://tickets.ticketmaster.com/e/1?irmp=1234567'
    )
  })

  it('is idempotent under re-application', () => {
    const once = ticketLink('https://www.ticketweb.com/e/2', IMPACT)
    const twice = ticketLink(once.href, IMPACT)
    expect(twice).toEqual(once)
  })

  // A contributor-submitted URL may already credit somebody else. Overwriting
  // their ID would silently redirect their commission to us.
  it('never overwrites another partner ID, and does not claim it as sponsored', () => {
    const url = 'https://www.ticketweb.com/e/2?irmp=9999999'
    expect(ticketLink(url, IMPACT)).toEqual({ href: url, sponsored: false })
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
