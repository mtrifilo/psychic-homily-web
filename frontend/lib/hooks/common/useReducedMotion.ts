'use client'

import { useMediaQuery } from './useMediaQuery'

/**
 * Whether the visitor has asked for less motion (`prefers-reduced-motion:
 * reduce`), re-rendering when the preference changes mid-session. False on
 * the server snapshot, per useMediaQuery's contract, so motion gated on it
 * stops at hydration rather than in the server HTML.
 */
export function useReducedMotion(): boolean {
  return useMediaQuery('(prefers-reduced-motion: reduce)')
}
