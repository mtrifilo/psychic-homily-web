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
 * ONE owner for a rule that is set by Google's link-spam policy rather than by
 * this codebase, so the next token it asks for (`nofollow` beside `sponsored`,
 * `ugc` for contributor-submitted destinations) is one edit and not a grep.
 * Two call sites need it and cannot share a component: `BracketLink` is a
 * generic primitive that must not learn what a ticket vendor is, and the
 * festival page's ticket link is an icon-and-text anchor matched to its
 * neighbours rather than a bracket.
 *
 * `sponsored` is ADDITIVE by construction: qualifying a paid link can never
 * cost that link its opener and referrer protection.
 */
export function outboundRel(sponsored = false): string {
  return (sponsored ? [...HYGIENE_TOKENS, 'sponsored'] : HYGIENE_TOKENS).join(
    ' '
  )
}
