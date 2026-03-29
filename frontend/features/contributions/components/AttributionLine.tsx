'use client'

import Link from 'next/link'
import { useEntityAttribution } from '../hooks/useEntityAttribution'

interface AttributionLineProps {
  entityType: string
  entityId: string | number
}

/**
 * Format a timestamp into a relative time string.
 */
function formatRelativeTime(dateStr: string): string {
  const date = new Date(dateStr)
  const now = new Date()
  const diffMs = now.getTime() - date.getTime()
  const diffSec = Math.floor(diffMs / 1000)
  const diffMin = Math.floor(diffSec / 60)
  const diffHr = Math.floor(diffMin / 60)
  const diffDays = Math.floor(diffHr / 24)

  if (diffSec < 60) return 'just now'
  if (diffMin < 60) return `${diffMin} minute${diffMin === 1 ? '' : 's'} ago`
  if (diffHr < 24) return `${diffHr} hour${diffHr === 1 ? '' : 's'} ago`
  if (diffDays < 30) return `${diffDays} day${diffDays === 1 ? '' : 's'} ago`

  return date.toLocaleDateString('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  })
}

/**
 * Displays "Last edited by [username] · [relative time]" for an entity.
 * Fetches the most recent revision and renders a small attribution line.
 * Returns null if no revisions exist or data is still loading.
 */
export function AttributionLine({ entityType, entityId }: AttributionLineProps) {
  const { data: attribution } = useEntityAttribution(entityType, entityId)

  if (!attribution) {
    return null
  }

  return (
    <p className="text-xs text-muted-foreground">
      Last edited by{' '}
      <Link
        href={`/users/${attribution.userName}`}
        className="hover:underline"
      >
        {attribution.userName}
      </Link>
      {' '}&middot;{' '}
      {formatRelativeTime(attribution.createdAt)}
    </p>
  )
}
