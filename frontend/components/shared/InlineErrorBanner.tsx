import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'

/**
 * Visual variants of the destructive-tone inline error banner.
 *
 * - `default` — compact `p-3 text-sm` shape used for mutation / form / preview
 *   failures inline above the failing input or above the form itself.
 * - `queryFallback` — `p-4 text-center` shape used as a list-load fallback
 *   when a feature page can't render its primary query result. The wider
 *   padding + center alignment lets it hold its own when it replaces the
 *   missing list/grid in the page layout.
 *
 * Both variants share the same destructive tone tokens, the same
 * `role="alert"` semantics, and the same `className` merge behaviour. The
 * variant only swaps the layout-shape classes documented in
 * `pattern_mutation_feedback.md`.
 */
export type InlineErrorBannerVariant = 'default' | 'queryFallback'

export interface InlineErrorBannerProps {
  /** Banner content — typically the error message string. */
  children: ReactNode

  /**
   * Visual variant. Defaults to `default` (the compact mutation-error
   * shape). Use `queryFallback` for query-load fallbacks that replace a
   * list/grid in the page layout.
   */
  variant?: InlineErrorBannerVariant

  /**
   * Extra Tailwind classes merged with the variant defaults. Useful for
   * layout tweaks (e.g. `flex items-start gap-2` when the banner needs to
   * host an icon + label).
   */
  className?: string

  /** Forwarded as `data-testid` onto the rendered `<div>`. */
  testId?: string
}

const VARIANT_CLASSES: Record<InlineErrorBannerVariant, string> = {
  default: 'p-3 text-sm text-destructive',
  queryFallback: 'p-4 text-center text-destructive',
}

/**
 * Inline error banner used on mutation / form / preview failures and
 * query-load fallbacks across admin entity-management surfaces (e.g.
 * tag/label/festival/release admin wired by PSY-623 + PSY-630). Bakes in
 * `role="alert"` and the canonical destructive-tone Tailwind classes
 * documented in `pattern_mutation_feedback.md`. Purely presentational —
 * does NOT couple to any mutation hook or state machine.
 */
export function InlineErrorBanner({
  children,
  variant = 'default',
  className,
  testId,
}: InlineErrorBannerProps) {
  return (
    <div
      role="alert"
      data-testid={testId}
      className={cn(
        'rounded-lg border border-destructive/50 bg-destructive/10',
        VARIANT_CLASSES[variant],
        className
      )}
    >
      {children}
    </div>
  )
}
