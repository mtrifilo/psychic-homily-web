'use client'

import { useState, type ReactNode } from 'react'
import { Check, Clock } from 'lucide-react'
import { cn } from '@/lib/utils'
import { useAutoDismissBanner } from '@/lib/hooks/common/useAutoDismissBanner'

// Sentinel so the start-visible guard below fires on the FIRST render too
// (a real `dismissAfterMs` — number or undefined — never equals it).
const DISMISS_UNSET = Symbol('dismiss-unset')

export type StatusBannerVariant = 'success' | 'pending'

export interface StatusBannerProps {
  /**
   * Visual tone:
   * - `success` — green, used for completed mutations (Changes saved,
   *   Report submitted, Approve / Reject success).
   * - `pending` — amber, used for "submitted, awaiting review" states
   *   (pending edits, pending comments, pending field notes).
   */
  variant: StatusBannerVariant

  /** Banner content. Caller controls inner typography. */
  children: ReactNode

  /**
   * Override the default leading icon. Default: `<Check>` for success,
   * `<Clock>` for pending. Pass `null` to suppress the icon entirely.
   */
  icon?: ReactNode

  /**
   * If set, the banner auto-hides after this many milliseconds. The
   * timer is cleared on unmount and on re-render with a different value.
   * `onDismiss` (if provided) fires when the timer elapses.
   *
   * Omitting this prop means the banner stays visible until the parent
   * unmounts it — the right call for in-drawer banners (drawer dismiss
   * is the implicit close) and parent-managed page banners that already
   * own a timer (see {@link EntitySaveSuccessBanner} / its hook).
   */
  dismissAfterMs?: number

  /** Fires when the auto-dismiss timer elapses. No-op if `dismissAfterMs` is unset. */
  onDismiss?: () => void

  /** Forwarded as `data-testid` onto the rendered `<div>`. */
  testId?: string

  /**
   * Extra Tailwind classes merged with the variant defaults. Useful when
   * a caller needs page-level layout adjustments (e.g. removing the
   * default `mb-4` margin or constraining width).
   */
  className?: string
}

/**
 * Inline status banner — green for success, amber for pending review.
 * Replaces 4–5 hand-rolled banners that used the same Tailwind chrome
 * (PSY-575). The codebase has no toast library; inline banners on the
 * affected surface are the project convention — see
 * `pattern_mutation_feedback.md`.
 *
 * Layout: `<icon>` + free-form children. Outer chrome (border + bg +
 * padding) and the default icon colour come from the variant. Inner
 * typography is the caller's responsibility, so a banner can render a
 * single sentence (pending) or a "title + description" stack (success).
 *
 * Auto-dismiss is owned by the primitive when `dismissAfterMs` is
 * supplied; otherwise visibility is controlled by the parent.
 */
export function StatusBanner({
  variant,
  children,
  icon,
  dismissAfterMs,
  onDismiss,
  testId,
  className,
}: StatusBannerProps) {
  // PSY-958: the show-then-auto-dismiss timer is the shared
  // useAutoDismissBanner primitive. Two modes:
  //   - timed (`dismissAfterMs` set): start visible, auto-hide after the
  //     delay, and fire `onDismiss` when it elapses (via `onAutoDismiss`).
  //   - untimed (`dismissAfterMs` undefined): parent-controlled — render until
  //     the parent unmounts us; no timer is ever armed.
  // NOTE: content-change reset is the consumer's responsibility via `key` (the
  // idiomatic React identity reset). A consumer that swaps the banner's
  // children while keeping it mounted with a constant `dismissAfterMs` should
  // pass a `key` that changes per message so each gets a fresh window (see
  // StreamingWorklist) — StatusBanner intentionally doesn't diff children.
  const {
    value: shown,
    show: triggerShow,
    clear: clearShow,
  } = useAutoDismissBanner<true>(dismissAfterMs ?? 0, {
    onAutoDismiss: onDismiss,
  })

  // Start visible — and re-arm if a caller changes `dismissAfterMs` to a new
  // number — by triggering the timer. React 19.2: adjust state during render via
  // the previous-value-guard idiom instead of a mount/update effect. The
  // sentinel makes the guard fire on the FIRST render too (so timed banners
  // show on first paint; the trigger's render-phase setState re-renders before
  // commit, so there's no visible flicker). Transitioning to untimed
  // (`dismissAfterMs` → undefined) clears any live timer so it can't fire a
  // spurious auto-dismiss / onDismiss.
  const [prevDismissAfterMs, setPrevDismissAfterMs] = useState<
    number | undefined | typeof DISMISS_UNSET
  >(DISMISS_UNSET)
  if (dismissAfterMs !== prevDismissAfterMs) {
    setPrevDismissAfterMs(dismissAfterMs)
    if (dismissAfterMs !== undefined) triggerShow(true)
    else clearShow()
  }

  // Timed mode hides once the timer auto-dismisses; untimed mode always renders.
  if (dismissAfterMs !== undefined && shown !== true) return null

  // Variant chrome — kept verbatim from the pre-PSY-575 hand-rolled
  // banners so the visual is byte-identical post-migration:
  //   success: border-green-800 / bg-green-950/50 / p-4 / icon text-green-400
  //   pending: border-amber-700/50 / bg-amber-950/40 / p-3 / icon text-amber-500
  const isSuccess = variant === 'success'
  const iconColorClass = isSuccess ? 'text-green-400' : 'text-amber-500'
  const containerClass = isSuccess
    ? 'border-green-800 bg-green-950/50 p-4'
    : 'border-amber-700/50 bg-amber-950/40 p-3'

  const defaultIcon = isSuccess ? (
    <Check className={cn('h-4 w-4 mt-0.5 shrink-0', iconColorClass)} aria-hidden="true" />
  ) : (
    <Clock className={cn('h-4 w-4 mt-0.5 shrink-0', iconColorClass)} aria-hidden="true" />
  )

  const renderedIcon = icon === undefined ? defaultIcon : icon

  return (
    <div
      role="status"
      aria-live="polite"
      data-testid={testId}
      className={cn(
        'rounded-md border flex items-start gap-2',
        containerClass,
        className
      )}
    >
      {renderedIcon}
      <div className="flex-1 min-w-0">{children}</div>
    </div>
  )
}
