'use client'

import { Badge } from '@/components/ui/badge'
import type { ShowResponse } from '../types'

interface ShowStatusBadgeProps {
  show: ShowResponse
  className?: string
  /**
   * How the CANCELLED chip is painted. `destructive` is the card register that
   * every surface but the dense `/shows` row uses; that row's frame draws it in
   * primary, where a red block among muted mono columns reads as an error state
   * rather than a status.
   */
  cancelledVariant?: 'destructive' | 'default'
}

/**
 * Displays status badges for cancelled and/or sold out shows
 */
export function ShowStatusBadge({
  show,
  className,
  cancelledVariant = 'destructive',
}: ShowStatusBadgeProps) {
  if (!show.is_cancelled && !show.is_sold_out) {
    return null
  }

  return (
    <span className={className}>
      {show.is_cancelled && (
        <Badge variant={cancelledVariant} className="text-xs font-semibold">
          CANCELLED
        </Badge>
      )}
      {show.is_sold_out && (
        <Badge variant="secondary" className="text-xs font-semibold bg-orange-100 text-orange-800 dark:bg-orange-900/30 dark:text-orange-400">
          SOLD OUT
        </Badge>
      )}
    </span>
  )
}
