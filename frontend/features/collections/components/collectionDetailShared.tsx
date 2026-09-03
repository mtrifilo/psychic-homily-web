'use client'

/**
 * Small helpers shared across the collections feature's banner surfaces
 * (`CollectionDetail`, its lazily-loaded items list `CollectionItemsList`,
 * and `CollectionCard`). Extracted in PSY-951 so the items list — which
 * carries the `@dnd-kit/*` drag-reorder machinery — can live in its own module
 * and be `dynamic()`-imported (evicting `@dnd-kit` from the global shared
 * client chunk) without a circular import back into `CollectionDetail.tsx`.
 *
 * PSY-957: the generic auto-dismiss timer primitives now live in
 * `@/lib/hooks/common/useAutoDismissBanner` (the cross-feature consolidation
 * home). What stays here is collections-specific: the `MutationFeedback`
 * render primitive, the 403-aware error copy, and `useAutoDismissError` (the
 * reactive wrapper that adapts a TanStack mutation's error state onto the
 * shared banner timer). This module stays dependency-light (react + lucide
 * icons + cn only) — keep it that way so importing it from browse-surface
 * components (CollectionCard) never drags detail-page-only libs into the
 * shared chunk.
 */

import { useState } from 'react'
import { Mic2, MapPin, Calendar, Disc3, Tag, Tent, Check, AlertCircle } from 'lucide-react'
import { cn } from '@/lib/utils'
import { useAutoDismissBanner } from '@/lib/hooks/common/useAutoDismissBanner'

export const ENTITY_ICONS: Record<string, React.ElementType> = {
  artist: Mic2,
  venue: MapPin,
  show: Calendar,
  release: Disc3,
  label: Tag,
  festival: Tent,
}

/**
 * What a `replaced: true` creation request means to the person who filed it:
 * the queue holds ONE request under this name, and it now carries what they
 * just submitted. Shared by both queue-create surfaces (the AI filler's per-row
 * chip, the paste picker's per-line chip) so the sentence cannot drift.
 *
 * Every chip that shows it must carry it BOTH as a `title` and as sr-only text.
 * A title reaches neither a screen reader nor a touch device, and `aria-label`
 * is not an option: Badge renders a bare <div>, on which browsers drop it.
 */
export const REPLACED_REQUEST_EXPLANATION =
  'Your earlier request was replaced with this one'

/**
 * Render a 4xx mutation failure with dedicated copy for 403. Falls back to the
 * server's `detail`/`message` for everything else, then to the caller's copy.
 *
 * A 403 MEANS PERMISSION, NEVER PRIVACY. Every collection service refusal that
 * means "you may not see this" is a 404 carrying the same body an unused slug
 * gets, and each tests visibility BEFORE ownership, so a 403 from the collection
 * API is a collection the viewer can see and is not permitted to change. Which
 * callers are permitted differs per route (creator only for the collection and
 * the reorder, creator or item adder for the item writes), which is why the copy
 * names permission rather than a role.
 *
 * The copy therefore says permission and says nothing about visibility. Copy
 * naming privacy here would restate in the UI the fact the API answers 404 to
 * withhold. This branch keys on the status alone, so a 403 raised outside the
 * collection API (an edge or auth layer) renders it too; that is the same
 * trade-off the previous copy made, on a message that is now neutral.
 */
export function describeCollectionMutationError(
  err: unknown,
  fallback: string
): string {
  const status =
    err && typeof err === 'object' && 'status' in err
      ? Number((err as { status?: number }).status)
      : undefined
  if (status === 403) {
    return 'You do not have permission to change this collection.'
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
      ? 'text-success-foreground'
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
 * `formatter` is invoked only at the error-signal edge (when `isError`/`err`
 * change), not inside any dependency array, so its referential stability is
 * no longer required for timer correctness (PSY-957 moved the timer off a
 * formatter-derived dep). Keeping the existing call sites' `useCallback`
 * wrappers is tidy, not mandatory.
 *
 * PSY-957: timer mechanics live in `useAutoDismissBanner`; this wrapper owns
 * only the "react to a mutation error-state change" part. One edge differs
 * from the pre-PSY-957 `[message]`-keyed effect: a *fresh* error object whose
 * formatted copy is identical to the previous one now re-arms the dismiss
 * window (the banner times its 3s from the latest failure), because the timer
 * keys on the shown entry's identity rather than the message string. This is
 * the intended "re-arm on re-show" semantics — see the
 * `useAutoDismissError` re-arm test.
 */
export function useAutoDismissError(
  err: unknown,
  isError: boolean,
  formatter: (e: unknown) => string,
  delayMs = 3000
): string | null {
  const { value, show } = useAutoDismissBanner<string>(delayMs)

  // Show the formatted error the moment the mutation errors (or when the
  // error signal changes while still erroring). React 19.2: adjust state
  // during render via the previous-value-guard idiom instead of a synchronous
  // setState in an effect (cascading render). The tracker starts at a sentinel
  // so the guard also fires on the FIRST render when `isError` is already true
  // (matching the prior effect, which always ran on mount). `show` is a pure
  // state setter (see useAutoDismissBanner), so calling it here is the same
  // documented idiom.
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
      show(formatter(err))
    }
  }

  return value
}
