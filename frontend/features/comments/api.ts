/**
 * Comments API Configuration
 *
 * Co-located endpoint definitions and query keys for the comments feature.
 */

import { API_BASE_URL } from '@/lib/api-base'

// ============================================================================
// Endpoints
// ============================================================================

export const commentEndpoints = {
  LIST: (entityType: string, entityId: number) =>
    `${API_BASE_URL}/entities/${entityType}/${entityId}/comments`,
  CREATE: (entityType: string, entityId: number) =>
    `${API_BASE_URL}/entities/${entityType}/${entityId}/comments`,
  REPLY: (commentId: number) =>
    `${API_BASE_URL}/comments/${commentId}/replies`,
  UPDATE: (commentId: number) =>
    `${API_BASE_URL}/comments/${commentId}`,
  DELETE: (commentId: number) =>
    `${API_BASE_URL}/comments/${commentId}`,
  VOTE: (commentId: number) =>
    `${API_BASE_URL}/comments/${commentId}/vote`,
  THREAD: (commentId: number) =>
    `${API_BASE_URL}/comments/${commentId}/thread`,
  // PSY-1512: single-comment fetch, used to resolve a `#comment-{id}`
  // deep link (from notifications/emails) to its thread root.
  SINGLE: (commentId: number) =>
    `${API_BASE_URL}/comments/${commentId}`,
  // PSY-296: owner-only reply-permission toggle.
  REPLY_PERMISSION: (commentId: number) =>
    `${API_BASE_URL}/comments/${commentId}/reply-permission`,
} as const

// PSY-296: preferences endpoint for default reply permission.
export const commentPreferencesEndpoints = {
  DEFAULT_REPLY_PERMISSION: `${API_BASE_URL}/auth/preferences/default-reply-permission`,
} as const

export const fieldNoteEndpoints = {
  LIST: (showId: number) =>
    `${API_BASE_URL}/shows/${showId}/field-notes`,
  CREATE: (showId: number) =>
    `${API_BASE_URL}/shows/${showId}/field-notes`,
  // PSY-1590: the venue ROLLUP — the notes written about shows held at a
  // venue. Venues own no field notes of their own (`CreateFieldNote` writes
  // entity_type='show'), so there is no CREATE twin here and there never will
  // be; this is a read of show-scoped rows from a different direction.
  //
  // Numeric id only, like CONFIRM and unlike the id-or-slug venue reads — the
  // backend refuses a slug at this path (see ListVenueFieldNotesRequest).
  LIST_FOR_VENUE: (venueId: number) =>
    `${API_BASE_URL}/venues/${venueId}/field-notes`,
} as const

// ============================================================================
// Query Keys
// ============================================================================

export const commentQueryKeys = {
  all: ['comments'] as const,
  entity: (entityType: string, entityId: number) =>
    ['comments', entityType, entityId] as const,
  thread: (commentId: number) =>
    ['comments', 'thread', commentId] as const,
  // PSY-1512: single-comment fetch for deep-link resolution.
  single: (commentId: number) =>
    ['comments', 'single', commentId] as const,
} as const

export const fieldNoteQueryKeys = {
  all: ['field-notes'] as const,
  show: (showId: number) =>
    ['field-notes', showId] as const,
  // Keyed on the limit as well as the venue (PSY-1698's rule): the Atlas
  // teaser asks for ONE note, and a future surface asking for a page of them
  // is a different question that must not be answered by whichever request
  // happened to land first.
  venue: (venueId: number, limit: number) =>
    ['field-notes', 'venue', venueId, limit] as const,
} as const

/**
 * Notes fetched for the Atlas venue panel's teaser (PSY-1590).
 *
 * The teaser quotes ONE note, so this is a candidate pool rather than a page
 * size. `pickVenueFieldNoteForTeaser` skips notes it must not quote — setlist
 * spoilers, and notes whose show can no longer be named — and fetching only
 * the single best-ranked note would let one such note hide a section the venue
 * has perfectly good notes for.
 *
 * Five is a deliberate small number: it is enough that a run of skips is
 * unlikely to exhaust it, and small enough that the panel is not paying for
 * rows it will never render. The count shown beside the quote comes from the
 * response's `total`, which spans the whole venue, so this bound never
 * influences what the reader is told.
 */
export const VENUE_FIELD_NOTE_TEASER_LIMIT = 5
