import type { ReactNode } from 'react'
import type { LucideIcon } from 'lucide-react'

import { cn } from '@/lib/utils'

export interface AdminEmptyStateProps {
  /** Lucide icon rendered inside the muted square chip. */
  icon: LucideIcon
  /**
   * Optional short heading, e.g. "No pending items". Rendered as an <h3>
   * when provided. Omit for the lighter icon + message variant (some
   * surfaces, e.g. Radio, only ever had a single line of copy).
   */
  title?: string
  /** One-line supporting message. */
  message: string
  /** Optional call-to-action (e.g. a Button) rendered below the message. */
  action?: ReactNode
  className?: string
  testId?: string
}

/**
 * Canonical admin empty state — bordered card + square muted icon chip +
 * heading + message. Replaces the per-surface hand-rolled variants so every
 * "nothing to review" surface reads identically.
 *
 * This is a deliberate visual NORMALIZATION to the PSY-912 mock, not a
 * pixel-for-pixel dedup. Beyond unifying the already-carded surfaces, it
 * also wraps the previously BORDERLESS `py-12` bare-centered variants
 * (audit-log / reports / users / moderation) and the DASHED-border Radio
 * variants in the same solid `border + bg-card` box. It standardizes the
 * icon chip to `rounded-md` (`rounded-full` is banned by the editorial
 * direction), the card fill to full-opacity `bg-card` (some originals used
 * `bg-card/50`), and the heading to `text-base font-semibold` (originals
 * varied between `text-lg font-medium` and `font-medium`). Per-call-site
 * copy and heading level (`h3`) are preserved.
 */
export function AdminEmptyState({
  icon: Icon,
  title,
  message,
  action,
  className,
  testId,
}: AdminEmptyStateProps) {
  return (
    <div
      className={cn(
        'flex flex-col items-center justify-center rounded-lg border border-border bg-card p-8 text-center',
        className
      )}
      data-testid={testId}
    >
      <div className="mb-4 flex h-14 w-14 items-center justify-center rounded-md bg-muted">
        <Icon className="h-6 w-6 text-muted-foreground" />
      </div>
      {title && <h3 className="mb-1 text-base font-semibold">{title}</h3>}
      <p className="max-w-sm text-sm text-muted-foreground">{message}</p>
      {action && <div className="mt-4">{action}</div>}
    </div>
  )
}
