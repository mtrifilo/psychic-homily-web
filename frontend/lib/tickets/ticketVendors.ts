/**
 * The one place this app knows anything about ticket vendors: who they are
 * (the name structured data is willing to print) and how an outbound link to
 * them is tagged once the site is an affiliate.
 *
 * Two surfaces read it and must not drift: the visible Buy Tickets link on the
 * show and festival pages, and the `seller` name in the show's `MusicEvent`
 * JSON-LD. The JSON-LD deliberately emits no offer URL at all, so this module
 * is the only thing that ever touches a vendor URL.
 */

/**
 * An affiliate network whose partner ID tags a vendor's OWN domain.
 *
 * Direct-domain tracking only. Every network's redirect domain
 * (`ticketmaster.evyy.net` and `imp.pxf.io` for Impact, CJ's `anrdoezrs.net`,
 * Rakuten's `click.linksynergy.com`, Skimlinks) answers Googlebot with
 * `Disallow: /`, so a link routed through one is uncrawlable and could never
 * be reused as a structured-data URL. The guard is structural rather than
 * advisory: {@link ticketLink} can only append a query parameter to the URL a
 * contributor already stored, and has no way to emit a different host.
 *
 * The choice of PARAMETER is still per-vendor and still has to be verified
 * against that vendor's robots.txt before the vendor is given an
 * {@link VendorAffiliate} entry, because a vendor can disallow the very shape
 * its tracking produces: Ticketmaster disallows `/goto*` (their redirect
 * links) and AXS disallows `/*referrer=*` (any URL carrying that param).
 * Impact's `irmp` parameter on the advertiser's own domain avoids both.
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
 * Ticket vendors we recognize, keyed by registrable domain.
 *
 * `ticketmaster.com` and `ticketweb.com` sit inside one Ticketmaster/Impact
 * program; the same approval also covers Front Gate, Universe, Veeps and
 * Moshtix, which are absent here because naming a vendor is a structured-data
 * claim of its own and none of those has been added as a seller yet.
 */
/**
 * Impact's direct-domain tracking: the partner ID rides on the advertiser's
 * own host as `?irmp=`. Shared by every vendor inside one Impact program, so
 * correcting the parameter is one edit rather than one per vendor.
 */
const IMPACT_DIRECT_TRACKING: VendorAffiliate = {
  network: 'impact',
  param: 'irmp',
}

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
 * A stored ticket URL turned into the href the page actually renders.
 *
 * With no partner ID configured for the vendor's network, or for a vendor
 * outside an affiliate program, or for a value that is not an absolute http(s)
 * URL, the input string is returned BYTE-IDENTICAL. That is the contract that
 * makes turning affiliate links on a config flip: nothing about the markup or
 * the call sites changes when the environment starts carrying an ID.
 *
 * Never throws. Ticket URLs are contributor-entered paste, so junk, relative
 * and protocol-relative values all take the pass-through branch.
 *
 * A vendor URL that ALREADY carries the affiliate parameter is left exactly as
 * stored, whoever it credits: overwriting someone else's tracking ID inside a
 * URL a contributor submitted would silently redirect their commission to us.
 * It counts as sponsored only when the ID present is our own, which also makes
 * this idempotent under re-application.
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

  let url: URL
  try {
    url = new URL(trimmed)
  } catch {
    return passthrough
  }

  const existing = url.searchParams.get(affiliate.param)
  if (existing !== null) {
    return { href: rawUrl, sponsored: existing === partnerId }
  }

  url.searchParams.set(affiliate.param, partnerId)
  return { href: url.toString(), sponsored: true }
}
