'use client'

/**
 * Small helpers shared between `CollectionDetail` and its lazily-loaded items
 * list (`CollectionItemsList`). Extracted in PSY-951 so the items list — which
 * carries the `@dnd-kit/*` drag-reorder machinery — can live in its own module
 * and be `dynamic()`-imported (evicting `@dnd-kit` from the global shared
 * client chunk) without a circular import back into `CollectionDetail.tsx`.
 *
 * These were previously module-private in `CollectionDetail.tsx`; behavior is
 * unchanged, this is purely a move so both modules can share them.
 */

import { useState, useRef, useEffect } from 'react'
import {
  Mic2,
  MapPin,
  Calendar,
  Disc3,
  Tag,
  Tent,
  Check,
  AlertCircle,
} from 'lucide-react'
import { cn } from '@/lib/utils'

export const ENTITY_ICONS: Record<string, React.ElementType> = {
  artist: Mic2,
  venue: MapPin,
  show: Calendar,
  release: Disc3,
  label: Tag,
  festival: Tent,
}

/**
 * PSY-609: render a 4xx mutation failure with copy that handles the common
 * "this collection is private" case (403). Falls back to the server's
 * `detail`/`message` for everything else, then to a generic copy.
 *
 * `unlikePrivate` toggles the wording for the like-vs-unlike asymmetry —
 * unlike on a 403 means the target was made private after the like, which
 * deserves slightly different copy from "you can't like a private collection".
 */
export function describeCollectionMutationError(
  err: unknown,
  fallback: string,
  context?: { unlikePrivate?: boolean }
): string {
  const status =
    err && typeof err === 'object' && 'status' in err
      ? Number((err as { status?: number }).status)
      : undefined
  if (status === 403) {
    return context?.unlikePrivate
      ? 'This collection is private — your like was removed.'
      : 'This collection is private.'
  }
  if (err instanceof Error && err.message) return err.message
  return fallback
}

/**
 * PSY-609: shared inline-banner primitive used by the silent collection
 * mutation surfaces. Mirrors the success banner already in
 * AddItemsSection (Check icon + green tone) and adds a destructive
 * variant (AlertCircle + destructive tone). Used as a sibling to the
 * mutating control so screen readers + sighted users see the result on
 * the same card. `role="status"` (vs `alert`) keeps the announcement
 * polite — these are not safety-critical errors.
 */
export function MutationFeedback({
  variant,
  message,
  testId,
}: {
  variant: 'success' | 'error'
  message: string
  testId?: string
}) {
  const Icon = variant === 'success' ? Check : AlertCircle
  const tone =
    variant === 'success'
      ? 'text-green-600 dark:text-green-400'
      : 'text-destructive'
  return (
    <div
      role="status"
      data-testid={testId}
      className={cn('mt-2 flex items-start gap-1.5 text-sm', tone)}
    >
      <Icon className="h-3.5 w-3.5 mt-0.5 shrink-0" aria-hidden="true" />
      <span className="flex-1">{message}</span>
    </div>
  )
}

const ERROR_SIGNAL_UNSET = Symbol('error-signal-unset')

/**
 * PSY-609: when an optimistic-rollback mutation fails (like / unlike /
 * reorder), surface the error inline for ~3s then auto-dismiss so the
 * UI doesn't accrue stale banners after the user already moved on. The
 * snap-back of the optimistic state is the primary signal; this banner
 * just makes the *reason* visible.
 *
 * `formatter` MUST be stable across renders (wrap in useCallback) — it
 * sits in the timer effect's dependency array indirectly via `message`, so
 * an unstable reference would otherwise reset the auto-dismiss timer.
 */
export function useAutoDismissError(
  err: unknown,
  isError: boolean,
  formatter: (e: unknown) => string,
  delayMs = 3000
): string | null {
  const [message, setMessage] = useState<string | null>(null)
  const timeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  // Surface the formatted error the moment the mutation errors (or when the
  // error signal changes while still erroring). React 19.2: adjust state
  // during render via the previous-value-guard idiom instead of a synchronous
  // setState in an effect (cascading render). The tracker starts at a sentinel
  // so the guard also fires on the FIRST render when `isError` is already true
  // (matching the prior effect, which always ran on mount). The auto-dismiss
  // timer below re-arms whenever `message` changes, preserving the prior
  // single-effect behavior. `formatter` is required-stable (see the doc
  // comment), so keying the guard on `isError`/`err` matches the prior
  // dependency semantics.
  const [prevErrorSignal, setPrevErrorSignal] = useState<
    { isError: boolean; err: unknown } | typeof ERROR_SIGNAL_UNSET
  >(ERROR_SIGNAL_UNSET)
  const errorSignalChanged =
    prevErrorSignal === ERROR_SIGNAL_UNSET ||
    prevErrorSignal.isError !== isError ||
    prevErrorSignal.err !== err
  if (errorSignalChanged) {
    setPrevErrorSignal({ isError, err })
    // Only (re)show on the erroring edge; when the error clears we just keep
    // the tracker in step so the next error re-triggers (even with the same
    // `err` value).
    if (isError) {
      setMessage(formatter(err))
    }
  }

  // Arm / re-arm the auto-dismiss timer whenever a message is shown. The
  // setMessage(null) here is inside the deferred timer callback, not
  // synchronous in the effect body.
  useEffect(() => {
    if (message === null) return
    if (timeoutRef.current) clearTimeout(timeoutRef.current)
    timeoutRef.current = setTimeout(() => {
      setMessage(null)
      timeoutRef.current = null
    }, delayMs)
    return () => {
      if (timeoutRef.current) clearTimeout(timeoutRef.current)
    }
  }, [message, delayMs])

  return message
}
