'use client'

import { useSyncExternalStore } from 'react'
import {
  matchesSoftKeyboardViewport,
  subscribeSoftKeyboardViewport,
} from '@/lib/softKeyboardViewport'

const getServerSnapshot = (): boolean => false

/**
 * True on viewports where a filter overlay opens as a bottom sheet rather than
 * as a popover anchored to its trigger.
 *
 * Subscribed rather than read once at open, because the value also decides the
 * trigger's ARIA contract (`aria-haspopup="dialog"` versus `role="combobox"`),
 * which a reader has to be able to trust before the control is activated.
 * `false` on the server, so the server HTML is the popover contract and the
 * client swaps it at hydration; the trigger renders the same box either way,
 * so nothing around it moves.
 */
export function useSoftKeyboardViewport(): boolean {
  return useSyncExternalStore(
    subscribeSoftKeyboardViewport,
    matchesSoftKeyboardViewport,
    getServerSnapshot
  )
}
