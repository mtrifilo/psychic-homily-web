// Notification filter types — aligned with backend contracts/notification_filter.go response types.

import { formatTimeAgo as sharedFormatTimeAgo } from '@/lib/formatTimeAgo'

/** City criteria for notification filter */
export interface FilterCity {
  city: string
  state: string
}

/**
 * NotificationLogEntry mirrors backend contracts/notification_filter.go
 * `NotificationLogEntry`. Two row shapes share this contract:
 *
 *  - Show-filter rows: entity_type="show", channel="email", filter_name set.
 *  - Comment-driven rows (PSY-595): entity_type="comment_reply" |
 *    "comment_mention", channel="in_app". The backend enriches each
 *    comment-driven row with commenter_*, comment_excerpt, comment_url, and
 *    comment_entity_* so the bell + inbox UI can render a clickable line.
 *
 * Comment-enrichment fields are optional; show rows leave them undefined.
 */
export interface NotificationLogEntry {
  id: number
  filter_id?: number | null
  filter_name?: string
  entity_type: string
  entity_id: number
  channel: string
  sent_at: string
  read_at?: string | null
  // Comment-driven enrichment (PSY-595)
  commenter_name?: string
  commenter_username?: string
  comment_excerpt?: string
  comment_url?: string
  comment_entity_type?: string
  comment_entity_id?: number
  comment_entity_name?: string
  // Request-driven enrichment (PSY-890): entity_id holds the request_id; the
  // backend populates request_title + request_url for the inbox row + link.
  request_title?: string
  request_url?: string
}

/** GET /me/notifications response shape */
export interface NotificationListResponse {
  notifications: NotificationLogEntry[]
  unread_count: number
}

/** POST /me/notifications/mark-read response shape */
export interface MarkReadResponse {
  updated_count: number
  unread_count: number
}

/**
 * Backend notification_log entity_type values for PSY-595 in-app rows. Must
 * stay in sync with the engagement.NotificationEntity* constants on the Go
 * side — both surfaces query / render the same rows.
 */
export const NOTIFICATION_ENTITY_COMMENT_REPLY = 'comment_reply' as const
export const NOTIFICATION_ENTITY_COMMENT_MENTION = 'comment_mention' as const

/**
 * Backend notification_log entity_type for the PSY-890 request-fulfillment
 * in-app row. Must stay in sync with
 * models/notification.NotificationEntityRequestFulfillmentProposed.
 */
export const NOTIFICATION_ENTITY_REQUEST_FULFILLMENT_PROPOSED =
  'request_fulfillment_proposed' as const

/** isCommentNotification returns true for the PSY-595 comment row types. */
export function isCommentNotification(entry: NotificationLogEntry): boolean {
  return (
    entry.entity_type === NOTIFICATION_ENTITY_COMMENT_REPLY ||
    entry.entity_type === NOTIFICATION_ENTITY_COMMENT_MENTION
  )
}

/** isRequestNotification returns true for the PSY-890 request-fulfillment row. */
export function isRequestNotification(entry: NotificationLogEntry): boolean {
  return entry.entity_type === NOTIFICATION_ENTITY_REQUEST_FULFILLMENT_PROPOSED
}

/** Notification filter ownership (PSY-1467). Must stay in sync with
 * models/notification.FilterSource* on the Go side. */
export const FILTER_SOURCE_USER = 'user' as const
export const FILTER_SOURCE_MANAGED = 'managed' as const
export type FilterSource =
  | typeof FILTER_SOURCE_USER
  | typeof FILTER_SOURCE_MANAGED

/** Notification filter response from the API */
export interface NotificationFilter {
  id: number
  name: string
  /** 'user' = settings-authored; 'managed' = entity-page quick toggle (PSY-1467) */
  source: FilterSource
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

/** Entity types that support quick-create notification filters */
export const NOTIFY_ENTITY_TYPES = ['artist', 'venue', 'label', 'tag'] as const
export type NotifyEntityType = (typeof NOTIFY_ENTITY_TYPES)[number]

/**
 * Helper: format relative time for last_matched_at.
 *
 * Re-exports the canonical `formatTimeAgo` (PSY-780). Prior copy was missing
 * the month branch, so anything older than 5 weeks dropped straight to an
 * absolute date — the divergence was accidental, not a deliberate UX choice.
 */
export const formatTimeAgo = sharedFormatTimeAgo

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
