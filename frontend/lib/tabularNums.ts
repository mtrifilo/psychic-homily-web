import type { ReactNode } from 'react'

const DIGITS_ONLY = /^\d+$/

/**
 * `'tabular-nums'` when the rendered content is a bare run of digits,
 * `undefined` otherwise, for use inside `cn()`.
 *
 * The body face (Schibsted Grotesk) widens comma, period, colon and
 * semicolon to the tabular digit width under `tnum`, so the feature may only
 * cover digits: "1204" keeps aligned figures, while "1,204", "91.1 MHz" or
 * "Tue, Nov 3" would render with a gap before each punctuation mark. Strings
 * and numbers are tested as they render (`String(n)`); any other node returns
 * `undefined` because its text cannot be known here.
 */
export function tabularNumsIfDigitsOnly(content: ReactNode): 'tabular-nums' | undefined {
  if (typeof content !== 'string' && typeof content !== 'number') return undefined
  return DIGITS_ONLY.test(String(content)) ? 'tabular-nums' : undefined
}
