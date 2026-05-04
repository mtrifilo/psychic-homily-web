'use client'

import Link from 'next/link'
import { useEntityAttribution } from '../hooks/useEntityAttribution'
import { formatRelativeTime } from '@/lib/formatRelativeTime'

interface AttributionLineProps {
  entityType: string
  entityId: string | number
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

  // PSY-560: link to /users/:username only when the user has a username
  // slug — falling back to the resolved display name (first/last,
  // email-prefix, "Anonymous") as plain text otherwise. Mirrors the
  // CommentCard byline pattern (PSY-552 / PSY-353).
  return (
    <p className="text-xs text-muted-foreground">
      Last edited by{' '}
      {attribution.userUsername ? (
        <Link
          href={`/users/${attribution.userUsername}`}
          className="hover:underline"
        >
          {attribution.userName}
        </Link>
      ) : (
        <span>{attribution.userName}</span>
      )}
      {' '}&middot;{' '}
      {formatRelativeTime(attribution.createdAt)}
    </p>
  )
}
