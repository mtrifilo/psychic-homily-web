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
 * the part most prone to subtle bugs (cancel races, falsy-zero ids, missing unmount
 * cleanup) — is tested once in isolation instead of inline at the call site.
 * (`useHoverIntentMenu` / `useAutoDismissBanner` hand-roll similar lifecycles but
 * around different state machines; consolidating them onto this primitive would be a
 * separate refactor, not assumed here.)
 *
 * `onDismiss` is read through a ref, so callers may pass a fresh closure each render
 * (e.g. one that reads current state) without re-creating `schedule` / `cancel` —
 * their identities stay stable across renders. `delayMs`, by contrast, is captured by
 * `schedule` at call time and is NOT re-applied to an already-pending timer, so pass
 * a stable value (callers use module constants).
 *
 * ## Why a bare `setTimeout` is not good enough (PSY-1664)
 *
 * This is the canonical explanation; call sites just point here.
 *
 * An untracked `setTimeout` in a blur/dismiss handler still fires after the
 * component unmounts, and its `setState` then runs against a React DOM that no
 * longer has a document. In the browser that is a harmless no-op warning. Under
 * vitest it lands after the jsdom environment has been torn down and throws
 * `ReferenceError: window is not defined` from `resolveUpdatePriority`, which
 * vitest reports as an unhandled error and fails the ENTIRE run — with every
 * test passing, and the stack trace blaming whatever PR happens to be in flight.
 * It is a race against teardown, so it stays invisible on a fast machine and
 * surfaces on a loaded CI runner (PR #1757 was failed by exactly this).
 *
 * Scope note: this and `useAutoDismissBanner` are the two shared timer
 * lifecycles for the app. PSY-1664 swept the frontend onto them. New deferred
 * UI actions should use one of these primitives, not a hand-rolled timer.
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
