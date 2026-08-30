/**
 * The one place this app knows anything about ticket vendors: who they are
 * (the name structured data is willing to print) and how an outbound link to
 * them is tagged once the site is an affiliate.
 *
 * Two surfaces read it and must not drift: the visible Buy Tickets link on the
 * show and festival pages, and the `seller` name in the show's `MusicEvent`
 * JSON-LD, which names the company without linking to it.
 *
 * The one owner of how a vendor URL is TAGGED, not of vendor URLs generally.
 * Repair still lives at the call site that needs it: `ticketHref` in
 * `features/shows/components/showTicketLine` trims the stored value and
 * supplies a missing scheme before this module ever sees it.
 */

/**
 * An affiliate network whose partner ID tags a vendor's OWN domain.
 *
 * Direct-domain tracking only. Every network's redirect domain
 * (`ticketmaster.evyy.net` and `imp.pxf.io` for Impact, CJ's `anrdoezrs.net`,
 * Rakuten's `click.linksynergy.com`, Skimlinks) answers Googlebot with
 * `Disallow: /`, so a link routed through one is uncrawlable and could never
 * be reused as a structured-data URL.
 *
 * The guarantee here is narrow, and worth stating narrowly: {@link ticketLink}
 * only ever appends a query parameter to the URL a contributor already stored,
 * so IT cannot introduce a redirect domain. It does not make one unreachable —
 * a contributor who stores `ticketmaster.evyy.net/c/123` gets it rendered
 * as-is, matching no vendor and carrying no tag. Nor does it police the PATH:
 * a stored Ticketmaster `/goto/` link (robots-disallowed, and what their share
 * UI hands out) would be tagged like any other. None of that is load-bearing
 * while the JSON-LD emits no offer URL; all of it becomes a gate to build if
 * an affiliate-tagged `offers.url` is ever reconsidered.
 *
 * The choice of PARAMETER is per-vendor and has to be verified against that
 * vendor's robots.txt before the vendor is given an {@link VendorAffiliate}
 * entry, because a vendor can disallow the very shape its tracking produces:
 * Ticketmaster disallows `/goto*` and AXS disallows `/*referrer=*` (any URL
 * carrying that param). Impact's `irmp` on the advertiser's own domain avoids
 * both.
 */
export type AffiliateNetwork = 'impact'

/** How one vendor's own domain carries our partner ID. */
interface VendorAffiliate {
  network: AffiliateNetwork
  /** Query parameter name, set on the vendor's own host. */
  param: string
}

export interface TicketVendor {
  /**
   * The vendor's real name. `seller.name` is a claim about a real company, so
   * this is written down rather than prettified out of a hostname, which would
   * turn `tix.some-venue.example` into an invented "Tix".
   */
  name: string
  /**
   * Present only for vendors inside an affiliate program the site has applied
   * to. Absent means outbound links to this vendor stay untagged forever, no
   * matter what partner IDs are configured.
   */
  affiliate?: VendorAffiliate
}

/**
 * Impact's direct-domain tracking: the partner ID rides on the advertiser's
 * own host as `?irmp=`. Shared by every vendor inside one Impact program, so
 * correcting the parameter is one edit rather than one per vendor.
 */
const IMPACT_DIRECT_TRACKING: VendorAffiliate = {
  network: 'impact',
  param: 'irmp',
}

/**
 * Ticket vendors we recognize, keyed by registrable domain. THE table: adding
 * a vendor here is the whole edit, and the tagged-shape test derives its cases
 * from this object rather than repeating it.
 *
 * `ticketmaster.com` and `ticketweb.com` sit inside one Ticketmaster/Impact
 * program; the same approval also covers Front Gate, Universe, Veeps and
 * Moshtix, which are absent here because naming a vendor is a structured-data
 * claim of its own and none of those has been added as a seller yet.
 */
export const TICKET_VENDORS_BY_DOMAIN: Record<string, TicketVendor> = {
  'dice.fm': { name: 'DICE' },
  'eventbrite.com': { name: 'Eventbrite' },
  'ticketmaster.com': {
    name: 'Ticketmaster',
    affiliate: IMPACT_DIRECT_TRACKING,
  },
  'ticketweb.com': { name: 'TicketWeb', affiliate: IMPACT_DIRECT_TRACKING },
  'seetickets.us': { name: 'See Tickets' },
  'etix.com': { name: 'Etix' },
}

/**
 * An absolute http(s) URL: one spelling of the test, shared by the two
 * questions that ask it. Classification tolerates a scheme-less value by
 * supplying a scheme; rewriting refuses one, because it would have to invent
 * the scheme it then renders. Those answers differ, so the RULE they differ
 * about must not.
 */
const ABSOLUTE_HTTP_URL = /^https?:\/\//i

/**
 * The vendor behind a stored ticket URL, or `undefined` when it is not one we
 * recognize.
 *
 * Host-anchored (`host === domain || host.endsWith('.' + domain)`) so
 * `evil-dice.fm` and `dice.fm.evil.test` cannot borrow a real vendor's name or
 * collect a real vendor's affiliate tag. A substring test would hand both.
 *
 * Scheme-less values are classified too: submitters type bare hosts, and
 * "ticketweb.com/e/1" is as much a TicketWeb URL as its https form. That
 * tolerance is for CLASSIFICATION only — {@link ticketLink} refuses to rewrite
 * a value whose scheme it would have to invent.
 */
export function resolveTicketVendor(
  rawUrl: string | undefined | null
): TicketVendor | undefined {
  const raw = rawUrl?.trim()
  if (!raw) return undefined

  const candidate = ABSOLUTE_HTTP_URL.test(raw)
    ? raw
    : `https://${raw.replace(/^\/+/, '')}`
  let host: string
  try {
    host = new URL(candidate).hostname.toLowerCase()
  } catch {
    return undefined
  }

  const domain = Object.keys(TICKET_VENDORS_BY_DOMAIN).find(
    d => host === d || host.endsWith(`.${d}`)
  )
  return domain ? TICKET_VENDORS_BY_DOMAIN[domain] : undefined
}

/**
 * A stored ticket URL repaired into something navigable, or null when the
 * field holds nothing.
 *
 * Stored ticket URLs are contributor paste. The backend persists the column
 * untrimmed and the ingest paths skip the handler's validator, so whitespace
 * padding, scheme-less hosts ("tix.example/1") and protocol-relative values
 * all arrive intact. Left alone, a scheme-less value renders as a RELATIVE
 * href that navigates inside this site instead of out to the vendor.
 *
 * The scheme test is anchored and case-insensitive rather than a bare prefix
 * check: `startsWith('http')` passed "httpfoo.example" through as exactly that
 * relative href. Vendors do print uppercase schemes, and those are left as
 * they are — already absolute, and not ours to restyle.
 */
export function repairTicketUrl(
  rawUrl: string | undefined | null
): string | null {
  const raw = rawUrl?.trim()
  if (!raw) return null
  if (ABSOLUTE_HTTP_URL.test(raw)) return raw
  if (raw.startsWith('//')) return `https:${raw}`
  return `https://${raw}`
}

/** Partner IDs the deployment holds, keyed by network. */
export type AffiliatePartnerIds = Partial<Record<AffiliateNetwork, string>>

/**
 * The partner IDs this deployment is configured with. Empty by default, which
 * is what makes every ticket link a byte-identical pass-through until the
 * affiliate application is approved and the environment carries an ID.
 *
 * An Impact partner ID rides in public URLs and is not a secret, so it is a
 * `NEXT_PUBLIC_` value; it is read here as a literal property access, because
 * a dynamic `process.env[name]` lookup is not inlined into the browser bundle
 * and would read as configured on the server and empty in the client.
 *
 * `NEXT_PUBLIC_` values inline at BUILD time, so the flip is a redeploy, not
 * an env edit on a running deployment: setting the variable without rebuilding
 * leaves the shipped bundle carrying the old (empty) value. Tests mutate
 * `process.env` at runtime, which vitest allows and a browser bundle does not,
 * so they pin this function's LOGIC and not the mechanism production uses.
 */
export function affiliatePartnerIds(): AffiliatePartnerIds {
  const impact = process.env.NEXT_PUBLIC_IMPACT_PARTNER_ID?.trim()
  return impact ? { impact } : {}
}

export interface TicketLink {
  /** The href to render. */
  href: string
  /**
   * We attached (or already own) the affiliate tag on this URL, so the visible
   * anchor must carry `rel="sponsored"` — Google's link-spam policy requires
   * paid links to be qualified.
   */
  sponsored: boolean
}

/**
 * One `&`-separated segment of a query string.
 *
 * `raw` is the segment exactly as stored and is what gets written back, so a
 * segment this module does not remove survives byte-for-byte — including the
 * spellings a split-and-rejoin would quietly normalize (`flag` vs `flag=`, an
 * empty segment from a doubled `&`).
 */
interface QueryPair {
  raw: string
  key: string
  value: string
}

/**
 * A URL split into the three parts tagging cares about, WITHOUT decoding
 * anything.
 *
 * The parse is textual on purpose. Round-tripping through `URL`/
 * `URLSearchParams` re-serializes the whole query as form-encoded data: it
 * turns `?q=a%20b` into `?q=a+b`, `?flag` into `?flag=`, and it percent-encodes
 * the `/`, `?`, `:` and `=` inside values that vendors really do carry —
 * `?next=/a/b?c=d` collapses into a single opaque value, and a base64 signature
 * loses its padding. Those are the contributor's bytes, and the destination is
 * the vendor's to interpret. Appending our parameter must not rewrite them.
 */
function splitUrlForTagging(url: string): {
  base: string
  pairs: QueryPair[]
  fragment: string
} {
  const hashAt = url.indexOf('#')
  const fragment = hashAt === -1 ? '' : url.slice(hashAt)
  const beforeFragment = hashAt === -1 ? url : url.slice(0, hashAt)

  const queryAt = beforeFragment.indexOf('?')
  if (queryAt === -1) {
    return { base: beforeFragment, pairs: [], fragment }
  }

  const query = beforeFragment.slice(queryAt + 1)
  const base = beforeFragment.slice(0, queryAt)
  if (query === '') return { base, pairs: [], fragment }

  const pairs = query.split('&').map(raw => {
    const eq = raw.indexOf('=')
    return eq === -1
      ? { raw, key: raw, value: '' }
      : { raw, key: raw.slice(0, eq), value: raw.slice(eq + 1) }
  })
  return { base, pairs, fragment }
}

/**
 * A stored ticket URL turned into the href the page actually renders.
 *
 * With no partner ID configured for the vendor's network, or for a vendor
 * outside an affiliate program, or for a value that is not an absolute http(s)
 * URL, the input string is returned BYTE-IDENTICAL. That is the contract that
 * makes turning affiliate links on a config flip: nothing about the markup or
 * the call sites changes when the environment starts carrying an ID.
 *
 * When it DOES tag, the result is the stored string plus one appended
 * parameter, ahead of any fragment. Every other byte of the query survives
 * untouched — see {@link splitUrlForTagging} for why that has to be done
 * textually.
 *
 * Never throws. Ticket URLs are contributor-entered paste, so junk, relative
 * and protocol-relative values all take the pass-through branch.
 *
 * A vendor URL that ALREADY credits somebody through this parameter is left
 * exactly as stored, whoever that is: overwriting a tracking ID inside a URL a
 * contributor submitted would silently redirect their commission to us. The
 * key is matched case-insensitively, because `?IRMP=` is another party's
 * credit just as much as `?irmp=` is, and appending ours beside it would both
 * hijack and double-count. It counts as sponsored when our own ID is among
 * those present, which makes this idempotent under re-application.
 *
 * A VALUELESS occurrence (`?irmp` or `?irmp=`) credits nobody — it is a
 * truncated paste, not a competing partner — so it is dropped in favour of a
 * real tag rather than blocking one forever.
 */
export function ticketLink(
  rawUrl: string,
  partnerIds: AffiliatePartnerIds = affiliatePartnerIds()
): TicketLink {
  const passthrough: TicketLink = { href: rawUrl, sponsored: false }

  const affiliate = resolveTicketVendor(rawUrl)?.affiliate
  if (!affiliate) return passthrough

  const partnerId = partnerIds[affiliate.network]
  if (!partnerId) return passthrough

  const trimmed = rawUrl.trim()
  if (!ABSOLUTE_HTTP_URL.test(trimmed)) return passthrough

  // Parsed only to confirm the value is a URL at all; the rewrite below works
  // on the original text so nothing is re-encoded.
  try {
    new URL(trimmed)
  } catch {
    return passthrough
  }

  const { base, pairs, fragment } = splitUrlForTagging(trimmed)
  const param = affiliate.param.toLowerCase()
  const credited = pairs.filter(
    pair => pair.key.toLowerCase() === param && pair.value !== ''
  )
  if (credited.length > 0) {
    return {
      href: rawUrl,
      sponsored: credited.some(pair => pair.value === partnerId),
    }
  }

  const kept = pairs.filter(pair => pair.key.toLowerCase() !== param)
  const query = [
    ...kept.map(pair => pair.raw),
    `${affiliate.param}=${encodeURIComponent(partnerId)}`,
  ].join('&')
  return { href: `${base}?${query}${fragment}`, sponsored: true }
}
