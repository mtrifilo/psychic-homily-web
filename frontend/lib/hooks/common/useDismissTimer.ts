import { useCallback, useEffect, useRef } from 'react'

/**
 * A cancelable delayed dismiss — the "hover-intent grace period" timer primitive.
 *
 * `schedule()` calls `onDismiss` after `delayMs`; `cancel()` clears a pending
 * dismiss; the timer is always cleared on unmount (no fire-after-unmount). Use it
 * to keep a transient UI element (a hoverable tooltip, a menu) open while the
 * pointer travels onto it, then dismiss shortly after it leaves.
 *
 * Extracted for PSY-1218 (the artist-graph node tooltip) so the timer lifecycle —
 * the part most prone to subtle bugs (cancel races, falsy-zero ids, missing
 * unmount cleanup) — is tested once in isolation instead of hand-rolled per call
 * site. `useHoverIntentMenu` and `useAutoDismissBanner` re-derive the same
 * lifecycle and could adopt this later.
 *
 * `onDismiss` is read through a ref, so callers may pass a fresh closure each
 * render (e.g. one that reads current state) without re-creating `schedule` /
 * `cancel` — their identities stay stable across renders.
 */
export function useDismissTimer(onDismiss: () => void, delayMs: number) {
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  // Latest-ref pattern so `schedule`/`cancel` stay identity-stable even when the
  // caller passes a fresh `onDismiss` closure each render. Written in an effect (not
  // during render) per react-hooks/refs; the timer only fires after `delayMs`, well
  // after the effect has committed the current callback.
  const onDismissRef = useRef(onDismiss)
  useEffect(() => {
    onDismissRef.current = onDismiss
  }, [onDismiss])

  const cancel = useCallback(() => {
    // `!== null`, not a truthiness check — a setTimeout id can legitimately be 0,
    // which `if (timerRef.current)` would skip, leaking the pending dismiss.
    if (timerRef.current !== null) {
      clearTimeout(timerRef.current)
      timerRef.current = null
    }
  }, [])

  const schedule = useCallback(() => {
    cancel()
    timerRef.current = setTimeout(() => {
      timerRef.current = null
      onDismissRef.current()
    }, delayMs)
  }, [cancel, delayMs])

  useEffect(() => cancel, [cancel])

  return { schedule, cancel }
}
