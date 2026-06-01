// Request types — aligned with backend contracts/request.go response types.

import { formatTimeAgo as sharedFormatTimeAgo } from '@/lib/formatTimeAgo'

export const REQUEST_ENTITY_TYPES = [
  'artist',
  'release',
  'label',
  'show',
  'venue',
  'festival',
] as const

export const REQUEST_STATUSES = [
  'pending',
  'in_progress',
  'pending_fulfillment',
  'fulfilled',
  'rejected',
  'cancelled',
] as const

export const REQUEST_SORT_OPTIONS = ['votes', 'newest', 'oldest'] as const

/** Request list item and detail response */
export interface Request {
  id: number
  title: string
  description?: string
  entity_type: string
  requested_entity_id?: number
  /**
   * PSY-917: slug + display name of the entity `requested_entity_id` points
   * at, resolved server-side on the single-request detail fetch (null on
   * list rows). After a fulfillment is proposed with an entity, this is the
   * PROPOSED entity. Entity detail pages route by slug, so the slug — not the
   * numeric id — is what builds a working "View proposed {entity}" link. Slug
   * is nullable because catalog rows can lack a slug; when null, suppress the
   * link. Name is the label for the link text.
   */
  requested_entity_slug?: string | null
  requested_entity_name?: string | null
  status: string
  requester_id: number
  requester_name: string
  /**
   * PSY-619: requester's username when set, null otherwise. Pass to
   * `<UserAttribution username={...} />` to render the byline as a link to
   * /users/:username when non-null; null renders plain text.
   */
  requester_username: string | null
  fulfiller_id?: number
  fulfiller_name?: string
  fulfiller_username?: string | null
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

/**
 * Helper: the indefinite article ("a"/"an") for an entity type's label, so UI
 * copy reads "Propose an artist" / "Propose a venue" instead of the
 * always-"a" version. Of the six request entity types only "artist" starts
 * with a vowel sound, but we check the resolved label's first letter so the
 * helper stays correct if the label set grows. PSY-917.
 */
export function getEntityTypeArticle(entityType: string): 'a' | 'an' {
  const first = getEntityTypeLabel(entityType).charAt(0).toLowerCase()
  return 'aeiou'.includes(first) ? 'an' : 'a'
}

/** Helper: get a display label for a request status */
export function getStatusLabel(status: string): string {
  switch (status) {
    case 'pending':
      return 'Pending'
    case 'in_progress':
      return 'In Progress'
    case 'pending_fulfillment':
      return 'Pending review'
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
    case 'pending_fulfillment':
      return 'bg-primary/15 text-primary'
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

/**
 * Helper: format a date string as relative time (e.g., "3 hours ago").
 *
 * Re-exports the canonical `formatTimeAgo` (PSY-780) — the months-aware
 * implementation that previously lived only in this file. Past ~12 months
 * the shared helper falls back to the same `month: short, day, year`
 * format as `formatDate`, so the visible output for callers is unchanged.
 */
export const formatTimeAgo = sharedFormatTimeAgo

/** Helper: format a date string as "Jan 5, 2026" */
export function formatDate(dateString: string): string {
  const date = new Date(dateString)
  return date.toLocaleDateString('en-US', {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  })
}

/**
 * Helper: build an entity URL from entity type and ID.
 *
 * @deprecated PSY-917 — entity detail pages route by SLUG, not numeric ID, so
 * a `/artists/<id>` URL doesn't resolve (the backend looks rows up by slug).
 * Use {@link getEntityUrlBySlug} with the resolved `requested_entity_slug`
 * instead. Retained only so the historical signature stays covered; not used
 * for any live link.
 */
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

/**
 * Helper: build an entity detail URL from entity type and SLUG.
 *
 * This is the correct way to link to a knowledge-graph entity — detail pages
 * are `/<type>/[slug]` routes resolved against the slug column. Returns null
 * for an unknown type or an empty slug so callers can suppress the link
 * rather than emit a dead `#` href. PSY-917.
 */
export function getEntityUrlBySlug(
  entityType: string,
  slug: string | null | undefined
): string | null {
  if (!slug) return null
  switch (entityType) {
    case 'artist':
      return `/artists/${slug}`
    case 'venue':
      return `/venues/${slug}`
    case 'show':
      return `/shows/${slug}`
    case 'release':
      return `/releases/${slug}`
    case 'label':
      return `/labels/${slug}`
    case 'festival':
      return `/festivals/${slug}`
    default:
      return null
  }
}
