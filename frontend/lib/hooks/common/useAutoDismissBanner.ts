'use client'

import { useState, useEffect, useCallback, useRef } from 'react'

interface AutoDismissOptions {
  /**
   * Fires when the auto-dismiss timer elapses and clears the value (NOT on a
   * manual `clear()`, a `showSticky()` replacement, or unmount). Lets a
   * caller react to "the banner timed out on its own" — e.g. `StatusBanner`'s
   * `onDismiss` prop. The latest reference is always used, so it need not be
   * memoized.
   */
  onAutoDismiss?: () => void
}

/**
 * PSY-957: the one timer implementation behind auto-dismiss banners. Holds a
 * banner value (message string or feedback object), shows it for `delayMs`,
 * then clears it. Centralizes the lifecycle every banner call site used to
 * hand-roll — arm, re-arm on re-show, clear on unmount — so new banners can't
 * get those wrong (one pre-PSY-957 collections banner, for example, never
 * cleared its timer on unmount).
 *
 * - `show(value)` — display + auto-dismiss after `delayMs`. Re-showing before
 *   dismissal re-arms the timer, so the banner stays up for a full window
 *   after the latest trigger (even when the value is identical).
 * - `showSticky(value)` — display until explicitly cleared or replaced. For
 *   non-optimistic failures the user needs time to read (the PSY-608/609
 *   sticky-vs-auto-dismiss policy).
 * - `clear()` — hide immediately, canceling any pending dismissal.
 *
 * `value` is `null` only when nothing is shown; consumers typically gate
 * rendering on it (`{value && <Banner .../>}`). Don't `show()` a falsy-but-valid
 * value (empty string, `0`) — the truthiness gate would suppress the banner
 * while the dismiss timer still runs. Wrap such payloads in an object instead.
 *
 * The timer is armed in an effect keyed on the shown entry (not imperatively
 * inside `show`), so `show` is a pure state setter — safe to call from event
 * handlers, async continuations, AND during render via the
 * adjust-state-during-render idiom (collections' `useAutoDismissError` relies
 * on this).
 *
 * Scope note: this is the shared auto-dismiss timer for the whole app. PSY-957
 * landed it with collections as the first adopter; PSY-958 routed the
 * remaining timers through it — comments' vote-error banner
 * (`CommentVoteControls`), contributions' `useEntitySaveSuccessBanner`, and
 * `StatusBanner`'s `dismissAfterMs` mode (via `onAutoDismiss`). New
 * auto-dismiss banners should use this primitive, not a hand-rolled timer.
 */
export function useAutoDismissBanner<T>(
  delayMs: number,
  options?: AutoDismissOptions
): {
  value: T | null
  show: (value: T) => void
  showSticky: (value: T) => void
  clear: () => void
} {
  // Entry object identity doubles as the re-arm key: every show() creates a
  // new entry, so the timer effect re-runs even when the value is identical
  // (e.g. the same error message twice in a row).
  const [entry, setEntry] = useState<{
    value: T
    autoDismiss: boolean
  } | null>(null)

  // Latest-ref the callback so an unmemoized `onAutoDismiss` doesn't sit in
  // the timer effect's deps (which would re-arm — and reset — the countdown
  // on every render). The timer still always invokes the current callback.
  // The ref is updated in an effect (not during render) per React 19.2's
  // no-ref-writes-during-render rule.
  const onAutoDismissRef = useRef(options?.onAutoDismiss)
  useEffect(() => {
    onAutoDismissRef.current = options?.onAutoDismiss
  })

  useEffect(() => {
    if (entry === null || !entry.autoDismiss) return
    const timer = setTimeout(() => {
      setEntry(null)
      onAutoDismissRef.current?.()
    }, delayMs)
    // Cleanup covers every cancellation path: re-show (effect re-runs),
    // sticky replacement, manual clear, and unmount. onAutoDismiss fires ONLY
    // on the timer callback above — never from this cleanup — so a re-show /
    // clear / unmount does not spuriously signal "dismissed".
    return () => clearTimeout(timer)
  }, [entry, delayMs])

  const show = useCallback(
    (value: T) => setEntry({ value, autoDismiss: true }),
    []
  )
  const showSticky = useCallback(
    (value: T) => setEntry({ value, autoDismiss: false }),
    []
  )
  const clear = useCallback(() => setEntry(null), [])

  return {
    value: entry === null ? null : entry.value,
    show,
    showSticky,
    clear,
  }
}

/**
 * PSY-957: boolean variant of `useAutoDismissBanner` for "it worked" blips
 * (edit-save success, link-copied). Returns a `[visible, trigger]` tuple
 * deliberately — it mirrors `useState`'s `[value, setter]` shape so call
 * sites read as `const [shown, show] = ...`. (The base hook returns an object
 * because it exposes three methods; the tuple here is the intentional
 * exception, not drift.) `trigger` is referentially stable, so it's safe in
 * dependency arrays. Callers needing `onAutoDismiss` / `clear` (e.g.
 * `StatusBanner`) use `useAutoDismissBanner` directly.
 */
export function useAutoDismissFlag(delayMs: number): [boolean, () => void] {
  const { value, show } = useAutoDismissBanner<true>(delayMs)
  const trigger = useCallback(() => show(true), [show])
  return [value === true, trigger]
}
