'use client'

import { Badge } from '@/components/ui/badge'
import type { ShowResponse } from '@/lib/types/show'

interface ShowStatusBadgeProps {
  show: ShowResponse
  className?: string
}

/**
 * Displays status badges for cancelled and/or sold out shows
 */
export function ShowStatusBadge({ show, className }: ShowStatusBadgeProps) {
  if (!show.is_cancelled && !show.is_sold_out) {
    return null
  }

  return (
    <span className={className}>
      {show.is_cancelled && (
        <Badge variant="destructive" className="text-xs font-semibold">
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
