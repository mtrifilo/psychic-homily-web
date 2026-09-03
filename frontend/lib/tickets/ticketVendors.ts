/**
 * The one place this app knows anything about ticket vendors: who they are
 * (the name structured data is willing to print) and how an outbound link to
 * them is tagged once the site is an affiliate.
 *
 * The visible ticket surfaces name the vendor and link to it only when the
 * click is paid for; the `seller` name in the show's `MusicEvent` JSON-LD
 * names the company without linking to it. They must not drift about who a
 * stored URL belongs to.
 *
 * It owns classification ({@link resolveTicketVendor}), naming
 * ({@link ticketVendorLabel}), repair ({@link repairTicketUrl}), tagging
 * ({@link ticketLink}), the paid-link test ({@link carriesOurAffiliateTag})
 * and the rule the visible surfaces render ({@link ticketOffer}). A call site
 * owns only its own POLICY: which refusals apply before there is anything to
 * offer, and whether admission is free.
 *
 * {@link ticketLink} must stay TOTAL. Callers are not required to repair
 * first, and `show.ticket_url` is open contribution that publishes without
 * review, so junk, relative and protocol-relative values reach it in practice
 * and take its pass-through branch. Its guards are not dead code.
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
 * for any vendor that carries an affiliate entry, a robots-disallowed path
 * would be tagged like any other. (Ticketmaster's `/goto*` is the live example
 * of such a path, but Ticketmaster is untaggable today for the separate reason
 * recorded on {@link TICKET_VENDORS_BY_DOMAIN}.) None of that is load-bearing
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
   * to. Absent means links to this vendor stay untagged forever, no matter
   * what partner IDs are configured, and therefore that the ticket surfaces
   * name the vendor instead of linking to it.
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
 * ONLY `ticketweb.com` carries an affiliate entry, and the omissions are
 * deliberate rather than unfinished:
 *
 *  - `ticketmaster.com` sits in the same Impact program, but whether
 *    Ticketmaster has Impact's DIRECT-DOMAIN (`irmp`) tracking enabled is
 *    explicitly unverified: it is a per-advertiser configuration, and the
 *    affiliate research records it as a question for onboarding. Tagging it on
 *    that assumption would declare a paid relationship on the table's
 *    highest-volume vendor while possibly earning nothing. Confirm at
 *    onboarding, then add the entry.
 *  - Front Gate, Universe, Veeps and Moshtix ride the same approval but are
 *    not in the table at all, and a row here is also a claim that we can name
 *    that company as the `seller` in structured data. Adding them is a
 *    deliberate follow-up, not a side effect of this change.
 */
export const TICKET_VENDORS_BY_DOMAIN: Record<string, TicketVendor> = {
  'dice.fm': { name: 'DICE' },
  'eventbrite.com': { name: 'Eventbrite' },
  'ticketmaster.com': { name: 'Ticketmaster' },
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
 * One spelling of a hostname, shared by everything in this module that names
 * one.
 *
 * A single trailing dot is the fully-qualified spelling of the same host
 * ("ticketweb.com." resolves to TicketWeb), so it must not read as a different
 * domain. Left in during CLASSIFICATION it silently opts a real vendor URL out
 * of both its seller name and its affiliate tag; left in on a REPORTED host it
 * splits one vendor into two identities, evading a host-scoped alert and
 * taking a second dedupe slot. Both call sites normalize here so the two
 * cannot disagree about what host a URL is on.
 */
function normalizeHost(hostname: string): string {
  return hostname.toLowerCase().replace(/\.$/, '')
}

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
  const host = ticketUrlHost(rawUrl)
  if (host === null) return undefined
  const domain = vendorDomainFor(host)
  return domain ? TICKET_VENDORS_BY_DOMAIN[domain] : undefined
}

/**
 * Whether a stored ticket URL names a host at all.
 *
 * The question "is this a destination", asked directly. A caller needing it
 * must not reach for {@link ticketVendorLabel} instead: that one answers a
 * DISPLAY question and is null for a hostless value only incidentally, so a
 * future placeholder for unnamed vendors would silently turn every such value
 * back into a destination.
 */
export function namesTicketHost(rawUrl: string | undefined | null): boolean {
  return ticketUrlHost(rawUrl) !== null
}

/**
 * The normalized host a stored ticket URL points at, or null when it names
 * none.
 *
 * The ONE reading of contributor paste in this module: everything that asks
 * what host a value is on goes through here, so classification and naming
 * cannot answer differently. A value with no scheme is given one, because
 * submitters type bare hosts.
 */
function ticketUrlHost(rawUrl: string | undefined | null): string | null {
  const raw = rawUrl?.trim()
  if (!raw) return null
  const candidate = ABSOLUTE_HTTP_URL.test(raw)
    ? raw
    : `https://${raw.replace(/^\/+/, '')}`
  try {
    return normalizeHost(new URL(candidate).hostname) || null
  } catch {
    return null
  }
}

/**
 * The table key this host belongs to, or undefined.
 *
 * Host-anchored, never a substring test: that is what stops `evil-dice.fm`
 * and `dice.fm.evil.test` borrowing a real vendor's name or affiliate tag.
 */
function vendorDomainFor(host: string): string | undefined {
  return Object.keys(TICKET_VENDORS_BY_DOMAIN).find(
    domain => host === domain || host.endsWith(`.${domain}`)
  )
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

/**
 * An affiliate tag found ALREADY PRESENT in a stored ticket URL.
 *
 * "Planted" is exact, not a guess: this app never writes a tag into the
 * database. {@link ticketLink} appends ours at render time and returns a
 * string, so anything found in the stored value was put there by whoever
 * submitted it. On shows that is any authenticated contributor, publishing
 * without review.
 *
 * Carries the parameter NAME and the HOST only. The value is a partner's
 * account identifier and the rest of the URL is the contributor's text; the
 * two fields here are what an operator needs to find the row.
 */
export interface PlantedTicketTag {
  /** Affiliate parameter name, as the vendor would read it. Never its value. */
  param: string
  /** Hostname the link points at. Never the path, query or fragment. */
  host: string
  /**
   * This tag is plausibly OUR OWN rendered link copied back into a submission:
   * it credits an ID this deployment is configured with, on a vendor whose own
   * domain we tag, through that vendor's own parameter.
   *
   * All three clauses are load-bearing. A partner ID rides in public URLs, so
   * the value alone is attacker-settable on any host; an operator filtering
   * this away would then miss exactly the reports worth reading.
   *
   * Benign when true, and once the config is flipped it is the likeliest
   * source, because our own rendered links are the ones in circulation.
   * Separated so the noisy case cannot bury the one worth acting on. Always
   * false on a build with no partner ID configured, which has no tag of its
   * own.
   */
  matchesConfiguredPartner: boolean
}

export interface TicketLink {
  /** The href to render. */
  href: string
  /**
   * This link carries a known affiliate tag — OURS OR SOMEBODY ELSE'S — so the
   * visible anchor must carry `rel="sponsored"`. Google's link-spam policy
   * requires paid links to be qualified, and that is a fact about the link
   * rather than about who is paid.
   *
   * NOT "this link earns us money". A contributor can plant a stranger's
   * partner ID, and this is true for that link too. Anything measuring revenue
   * or attribution needs its own derivation; see {@link ticketLink} for why
   * over-qualifying is the deliberate choice.
   */
  sponsored: boolean
  /**
   * Set when the STORED value already carried an affiliate tag, which is
   * always somebody else's doing. Null otherwise, including when this call
   * appended our own.
   *
   * Reported, not acted on: the link still renders exactly as stored. See
   * `lib/tickets/plantedTagTelemetry`.
   */
  plantedTag: PlantedTicketTag | null
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
 * A query-string key reduced to the form a vendor's server will compare on.
 *
 * The decode is real and the leniency stops there. A server percent-decodes
 * parameter NAMES and reads `+` as a space, so `%69rmp` genuinely arrives as
 * `irmp` and matching raw text would let that spelling pass as "no tag here",
 * appending ours beside one already present and delivering two competing IDs.
 *
 * But query keys are CASE-SENSITIVE and no mainstream parser trims them, so
 * `IRMP`, `irmp%20` and `+irmp` arrive as `IRMP`, `irmp ` and ` irmp`, none of
 * which is the parameter the advertiser reads. Lowercasing or trimming here
 * would treat them as somebody's credit and refuse to add ours, handing a
 * contributor a one-character lever to suppress the tag forever while the page
 * announced a paid link that pays nobody. Matching what the vendor actually
 * reads is what makes both answers right.
 *
 * Used ONLY for comparison; output always keeps the stored spelling. A
 * malformed escape makes `decodeURIComponent` throw, which this module never
 * does.
 */
function affiliateParamKey(key: string): string {
  const spaced = key.replace(/\+/g, ' ')
  try {
    return decodeURIComponent(spaced)
  } catch {
    // Keep the raw spelling: an undecodable key is not a tag we recognize.
    return spaced
  }
}

/**
 * Every affiliate parameter this module knows about, in normalized form.
 *
 * Read for QUALIFICATION, which is a different question from tagging and has a
 * different scope. Tagging asks "does THIS vendor have a program we are in";
 * qualification asks "is this link already monetized for somebody", and that
 * is true of a planted `?irmp=` on any host, including a vendor with no
 * affiliate entry and a host not in the table at all. Scoping the check to the
 * matched vendor left every host but one publishing planted tags unqualified.
 */
const KNOWN_AFFILIATE_PARAMS: ReadonlySet<string> = new Set(
  [
    // Every network's parameter, whether or not a vendor currently uses it: a
    // planted tag is planted regardless of which vendors we have onboarded.
    IMPACT_DIRECT_TRACKING.param,
    ...Object.values(TICKET_VENDORS_BY_DOMAIN).flatMap(vendor =>
      vendor.affiliate ? [vendor.affiliate.param] : []
    ),
  ].map(affiliateParamKey)
)

/**
 * Splits a query segment the way a server that accepts `;` as a separator
 * would, for MATCHING only.
 *
 * `?a=1;irmp=<theirs>` hides a tag from a `&`-only parser: the guard sees one
 * pair named `a` and appends ours, delivering two competing IDs to any vendor
 * that splits on `;`. This module already preserves semicolons on the write
 * side because such servers exist, so it cannot also refuse to look behind
 * one.
 *
 * MATCHING ONLY, and the asymmetry with removal is deliberate. The write-back
 * unit is the whole `&`-segment, so a segment cannot be partially rewritten:
 * dropping `irmp=;x=1` to remove a valueless `irmp` would take `x=1` with it.
 * {@link ticketLink} therefore refuses to remove any `;`-bearing segment,
 * which keeps byte preservation absolute at the cost of leaving a valueless
 * tag in place there.
 */
function matchableKeys(pair: QueryPair): { key: string; value: string }[] {
  return pair.raw.split(';').map(part => {
    const eq = part.indexOf('=')
    return eq === -1
      ? { key: part, value: '' }
      : { key: part.slice(0, eq), value: part.slice(eq + 1) }
  })
}

/**
 * The known affiliate parameter this segment already credits somebody through,
 * or null. Returns the NAME only: the value is a partner's account identifier
 * and never leaves this module.
 */
/**
 * Whether the tag in this segment credits an ID this deployment is configured
 * with, i.e. us.
 *
 * Compared against both the raw and the percent-encoded spelling, because the
 * write side encodes and a copied-back link therefore carries the encoded
 * form. Returns false when nothing is configured, which is the honest answer:
 * a build with no partner ID has no tag of its own for this to be.
 */
function creditsConfiguredPartner(
  pair: QueryPair,
  partnerIds: AffiliatePartnerIds
): boolean {
  const ours = Object.values(partnerIds).filter(Boolean)
  if (ours.length === 0) return false

  return matchableKeys(pair).some(part => {
    if (part.value === '') return false
    if (!KNOWN_AFFILIATE_PARAMS.has(affiliateParamKey(part.key))) return false
    return ours.some(
      id => part.value === id || part.value === encodeURIComponent(id)
    )
  })
}

function affiliateTagIn(pair: QueryPair): string | null {
  for (const part of matchableKeys(pair)) {
    if (part.value === '') continue
    const key = affiliateParamKey(part.key)
    if (KNOWN_AFFILIATE_PARAMS.has(key)) return key
  }
  return null
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
 * A stored ticket URL turned into a tagged href and its qualification.
 *
 * NOT by itself the decision to render an anchor. {@link ticketOffer} is the
 * gate every visible surface goes through, and a surface that calls this
 * directly ships an outbound link the site is not paid for.
 *
 * With no partner ID configured for the vendor's network, or for a vendor
 * outside an affiliate program, or for a value that is not an absolute http(s)
 * URL, the input string is returned BYTE-IDENTICAL. That is the contract that
 * makes turning affiliate links on a config flip: nothing about the markup or
 * the call sites changes when the environment starts carrying an ID.
 *
 * When it DOES tag, the result is the TRIMMED stored string plus one appended
 * parameter, ahead of any fragment. Every other byte of the query survives
 * untouched — see {@link splitUrlForTagging} for why that has to be done
 * textually. Surrounding whitespace is the one difference between the two
 * branches, and it is why both call sites run {@link repairTicketUrl} first:
 * fed an already-repaired value, tagging adds a parameter and nothing else.
 *
 * Never throws. Ticket URLs are contributor-entered paste, so junk, relative
 * and protocol-relative values all take the pass-through branch.
 *
 * A vendor URL that ALREADY carries a non-empty value for this parameter is
 * left exactly as stored and reported as SPONSORED, whoever it credits.
 *
 * Both halves of that matter, and neither is about us. `show.ticket_url` is
 * open contribution that publishes without review, so a contributor can plant
 * any partner's tag. Rewriting it would silently redirect their commission to
 * us; leaving it UNQUALIFIED would publish somebody's affiliate link on an
 * indexed page with no `rel="sponsored"`, which is the link-spam exposure this
 * whole module exists to avoid. `sponsored` therefore describes the LINK, not
 * our commercial interest in it: over-qualifying costs nothing, and Google's
 * policy is about paid placements generally, not about who gets paid. It also
 * makes the answer independent of build configuration and of how the value was
 * encoded, which is what keeps this idempotent under re-application.
 *
 * The key is matched as the VENDOR reads it (see {@link affiliateParamKey}):
 * `?%69rmp=` is their credit and gets ours withheld, while `?IRMP=` and
 * `?irmp%20=` are inert spellings that credit nobody and do not block a real
 * tag. `;` is treated as a separator for matching, because a tag hidden behind
 * one would otherwise be invisible to this check.
 *
 * A VALUELESS occurrence (`?irmp` or `?irmp=`) credits nobody — it is a
 * truncated paste, not a competing partner — so it is dropped in favour of a
 * real tag rather than blocking one forever.
 */
export function ticketLink(
  rawUrl: string,
  partnerIds: AffiliatePartnerIds = affiliatePartnerIds()
): TicketLink {
  const passthrough: TicketLink = {
    href: rawUrl,
    sponsored: false,
    plantedTag: null,
  }

  // DETECTION runs on the repaired form, so a stored value that merely lacks a
  // scheme is still inspected. Scoping it to already-absolute values made the
  // two safety properties fail together on one input class: a planted tag on
  // "ticketweb.com/e/2?irmp=..." was neither qualified nor reported, and the
  // only thing preventing that shipping was every caller happening to repair
  // first. Detection must not rest on call-site discipline.
  const inspectable = repairTicketUrl(rawUrl)
  if (!inspectable) return passthrough

  // Control characters are stripped by both `new URL` and the browser before a
  // request goes out, so a key spelled "ir<TAB>mp" reaches the vendor as
  // `irmp`. A textual scan that kept them would read it as an unknown
  // parameter and append ours beside it, delivering two competing IDs.
  const scannable = inspectable.replace(/[\t\n\r]/g, '')

  let parsed: URL
  try {
    parsed = new URL(scannable)
  } catch {
    return passthrough
  }

  // The vendor behind this URL, read once: qualification below is deliberately
  // NOT scoped to it, but `matchesConfiguredPartner` is.
  const vendorAffiliate = resolveTicketVendor(rawUrl)?.affiliate

  // Qualification comes FIRST, and is not scoped to the matched vendor or to
  // this build's configuration: a planted tag on a vendor we have not
  // onboarded, or on a host not in the table at all, is still a monetized link
  // published on an indexed page.
  for (const pair of splitUrlForTagging(scannable).pairs) {
    const param = affiliateTagIn(pair)
    if (param) {
      return {
        href: rawUrl,
        sponsored: true,
        plantedTag: {
          param,
          host: normalizeHost(parsed.hostname),
          // Our own rendered links circulate, so a contributor copying one
          // back in is the most likely source once the config is flipped.
          // Told apart here so the common case cannot drown the one that
          // redirects our commission to somebody else. The boolean ships; the
          // ID never does.
          //
          // A CONJUNCTION, not a value comparison. A partner ID rides in
          // public URLs, so anyone can append ours to any host; matching the
          // value alone would call that "our own link" and hide the report
          // behind the benign filter. It is only our link if it is also on a
          // vendor whose own domain we tag, through that vendor's own
          // parameter.
          matchesConfiguredPartner:
            !!vendorAffiliate &&
            affiliateParamKey(vendorAffiliate.param) === param &&
            creditsConfiguredPartner(pair, partnerIds),
        },
      }
    }
  }

  if (!vendorAffiliate) return passthrough

  const partnerId = partnerIds[vendorAffiliate.network]
  if (!partnerId) return passthrough

  // REWRITING, unlike detection, refuses a value whose scheme it would have to
  // invent: the href rendered must be the stored string plus a parameter.
  const trimmed = rawUrl.trim()
  if (!ABSOLUTE_HTTP_URL.test(trimmed)) return passthrough

  const { base, pairs, fragment } = splitUrlForTagging(trimmed)

  const param = affiliateParamKey(vendorAffiliate.param)
  const isOurParam = (key: string) => affiliateParamKey(key) === param

  // A valueless occurrence hidden inside a `;`-bearing segment cannot be
  // removed (see below) and must not be appended beside either: on the very
  // servers the `;` handling exists for, the result would be `irmp=""` and
  // `irmp="<ours>"`, and a first-value-wins parser credits nobody while the
  // page claims a paid placement. Left exactly as stored instead.
  const hiddenValuelessTag = pairs.some(
    pair =>
      pair.raw.includes(';') &&
      matchableKeys(pair).some(part => isOurParam(part.key) && part.value === '')
  )
  if (hiddenValuelessTag) return passthrough

  // Removal works on whole `&`-segments, so it must only claim a segment that
  // is ENTIRELY the valueless tag. A `;`-bearing segment carries other
  // parameters (`irmp=;x=1`), and dropping it to clear the tag would delete
  // them — the one way this function could lose a stored byte.
  const kept = pairs.filter(
    pair => pair.raw.includes(';') || !isOurParam(pair.key)
  )
  const query = [
    ...kept.map(pair => pair.raw),
    `${vendorAffiliate.param}=${encodeURIComponent(partnerId)}`,
  ].join('&')
  return {
    href: `${base}?${query}${fragment}`,
    sponsored: true,
    plantedTag: null,
  }
}

/**
 * The name to print for the vendor behind a stored ticket URL, or null when
 * the value names no host.
 *
 * A known vendor prints the name written down in
 * {@link TICKET_VENDORS_BY_DOMAIN}; anything else prints its own hostname,
 * which is a fact about the URL rather than a claim about a company. That
 * split is why this is not `resolveTicketVendor(...)?.name`: a surface that
 * declines to LINK an unrecognized vendor still has to name it. Structured
 * data must not use this — a `seller` is a claim about a real company, so
 * `lib/seo/jsonld` stays on {@link resolveTicketVendor}.
 *
 * `www.` is dropped because it is not part of how a reader names a site.
 */
export function ticketVendorLabel(
  rawUrl: string | undefined | null
): string | null {
  const host = ticketUrlHost(rawUrl)
  if (host === null) return null
  const domain = vendorDomainFor(host)
  return domain ? TICKET_VENDORS_BY_DOMAIN[domain].name : host.replace(/^www\./, '')
}

/**
 * Whether THIS BUILD appended our partner ID to the href, which is the only
 * form of "the click is paid for" this module will act on.
 *
 * A different question from `sponsored`, which is also true for a tag found in
 * the STORED value. Only {@link ticketLink}'s tagging branch produces a
 * sponsored link with no planted tag, so the conjunction is the whole test.
 *
 * DELIBERATELY CONSERVATIVE, and it costs a real case: a stored value that
 * already credits our own configured ID pays us just as much, and is reported
 * as planted rather than linked. Crediting it needs a conjunctive test - the
 * host must resolve to a vendor with an affiliate entry, the parameter must be
 * that vendor's own, and the parameter must occur once, since a repeated one
 * makes which value the vendor reads a guess. `matchesConfiguredPartner` alone
 * is NOT that test: it is host- and vendor-agnostic by design (see
 * {@link KNOWN_AFFILIATE_PARAMS}), and an Impact partner ID is public, so
 * accepting it would let any URL carrying `?irmp=<our id>` render a
 * `rel="sponsored"` anchor to an arbitrary host that pays us nothing.
 */
export function carriesOurAffiliateTag(link: TicketLink): boolean {
  return link.sponsored && link.plantedTag === null
}

/**
 * What a ticket surface renders for one stored URL.
 *
 * THE paid-referral rule, and the reason it lives here rather than on either
 * surface: an outbound vendor anchor renders only when the click is paid for,
 * so a vendor with no affiliate entry, a network with no partner ID
 * configured, and a URL carrying a tag somebody else planted all render as
 * the vendor's NAME instead of a link. The show page and the festival page
 * both call this, so the rule cannot have two answers.
 *
 * DISCRIMINATED on `linked`, which is the key a render site must branch on:
 * narrowing through it gives an `href` of type `string`, while reading `href`
 * off the un-narrowed union gives `string | undefined`. A site that branches
 * on anything else (a derived string, a separate boolean) gets the second
 * shape, and `<a href={undefined}>` is legal JSX, so the discriminant is a
 * strong hint rather than a compiler-enforced gate.
 *
 * `freeAdmission` is the one exemption, and it is an INPUT because only the
 * caller knows: an RSVP or guestlist link on an event that states a price of
 * zero is the reader's only route in. Festivals record no price and never
 * pass it.
 *
 * THE EXEMPTION IS THE WEAK POINT, and its limit is worth stating exactly. It
 * refuses a link carrying an affiliate parameter THIS MODULE KNOWS
 * ({@link KNOWN_AFFILIATE_PARAMS}, which is built from the vendor table and
 * today holds `irmp` alone). It is not a test for monetization in general: a
 * stored `?aff=`, `?tag=` or `?cjevent=` is invisible to it, and price and
 * ticket_url are contributor-writable on the same unreviewed form. Such a
 * link renders, qualified `ugc` so it passes no ranking credit, but it does
 * send readers somewhere that may pay a stranger. Closing the class means
 * either narrowing what the exemption accepts or dropping it; both are
 * product decisions rather than something to infer here.
 *
 * The href it links must be an absolute http(s) URL. A scheme-less value
 * reaches this function as a RELATIVE href, and linking one navigates inside
 * this site instead of out.
 *
 * `plantedTag` rides both shapes, because the report is about the STORED
 * value rather than about what a page renders.
 *
 * `vendorName` is null only for a value that names no host
 * ({@link ticketVendorLabel}), and such a value is never `linked`: there is
 * nothing to navigate to and nobody to name.
 */
export type TicketOffer = {
  vendorName: string | null
  plantedTag: PlantedTicketTag | null
} & (
  | {
      linked: true
      href: string
      /**
       * Google's link-spam qualification for this href: a paid link is
       * `sponsored`, and a free-admission link is a contributor-supplied
       * destination nobody pays for, which is `ugc`.
       */
      sponsored: boolean
      ugc: boolean
    }
  | { linked: false; href?: never; sponsored?: never; ugc?: never }
)

export function ticketOffer(
  rawUrl: string,
  {
    freeAdmission = false,
    partnerIds,
  }: { freeAdmission?: boolean; partnerIds?: AffiliatePartnerIds } = {}
): TicketOffer {
  const link = ticketLink(rawUrl, partnerIds ?? affiliatePartnerIds())
  const vendorName = ticketVendorLabel(rawUrl)
  const shared = { vendorName, plantedTag: link.plantedTag }

  // A value that names no host is not a destination: there is nothing to
  // navigate to and nobody to name, so no shape of this offer may link it.
  // The scheme floor is the same refusal for a value that would render as a
  // relative href; both are checked here rather than at the call sites so a
  // third surface inherits them.
  if (!namesTicketHost(rawUrl) || !ABSOLUTE_HTTP_URL.test(rawUrl.trim())) {
    return { ...shared, linked: false }
  }

  const paid = carriesOurAffiliateTag(link)
  // `sponsored` is true for any affiliate parameter this module recognizes,
  // so a tag that credits somebody else keeps the exemption from linking it.
  // See the docblock for what this test does NOT catch.
  const unpaidButFree = freeAdmission && !link.sponsored
  if (!paid && !unpaidButFree) return { ...shared, linked: false }

  return {
    ...shared,
    linked: true,
    // Trimmed, so the href handed to a render site is the value the floor
    // above actually tested. `ticketLink` preserves surrounding whitespace on
    // its pass-through branch, and a raw <a href> would carry it out.
    href: link.href.trim(),
    sponsored: paid,
    ugc: !paid,
  }
}
