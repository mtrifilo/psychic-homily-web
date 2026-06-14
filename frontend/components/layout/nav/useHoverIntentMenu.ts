'use client'

import { useCallback, useEffect, useRef, useState } from 'react'

// Hover-intent timing, feel-tuned short for a snappy enter/leave (PSY-1089,
// shortened further in the PSY-1094 follow-up for an even faster open). The open
// dwell is deliberately near-instant; at 50ms a pointer merely passing over the
// trigger can briefly pop the panel — an accepted (and now slightly larger)
// trade-off for the snappier feel, not a bug. The close delay (200ms) is
// feel-tuned, not derived: it just has to comfortably outlast the time to drag a
// pointer across the trigger→panel gap (`sideOffset` on the menu content) so
// diagonal travel hits the panel's `onPointerEnter` → `clearTimer` and cancels
// the close instead of dismissing mid-move. The two are tuned independently —
// shrinking the open dwell (or `sideOffset`) does not license shrinking this
// delay. (NN/G's nominal 0.5s dwell is an upper bound; see
// docs/open-questions/navigation-redesign.md.)
const OPEN_DELAY_MS = 50
const CLOSE_DELAY_MS = 200

type HoverHandlers = {
  onPointerEnter: () => void
  onPointerLeave: () => void
}

export interface HoverIntentMenu {
  /** Controlled open state — pass to `<DropdownMenu open={…}>`. */
  open: boolean
  /** Pass to `<DropdownMenu onOpenChange={…}>`; integrates click + keyboard. */
  onOpenChange: (next: boolean) => void
  /** Spread onto `<DropdownMenuTrigger>` (enter opens, leave closes). */
  triggerHoverProps: HoverHandlers
  /** Spread onto `<DropdownMenuContent>` (enter keeps open, leave closes). */
  contentHoverProps: HoverHandlers
}

// NN/G hover-intent for a Radix DropdownMenu, shared by BrowseMenu and
// ContributeMenu so the two behave identically (PSY-1094). Radix DropdownMenu is
// click-only; this layers a controlled `open` state with open/close timers on top
// (Radix still owns click + keyboard). Pointer parity: hovering EITHER the
// trigger or the panel keeps it open; leaving both closes it after the delay.
//
// REQUIRES `modal={false}` on the consuming `<DropdownMenu>`: Radix's default
// modal menu sets `pointer-events: none` on <body> while open, which strips the
// trigger's pointer events the instant the menu opens → the browser fires
// pointerleave on the trigger → scheduleClose → close → pointer is over the
// trigger again → scheduleOpen → an endless open/close flicker. Non-modal keeps
// the page (and trigger) pointer-interactive; outside-click + Escape dismissal
// still work via Radix's dismissable layer.
export function useHoverIntentMenu(): HoverIntentMenu {
  const [open, setOpen] = useState(false)
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const clearTimer = useCallback(() => {
    if (timerRef.current) {
      clearTimeout(timerRef.current)
      timerRef.current = null
    }
  }, [])

  // Avoid leaking a pending open/close timer if the component unmounts mid-dwell.
  useEffect(() => clearTimer, [clearTimer])

  const scheduleOpen = useCallback(() => {
    clearTimer()
    timerRef.current = setTimeout(() => setOpen(true), OPEN_DELAY_MS)
  }, [clearTimer])

  const scheduleClose = useCallback(() => {
    clearTimer()
    timerRef.current = setTimeout(() => setOpen(false), CLOSE_DELAY_MS)
  }, [clearTimer])

  // Click / keyboard go through Radix's own open state; cancel any pending hover
  // timer so a click doesn't get clobbered by a late open/close fire.
  const onOpenChange = useCallback(
    (next: boolean) => {
      clearTimer()
      setOpen(next)
    },
    [clearTimer]
  )

  return {
    open,
    onOpenChange,
    triggerHoverProps: { onPointerEnter: scheduleOpen, onPointerLeave: scheduleClose },
    contentHoverProps: { onPointerEnter: clearTimer, onPointerLeave: scheduleClose },
  }
}
