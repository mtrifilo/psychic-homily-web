'use client'

import { useEffect, useState, type ReactNode } from 'react'
import { Check, Clock } from 'lucide-react'
import { cn } from '@/lib/utils'

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
  const [hidden, setHidden] = useState(false)

  // Reset visibility whenever the timer config changes — supports the
  // "show, auto-dismiss, show again on a later mutation" cycle without
  // requiring the caller to remount.
  useEffect(() => {
    setHidden(false)
  }, [dismissAfterMs])

  useEffect(() => {
    if (dismissAfterMs === undefined) return

    const timer = setTimeout(() => {
      setHidden(true)
      onDismiss?.()
    }, dismissAfterMs)

    return () => clearTimeout(timer)
  }, [dismissAfterMs, onDismiss])

  if (hidden) return null

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
