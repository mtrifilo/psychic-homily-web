// Request types — aligned with backend contracts/request.go response types.

import { formatTimeAgo as sharedFormatTimeAgo } from '@/lib/formatTimeAgo'
import { getEntityTypeBadgeClasses } from '@/components/shared/EntityTypeBadge'

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

/**
 * Helper: get a color class for an entity-type badge.
 *
 * Delegates to the single-sourced DS-palette map shared with the admin
 * collections table and any future entity-type pill (PSY-943). The request
 * board renders its own `<span>` without a `border` utility, so the
 * `border-chart-*` slice the helper returns is inert here — harmless, and it
 * keeps the map in exactly one place.
 */
export function getEntityTypeColor(entityType: string): string {
  return getEntityTypeBadgeClasses(entityType)
}

/**
 * Helper: get a color class for a request status badge (PSY-943).
 *
 * Bound to the DS categorical palette (`--chart-*`, globals.css) instead of
 * raw Tailwind hues. Status semantics drive the hue choice:
 *   pending             → chart-3 (gold)  — waiting/attention
 *   in_progress         → chart-6 (denim) — active work
 *   pending_fulfillment → primary         — needs the owner's review (matches
 *                                            the app's primary call-to-action)
 *   fulfilled           → chart-2 (green) — done
 *   rejected            → destructive     — terminal-negative
 *   cancelled / unknown → muted           — inert
 */
export function getStatusColor(status: string): string {
  switch (status) {
    case 'pending':
      return 'bg-chart-3/15 text-chart-3'
    case 'in_progress':
      return 'bg-chart-6/15 text-chart-6'
    case 'pending_fulfillment':
      return 'bg-primary/15 text-primary'
    case 'fulfilled':
      return 'bg-chart-2/15 text-chart-2'
    case 'rejected':
      return 'bg-destructive/15 text-destructive'
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
