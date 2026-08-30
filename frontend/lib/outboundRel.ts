/**
 * The hygiene tokens every outbound anchor carries, whatever else is true of
 * it. `noopener` denies the opened page a handle back to ours; `noreferrer`
 * keeps our URL out of the vendor's logs.
 */
const HYGIENE_TOKENS = ['noopener', 'noreferrer']

/**
 * The `rel` for an outbound anchor: the hygiene tokens, plus `sponsored` when
 * the link is monetized.
 *
 * One owner for the rule as it applies to MONETIZED links, which is set by
 * Google's link-spam policy rather than by this codebase. Two call sites need
 * that and cannot share a component: `BracketLink` is a generic primitive that
 * must not learn what a ticket vendor is, and the festival page's ticket link
 * is an icon-and-text anchor matched to its neighbours rather than a bracket.
 *
 * NOT yet the owner for outbound `rel` site-wide. Roughly thirty anchors still
 * hardcode `rel="noopener noreferrer"` — including the "Official Website" link
 * directly above the festival ticket link, and the radio station/show links,
 * which are the contributor-submitted destinations a future `ugc` token would
 * target. Adding such a token is still a grep until those are swept; this
 * function is where the sweep should land, not evidence that it happened.
 *
 * `sponsored` is ADDITIVE by construction: qualifying a paid link can never
 * cost that link its opener and referrer protection.
 */
export function outboundRel(sponsored = false): string {
  return (sponsored ? [...HYGIENE_TOKENS, 'sponsored'] : HYGIENE_TOKENS).join(
    ' '
  )
}
