'use client'

import { cn } from '@/lib/utils'

interface EntityHeaderProps {
  /** Entity name displayed as h1 */
  title: string
  /** Optional subtitle line (location, year, type, etc.) */
  subtitle?: React.ReactNode
  /** Optional action buttons (Save, Follow, etc.) */
  actions?: React.ReactNode
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
  className,
}: EntityHeaderProps) {
  return (
    <div className={cn('space-y-2', className)}>
      <div className="flex items-start justify-between gap-4">
        <div className="flex-1 min-w-0">
          <h1 className="text-2xl md:text-3xl font-bold leading-8 md:leading-9">
            {title}
          </h1>
          {subtitle && (
            <div className="flex items-center gap-2 mt-2 text-muted-foreground">
              {subtitle}
            </div>
          )}
        </div>
        {actions && (
          <div className="flex items-center gap-2 shrink-0">{actions}</div>
        )}
      </div>
    </div>
  )
}
