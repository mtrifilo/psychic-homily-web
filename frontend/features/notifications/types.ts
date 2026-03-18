// Notification filter types — aligned with backend contracts/notification_filter.go response types.

/** City criteria for notification filter */
export interface FilterCity {
  city: string
  state: string
}

/** Notification filter response from the API */
export interface NotificationFilter {
  id: number
  name: string
  is_active: boolean
  artist_ids?: number[] | null
  venue_ids?: number[] | null
  label_ids?: number[] | null
  tag_ids?: number[] | null
  exclude_tag_ids?: number[] | null
  cities?: FilterCity[] | null
  price_max_cents?: number | null
  notify_email: boolean
  notify_in_app: boolean
  notify_push: boolean
  match_count: number
  last_matched_at?: string | null
  created_at: string
  updated_at: string
}

/** Notification log entry from the API */
export interface NotificationLogEntry {
  id: number
  filter_id?: number | null
  filter_name?: string
  entity_type: string
  entity_id: number
  channel: string
  sent_at: string
  read_at?: string | null
}

/** Create filter request body */
export interface CreateFilterInput {
  name: string
  artist_ids?: number[]
  venue_ids?: number[]
  label_ids?: number[]
  tag_ids?: number[]
  exclude_tag_ids?: number[]
  cities?: FilterCity[]
  price_max_cents?: number | null
  notify_email?: boolean
  notify_in_app?: boolean
}

/** Update filter request body (all fields optional) */
export interface UpdateFilterInput {
  name?: string
  is_active?: boolean
  artist_ids?: number[]
  venue_ids?: number[]
  label_ids?: number[]
  tag_ids?: number[]
  exclude_tag_ids?: number[]
  cities?: FilterCity[]
  price_max_cents?: number | null
  notify_email?: boolean
  notify_in_app?: boolean
}

/** Quick-create filter request body */
export interface QuickCreateFilterInput {
  entity_type: 'artist' | 'venue' | 'label' | 'tag'
  entity_id: number
}

/** Entity types that support quick-create notification filters */
export const NOTIFY_ENTITY_TYPES = ['artist', 'venue', 'label', 'tag'] as const
export type NotifyEntityType = (typeof NOTIFY_ENTITY_TYPES)[number]

/** Helper: format relative time for last_matched_at */
export function formatTimeAgo(dateString: string): string {
  const date = new Date(dateString)
  const now = new Date()
  const diffMs = now.getTime() - date.getTime()
  const diffSeconds = Math.floor(diffMs / 1000)
  const diffMinutes = Math.floor(diffSeconds / 60)
  const diffHours = Math.floor(diffMinutes / 60)
  const diffDays = Math.floor(diffHours / 24)
  const diffWeeks = Math.floor(diffDays / 7)

  if (diffSeconds < 60) return 'just now'
  if (diffMinutes === 1) return '1 minute ago'
  if (diffMinutes < 60) return `${diffMinutes} minutes ago`
  if (diffHours === 1) return '1 hour ago'
  if (diffHours < 24) return `${diffHours} hours ago`
  if (diffDays === 1) return '1 day ago'
  if (diffDays < 7) return `${diffDays} days ago`
  if (diffWeeks === 1) return '1 week ago'
  if (diffWeeks < 5) return `${diffWeeks} weeks ago`
  return date.toLocaleDateString('en-US', {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  })
}

/** Helper: build a human-readable summary of filter criteria */
export function getFilterSummary(filter: NotificationFilter): string {
  const parts: string[] = []

  if (filter.artist_ids?.length) {
    parts.push(
      `${filter.artist_ids.length} ${filter.artist_ids.length === 1 ? 'artist' : 'artists'}`
    )
  }
  if (filter.venue_ids?.length) {
    parts.push(
      `${filter.venue_ids.length} ${filter.venue_ids.length === 1 ? 'venue' : 'venues'}`
    )
  }
  if (filter.label_ids?.length) {
    parts.push(
      `${filter.label_ids.length} ${filter.label_ids.length === 1 ? 'label' : 'labels'}`
    )
  }
  if (filter.tag_ids?.length) {
    parts.push(
      `${filter.tag_ids.length} ${filter.tag_ids.length === 1 ? 'tag' : 'tags'}`
    )
  }
  if (filter.exclude_tag_ids?.length) {
    parts.push(
      `excluding ${filter.exclude_tag_ids.length} ${filter.exclude_tag_ids.length === 1 ? 'tag' : 'tags'}`
    )
  }
  if (filter.cities?.length) {
    parts.push(
      filter.cities.map(c => `${c.city}, ${c.state}`).join('; ')
    )
  }
  if (filter.price_max_cents != null) {
    if (filter.price_max_cents === 0) {
      parts.push('free only')
    } else {
      parts.push(`max $${(filter.price_max_cents / 100).toFixed(0)}`)
    }
  }

  return parts.length > 0 ? parts.join(' / ') : 'No criteria set'
}
