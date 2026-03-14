// Request types — aligned with backend contracts/request.go response types.

export const REQUEST_ENTITY_TYPES = [
  'artist',
  'release',
  'label',
  'show',
  'venue',
  'festival',
] as const

export type RequestEntityType = (typeof REQUEST_ENTITY_TYPES)[number]

export const REQUEST_STATUSES = [
  'pending',
  'in_progress',
  'fulfilled',
  'rejected',
  'cancelled',
] as const

export type RequestStatus = (typeof REQUEST_STATUSES)[number]

export const REQUEST_SORT_OPTIONS = ['votes', 'newest', 'oldest'] as const

export type RequestSortBy = (typeof REQUEST_SORT_OPTIONS)[number]

/** Request list item and detail response */
export interface Request {
  id: number
  title: string
  description?: string
  entity_type: string
  requested_entity_id?: number
  status: string
  requester_id: number
  requester_name: string
  fulfiller_id?: number
  fulfiller_name?: string
  vote_score: number
  upvotes: number
  downvotes: number
  wilson_score: number
  fulfilled_at?: string
  /** 1 = upvote, -1 = downvote, null/undefined = no vote */
  user_vote?: number | null
  created_at: string
  updated_at: string
}

/** List response shape */
export interface RequestListResponse {
  requests: Request[]
  total: number
}

/** Helper: get a display label for an entity type */
export function getEntityTypeLabel(entityType: string): string {
  switch (entityType) {
    case 'artist':
      return 'Artist'
    case 'venue':
      return 'Venue'
    case 'show':
      return 'Show'
    case 'release':
      return 'Release'
    case 'label':
      return 'Label'
    case 'festival':
      return 'Festival'
    default:
      return entityType
  }
}

/** Helper: get a display label for a request status */
export function getStatusLabel(status: string): string {
  switch (status) {
    case 'pending':
      return 'Pending'
    case 'in_progress':
      return 'In Progress'
    case 'fulfilled':
      return 'Fulfilled'
    case 'rejected':
      return 'Rejected'
    case 'cancelled':
      return 'Cancelled'
    default:
      return status
  }
}

/** Helper: get a color class for entity type badge */
export function getEntityTypeColor(entityType: string): string {
  switch (entityType) {
    case 'artist':
      return 'bg-purple-500/15 text-purple-700 dark:text-purple-400'
    case 'venue':
      return 'bg-blue-500/15 text-blue-700 dark:text-blue-400'
    case 'show':
      return 'bg-green-500/15 text-green-700 dark:text-green-400'
    case 'release':
      return 'bg-orange-500/15 text-orange-700 dark:text-orange-400'
    case 'label':
      return 'bg-pink-500/15 text-pink-700 dark:text-pink-400'
    case 'festival':
      return 'bg-amber-500/15 text-amber-700 dark:text-amber-400'
    default:
      return 'bg-muted text-muted-foreground'
  }
}

/** Helper: get a color class for status badge */
export function getStatusColor(status: string): string {
  switch (status) {
    case 'pending':
      return 'bg-yellow-500/15 text-yellow-700 dark:text-yellow-400'
    case 'in_progress':
      return 'bg-blue-500/15 text-blue-700 dark:text-blue-400'
    case 'fulfilled':
      return 'bg-green-500/15 text-green-700 dark:text-green-400'
    case 'rejected':
      return 'bg-red-500/15 text-red-700 dark:text-red-400'
    case 'cancelled':
      return 'bg-muted text-muted-foreground'
    default:
      return 'bg-muted text-muted-foreground'
  }
}

/** Helper: format a date string as relative time (e.g., "3 hours ago") */
export function formatTimeAgo(dateString: string): string {
  const date = new Date(dateString)
  const now = new Date()
  const diffMs = now.getTime() - date.getTime()
  const diffSeconds = Math.floor(diffMs / 1000)
  const diffMinutes = Math.floor(diffSeconds / 60)
  const diffHours = Math.floor(diffMinutes / 60)
  const diffDays = Math.floor(diffHours / 24)
  const diffWeeks = Math.floor(diffDays / 7)
  const diffMonths = Math.floor(diffDays / 30)

  if (diffSeconds < 60) return 'just now'
  if (diffMinutes === 1) return '1 minute ago'
  if (diffMinutes < 60) return `${diffMinutes} minutes ago`
  if (diffHours === 1) return '1 hour ago'
  if (diffHours < 24) return `${diffHours} hours ago`
  if (diffDays === 1) return '1 day ago'
  if (diffDays < 7) return `${diffDays} days ago`
  if (diffWeeks === 1) return '1 week ago'
  if (diffWeeks < 5) return `${diffWeeks} weeks ago`
  if (diffMonths === 1) return '1 month ago'
  if (diffMonths < 12) return `${diffMonths} months ago`
  return formatDate(dateString)
}

/** Helper: format a date string as "Jan 5, 2026" */
export function formatDate(dateString: string): string {
  const date = new Date(dateString)
  return date.toLocaleDateString('en-US', {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  })
}

/** Helper: build entity URL from entity type and ID */
export function getEntityUrl(entityType: string, entityId: number): string {
  switch (entityType) {
    case 'artist':
      return `/artists/${entityId}`
    case 'venue':
      return `/venues/${entityId}`
    case 'show':
      return `/shows/${entityId}`
    case 'release':
      return `/releases/${entityId}`
    case 'label':
      return `/labels/${entityId}`
    case 'festival':
      return `/festivals/${entityId}`
    default:
      return '#'
  }
}
