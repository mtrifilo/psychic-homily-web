import { Pencil, Flag, MessageSquare, type LucideIcon } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'

/**
 * Moderation "kind" of a queue item. Drives the colored category badge on
 * each moderation card.
 */
export type AdminCategoryKind = 'edit' | 'report' | 'comment'

interface KindConfig {
  label: string
  icon: LucideIcon
  /**
   * Ad-hoc Tailwind tone classes, preserved VERBATIM from the original
   * inline badges so this extraction is a pure no-visual-change dedup.
   * PSY-908 will swap these for DS chart/category tokens.
   */
  tone: string
}

const KIND_CONFIG: Record<AdminCategoryKind, KindConfig> = {
  edit: {
    label: 'Edit',
    icon: Pencil,
    tone: 'bg-blue-500/10 text-blue-700 dark:text-blue-400 border-blue-200 dark:border-blue-800',
  },
  report: {
    label: 'Report',
    icon: Flag,
    tone: 'bg-amber-500/10 text-amber-700 dark:text-amber-400 border-amber-200 dark:border-amber-800',
  },
  comment: {
    label: 'Comment',
    icon: MessageSquare,
    tone: 'bg-violet-500/10 text-violet-700 dark:text-violet-400 border-violet-200 dark:border-violet-800',
  },
}

export interface CategoryBadgeProps {
  kind: AdminCategoryKind
  className?: string
  testId?: string
}

/**
 * Colored "kind" badge for moderation queue items (Edit / Report / Comment).
 * Centralizes the five inline `<Badge variant="secondary">` copies that
 * previously hard-coded these colors per card. Visual output is identical
 * to the originals — see PSY-908 for migrating the tones to DS tokens.
 */
export function CategoryBadge({ kind, className, testId }: CategoryBadgeProps) {
  const { label, icon: Icon, tone } = KIND_CONFIG[kind]

  return (
    <Badge
      variant="secondary"
      className={cn('shrink-0', tone, className)}
      data-testid={testId}
    >
      <Icon className="h-3 w-3 mr-1" />
      {label}
    </Badge>
  )
}
