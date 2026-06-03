'use client'

import { useState, useEffect, useCallback } from 'react'

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
 * The timer is armed in an effect keyed on the shown entry (not imperatively
 * inside `show`), so `show` is a pure state setter — safe to call from event
 * handlers, async continuations, AND during render via the
 * adjust-state-during-render idiom (collections' `useAutoDismissError` relies
 * on this).
 *
 * Scope note: this is the shared consolidation target named in PSY-957.
 * Collections adopts it now (its 5 banner call sites). The pre-existing
 * per-feature auto-dismiss timers — comments' `useAutoDismissError`
 * (`features/comments/hooks`), contributions' `useEntitySaveSuccessBanner`,
 * and `StatusBanner`'s embedded `dismissAfterMs` — are tracked for migration
 * onto this primitive separately so the refactor stays reviewable.
 */
export function useAutoDismissBanner<T>(delayMs: number): {
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

  useEffect(() => {
    if (entry === null || !entry.autoDismiss) return
    const timer = setTimeout(() => setEntry(null), delayMs)
    // Cleanup covers every cancellation path: re-show (effect re-runs),
    // sticky replacement, manual clear, and unmount.
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
 * (edit-save success, link-copied). Returns `[visible, trigger]`; `trigger`
 * is referentially stable, so it's safe in dependency arrays.
 */
export function useAutoDismissFlag(delayMs: number): [boolean, () => void] {
  const { value, show } = useAutoDismissBanner<true>(delayMs)
  const trigger = useCallback(() => show(true), [show])
  return [value === true, trigger]
}
