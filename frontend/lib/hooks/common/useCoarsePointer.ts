'use client'

import { useMediaQuery } from './useMediaQuery'

/**
 * Whether the primary pointer is coarse (`pointer: coarse`, a finger rather
 * than a mouse or trackpad), re-rendering when that changes mid-session.
 * False on the server snapshot, per useMediaQuery's contract, so the server
 * HTML is the fine-pointer rendering.
 */
export function useCoarsePointer(): boolean {
  return useMediaQuery('(pointer: coarse)')
}
