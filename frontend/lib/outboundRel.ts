/**
 * The hygiene tokens every outbound anchor carries, whatever else is true of
 * it. `noopener` denies the opened page a handle back to ours; `noreferrer`
 * keeps our URL out of the vendor's logs.
 */
const HYGIENE_TOKENS = ['noopener', 'noreferrer']

/**
 * The `rel` for an outbound anchor: the hygiene tokens, plus `sponsored` when
 * the link is monetized and `ugc` when the destination is contributor-supplied
 * and nobody is paid for it.
 *
 * One owner for the rule as it applies to MONETIZED links, which is set by
 * Google's link-spam policy rather than by this codebase. Two call sites need
 * that and cannot share a component: `BracketLink` is a generic primitive that
 * must not learn what a ticket vendor is, and the festival page's ticket link
 * is an icon-and-text anchor matched to its neighbours rather than a bracket.
 *
 * NOT yet the owner for outbound `rel` site-wide. Roughly thirty anchors still
 * hardcode `rel="noopener noreferrer"` — including the "Official Website" link
 * directly above the festival ticket link, and the radio station/show links.
 * Those are contributor-submitted destinations that want `ugc` too; passing it
 * here is one anchor opting in, not the sweep.
 *
 * `ugc` is for a destination a contributor chose that earns the site nothing:
 * the free-admission ticket link is the one such anchor today, and it is the
 * only outbound ticket link that can render on a build with no partner ID, so
 * leaving it unqualified would concentrate every unpaid outbound ticket click
 * on an unreviewed, contributor-set field.
 *
 * Both tokens are ADDITIVE by construction: qualifying a link can never cost
 * it its opener and referrer protection. They are not mutually exclusive to
 * this function, though no caller sets both today.
 */
export function outboundRel(sponsored = false, ugc = false): string {
  return [
    ...HYGIENE_TOKENS,
    ...(sponsored ? ['sponsored'] : []),
    ...(ugc ? ['ugc'] : []),
  ].join(' ')
}
