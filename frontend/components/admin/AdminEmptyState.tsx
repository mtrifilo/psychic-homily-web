import type { ReactNode } from 'react'
import type { LucideIcon } from 'lucide-react'

import { cn } from '@/lib/utils'

export interface AdminEmptyStateProps {
  /** Lucide icon rendered inside the muted square chip. */
  icon: LucideIcon
  /** Short heading, e.g. "No pending items". Rendered as an <h3>. */
  title: string
  /** One-line supporting message. */
  message: string
  /** Optional call-to-action (e.g. a Button) rendered below the message. */
  action?: ReactNode
  className?: string
  testId?: string
}

/**
 * Canonical admin empty state — bordered card + square muted icon chip +
 * heading + message. Replaces the per-surface hand-rolled variants
 * (card+icon on Pending Shows, bare-centered on Reports/Moderation, etc.)
 * so every "nothing to review" surface reads identically. Matches the
 * PSY-912 Figma mock (rounded-md icon chip — `rounded-full` is banned by
 * the editorial design direction).
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
      <h3 className="mb-1 text-base font-semibold">{title}</h3>
      <p className="max-w-sm text-sm text-muted-foreground">{message}</p>
      {action && <div className="mt-4">{action}</div>}
    </div>
  )
}
