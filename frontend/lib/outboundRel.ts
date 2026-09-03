/**
 * The hygiene tokens every outbound anchor carries, whatever else is true of
 * it. `noopener` denies the opened page a handle back to ours; `noreferrer`
 * keeps our URL out of the vendor's logs.
 */
const HYGIENE_TOKENS = ['noopener', 'noreferrer']

/**
 * The `rel` for an outbound anchor: the hygiene tokens, plus the link-spam
 * qualifiers Google defines.
 *
 * One owner for the rule, which is set by Google's link-spam policy rather
 * than by this codebase. `sponsored` is for a paid link; `ugc` is for a
 * destination a contributor chose that earns the site nothing. Both are hints
 * not to pass ranking credit, and both are ADDITIVE by construction:
 * qualifying a link can never cost it its opener and referrer protection.
 *
 * NOT yet the owner for outbound `rel` site-wide: many anchors still hardcode
 * the hygiene pair, including the "Official Website" link directly above the
 * festival ticket link. This function is where that sweep should land, not
 * evidence that it happened.
 *
 * An OPTIONS object rather than positional flags. The qualifiers are
 * same-typed and read as order-independent, so a transposed pair would
 * produce a plausible-but-wrong `rel` with no type error.
 */
export interface OutboundRelOptions {
  sponsored?: boolean
  ugc?: boolean
}

export function outboundRel({
  sponsored = false,
  ugc = false,
}: OutboundRelOptions = {}): string {
  return [
    ...HYGIENE_TOKENS,
    ...(sponsored ? ['sponsored'] : []),
    ...(ugc ? ['ugc'] : []),
  ].join(' ')
}
