'use client'

import { cn } from '@/lib/utils'

interface EntityHeaderProps {
  /** Entity name displayed as h1 */
  title: string
  /** Optional subtitle line (location, year, type, etc.) */
  subtitle?: React.ReactNode
  /** Optional action buttons (Save, Follow, etc.) */
  actions?: React.ReactNode
  /**
   * Where the action cluster sits relative to the title/subtitle block.
   * - `'inline'` (default): actions sit beside the title at the sm breakpoint
   *   (`sm:justify-between`). Narrow actions (e.g. a single BracketLink linkbox)
   *   leave the title its full width.
   * - `'below'`: title + subtitle render at full width and the action row drops
   *   onto its own line underneath. Use when the actions are a WIDE cluster
   *   (e.g. VenueDetail's full Favorite/Follow/Collect/Notify/Edit/Report/Delete
   *   row) that would otherwise squeeze the title (PSY-959).
   */
  actionsPlacement?: 'inline' | 'below'
  className?: string
}

/**
 * Reusable entity header with title, subtitle, and action buttons.
 *
 * Usage:
 * ```tsx
 * <EntityHeader
 *   title="Album Name"
 *   subtitle={<><Badge>LP</Badge> 2024</>}
 *   actions={<Button>Save</Button>}
 * />
 * ```
 */
export function EntityHeader({
  title,
  subtitle,
  actions,
  actionsPlacement = 'inline',
  className,
}: EntityHeaderProps) {
  const titleBlock = (
    <div className={cn(actionsPlacement === 'inline' && 'flex-1 min-w-0')}>
      <h1 className="text-2xl md:text-3xl font-bold leading-8 md:leading-9">
        {title}
      </h1>
      {subtitle && (
        <div className="flex items-center gap-2 mt-2 text-muted-foreground">
          {subtitle}
        </div>
      )}
    </div>
  )

  const actionsRow = actions && (
    <div className="flex flex-wrap items-center gap-2 sm:shrink-0">{actions}</div>
  )

  if (actionsPlacement === 'below') {
    return (
      <div className={cn('space-y-4', className)}>
        {titleBlock}
        {actionsRow}
      </div>
    )
  }

  return (
    <div className={cn('space-y-2', className)}>
      <div className="flex flex-col sm:flex-row sm:items-start sm:justify-between gap-4">
        {titleBlock}
        {actionsRow}
      </div>
    </div>
  )
}
