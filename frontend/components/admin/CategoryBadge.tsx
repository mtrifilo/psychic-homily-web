import { Pencil, Flag, MessageSquare, PlusCircle, type LucideIcon } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'

/**
 * Moderation "kind" of a queue item. Drives the colored category badge on
 * each moderation card.
 */
export type AdminCategoryKind = 'edit' | 'report' | 'comment' | 'request'

interface KindConfig {
  label: string
  icon: LucideIcon
  /**
   * DS-palette tint for the moderation kind (PSY-943). Bound to the shared
   * categorical `--chart-*` tokens (globals.css, PSY-947) so the three kinds
   * stay distinct-but-muted and track light/dark via the CSS cascade — no more
   * raw blue/amber/violet hues with hand-tuned `dark:` overrides. edit =
   * chart-6 (denim), report = chart-3 (gold, "flag"), comment = chart-7 (plum).
   */
  tone: string
}

const KIND_CONFIG: Record<AdminCategoryKind, KindConfig> = {
  edit: {
    label: 'Edit',
    icon: Pencil,
    tone: 'bg-chart-6/10 text-chart-6 border-chart-6/30',
  },
  report: {
    label: 'Report',
    icon: Flag,
    tone: 'bg-chart-3/10 text-chart-3 border-chart-3/30',
  },
  comment: {
    label: 'Comment',
    icon: MessageSquare,
    tone: 'bg-chart-7/10 text-chart-7 border-chart-7/30',
  },
  // PSY-871: "Request" (queued entity CREATION). Purple "make new thing" tint,
  // locked in the design decision (#5) as distinct from edit/report/comment.
  // The categorical --chart-* palette has no second purple (chart-7 plum is
  // comment's), so this honors the locked purple via a raw hex with a dark
  // variant. Token-ifying the moderation palette (so all four kinds bind to
  // --chart-* uniformly) is scoped to PSY-872's admin cohesion pass (#8).
  request: {
    label: 'Request',
    icon: PlusCircle,
    tone: 'bg-[#a855f7]/10 text-[#7e22ce] border-[#a855f7]/30 dark:text-[#c084fc] dark:border-[#a855f7]/40',
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
