import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'

export interface InlineErrorBannerProps {
  /** Banner content — typically the error message string. */
  children: ReactNode

  /**
   * Extra Tailwind classes merged with the destructive-tone defaults.
   * Useful for layout tweaks (e.g. `flex items-start gap-2` when the
   * banner needs to host an icon + label).
   */
  className?: string

  /** Forwarded as `data-testid` onto the rendered `<div>`. */
  testId?: string
}

/**
 * Inline error banner used on mutation / form / preview failures across the
 * app (e.g. tag-admin surfaces wired by PSY-610). Bakes in `role="alert"`
 * and the canonical destructive-tone Tailwind classes documented in
 * `pattern_mutation_feedback.md`. Purely presentational — does NOT couple
 * to any mutation hook or state machine.
 */
export function InlineErrorBanner({
  children,
  className,
  testId,
}: InlineErrorBannerProps) {
  return (
    <div
      role="alert"
      data-testid={testId}
      className={cn(
        'rounded-lg border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive',
        className
      )}
    >
      {children}
    </div>
  )
}
