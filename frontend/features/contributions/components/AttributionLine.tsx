'use client'

import { UserAttribution } from '@/components/shared'
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
 *
 * Degrades to "Last edited · [relative time]" when the backend will not name
 * the author (a contributor who hid their contributions, or one whose only
 * resolvable name would be an email fragment — PSY-1940). The credit is dropped
 * rather than replaced with a placeholder: the edit is a fact, the person is
 * not ours to publish. Gated on the NAME rather than the username, because an
 * account with no linkable profile is still a person to credit; UserAttribution
 * renders that as plain text.
 */
export function AttributionLine({ entityType, entityId }: AttributionLineProps) {
  const { data: attribution } = useEntityAttribution(entityType, entityId)

  if (!attribution) {
    return null
  }

  if (!attribution.user_name) {
    return (
      <p className="text-xs text-muted-foreground">
        Last edited {formatRelativeTime(attribution.created_at)}
      </p>
    )
  }

  return (
    <p className="text-xs text-muted-foreground">
      Last edited by{' '}
      <UserAttribution
        name={attribution.user_name}
        username={attribution.user_username}
        className="hover:underline"
      />
      {' '}&middot;{' '}
      {formatRelativeTime(attribution.created_at)}
    </p>
  )
}
