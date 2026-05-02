// Collection types — aligned with backend contracts/collection.go response types.

import type { EntityTag, TagSummary } from '@/features/tags/types'

/**
 * Maximum length, in characters, for collection description and per-item notes.
 * Mirrors backend `contracts.MaxCollectionDescriptionLength` /
 * `MaxCollectionItemNotesLength`, both aliased to `models.MaxCommentBodyLength`.
 * Update both sides together if the comment limit ever changes (PSY-349).
 */
export const MAX_COLLECTION_MARKDOWN_LENGTH = 10000

/**
 * PSY-354: hard cap on tags per collection. Mirrors backend
 * `contracts.MaxCollectionTags`. Used by the picker to disable the
 * autocomplete + show a "limit reached" message rather than letting the
 * user submit and get a 400.
 */
export const MAX_COLLECTION_TAGS = 10

/**
 * PSY-356: minimum thresholds for a public collection to appear in the
 * /collections browse listing. Mirrors backend `services.MinPublicCollection*`
 * constants — keep both sides in sync.
 *
 * - MIN_PUBLIC_COLLECTION_ITEMS: number of items required.
 * - MIN_PUBLIC_COLLECTION_DESCRIPTION_CHARS: minimum CHAR_LENGTH of the raw
 *   description (the banner copy talks about "characters" for clarity).
 */
export const MIN_PUBLIC_COLLECTION_ITEMS = 3
export const MIN_PUBLIC_COLLECTION_DESCRIPTION_CHARS = 50

/**
 * PSY-371: maximum length for `cover_image_url`. The browser address-bar
 * de-facto cap is ~2048 chars; longer URLs almost always indicate
 * pasted-malformed input. We cap on the client to surface the error
 * inline rather than letting the request fail at the backend.
 */
export const MAX_COVER_IMAGE_URL_LENGTH = 2048

/**
 * PSY-371: validate a candidate cover-image URL for the Edit Collection
 * dialog.
 *
 * Returns `null` for valid input (including empty string — empty means
 * "clear the cover" and is always acceptable). Returns a short, human
 * error message otherwise so the form can display it inline.
 *
 * Rules:
 * - empty string → null (clearing is intentional)
 * - must parse as a URL via the WHATWG `URL` constructor
 * - protocol must be `http:` or `https:` (no `data:`, `file:`, `javascript:`)
 * - length must not exceed `MAX_COVER_IMAGE_URL_LENGTH`
 *
 * Server-side sanitization is the source of truth; this is purely UX so
 * curators see the problem before they hit Save.
 */
export function validateCoverImageUrl(value: string): string | null {
  const trimmed = value.trim()
  if (trimmed.length === 0) return null

  if (trimmed.length > MAX_COVER_IMAGE_URL_LENGTH) {
    return `URL is too long (max ${MAX_COVER_IMAGE_URL_LENGTH} characters).`
  }

  let parsed: URL
  try {
    parsed = new URL(trimmed)
  } catch {
    return 'Enter a valid URL starting with http:// or https://.'
  }

  if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
    return 'URL must start with http:// or https://.'
  }

  return null
}

export const COLLECTION_ENTITY_TYPES = [
  'artist',
  'release',
  'label',
  'show',
  'venue',
  'festival',
] as const

export type CollectionEntityType = (typeof COLLECTION_ENTITY_TYPES)[number]

/**
 * Minimal source-collection snapshot returned alongside a forked
 * collection so the frontend can render inline attribution. Always nil
 * when the source has been deleted (FK was set to NULL by ON DELETE
 * SET NULL on the backend). PSY-351.
 */
export interface ForkedFromInfo {
  id: number
  title: string
  slug: string
  creator_id: number
  creator_name: string
}

/**
 * Display mode for a collection.
 * - 'ranked'   → numbered positions, drag-to-reorder
 * - 'unranked' → flat grid/list, no numbers (default)
 */
export const COLLECTION_DISPLAY_MODES = ['ranked', 'unranked'] as const
export type CollectionDisplayMode = (typeof COLLECTION_DISPLAY_MODES)[number]

/**
 * Collection list item (returned by list endpoints, without items array).
 *
 * `description` is the raw markdown source (used to re-populate the editor on
 * Edit). `description_html` is the server-rendered + sanitized HTML produced
 * by goldmark + bluemonday — same allowlist as comments and field notes.
 * Render `description_html` with `dangerouslySetInnerHTML` for display; never
 * render `description` raw (it may contain markdown markers but is safe text).
 */
export interface Collection {
  id: number
  title: string
  slug: string
  description: string
  description_html?: string
  creator_id: number
  creator_name: string
  /**
   * Creator's username, used to link the attribution to /users/:username.
   * Null when the user has not set a username — render the name as plain
   * text in that case (PSY-353).
   */
  creator_username?: string | null
  collaborative: boolean
  cover_image_url?: string | null
  is_public: boolean
  is_featured: boolean
  display_mode: CollectionDisplayMode
  item_count: number
  subscriber_count: number
  contributor_count: number
  /** Public fork count — number of collections that cloned this one. PSY-351. */
  forks_count: number
  /**
   * Set when this collection was created via clone. May be set even when
   * `forked_from` is absent (the source was deleted post-fork). PSY-351.
   */
  forked_from_collection_id?: number | null
  entity_type_counts?: Record<string, number> | null
  /**
   * PSY-350: number of items added to this collection by other users since
   * the viewer's last visit. Only present (>0) for collections the
   * authenticated viewer is subscribed to. Always omitted/zero on public
   * list responses where the viewer has no subscription.
   */
  new_since_last_visit?: number
  /** PSY-352: aggregate count of likes on this collection. */
  like_count: number
  /**
   * PSY-352: whether the authenticated viewer has liked this collection.
   * Always false for anonymous viewers; only meaningfully populated for
   * the public browse list and the detail endpoint.
   */
  user_likes_this?: boolean
  /**
   * PSY-354: tag chips shown on cards. Lightweight TagSummary (no vote
   * counts, no user_vote) — the detail response carries the richer
   * EntityTag for the inline picker. Optional in the type so existing
   * test fixtures don't have to declare it; the server response always
   * includes the field (empty array when no tags).
   */
  tags?: TagSummary[]
  created_at: string
  updated_at: string
}

/** Full collection detail (returned by GET /collections/{slug}) */
export interface CollectionDetail extends Omit<Collection, 'tags'> {
  items: CollectionItem[]
  is_subscribed: boolean
  /**
   * Source-collection snapshot for inline attribution. Absent when the
   * collection wasn't forked OR when the source was deleted. PSY-351.
   */
  forked_from?: ForkedFromInfo | null
  /**
   * PSY-354: full EntityTag list with vote counts + user_vote so the
   * detail page can wire the same EntityTagList component the rest of
   * the entity detail pages use. Optional like Collection.tags so test
   * fixtures don't need to declare it. Differs from Collection.tags
   * (TagSummary) — detail uses EntityTag for the inline picker.
   */
  tags?: EntityTag[]
}

/**
 * PSY-354: response shape for POST /collections/{slug}/tags. Returns the
 * post-mutation tag list so the chip row can update without a follow-up
 * GET.
 */
export interface AddCollectionTagResponse {
  tags: EntityTag[]
}

/**
 * A single item within a collection.
 *
 * `notes` is the raw markdown source. `notes_html` is the server-rendered +
 * sanitized HTML for display. Render via `dangerouslySetInnerHTML`; the
 * sanitizer strips <script>, raw HTML, images, etc. (same policy as comments).
 */
export interface CollectionItem {
  id: number
  entity_type: string
  entity_id: number
  entity_name: string
  entity_slug: string
  position: number
  added_by_user_id: number
  added_by_name: string
  notes?: string | null
  notes_html?: string
  created_at: string
}

/** Collection stats response */
export interface CollectionStats {
  item_count: number
  subscriber_count: number
  contributor_count: number
  entity_type_counts: Record<string, number>
}

/** Helper: build entity URL from entity type and slug */
export function getEntityUrl(entityType: string, entitySlug: string): string {
  switch (entityType) {
    case 'artist':
      return `/artists/${entitySlug}`
    case 'venue':
      return `/venues/${entitySlug}`
    case 'show':
      return `/shows/${entitySlug}`
    case 'release':
      return `/releases/${entitySlug}`
    case 'label':
      return `/labels/${entitySlug}`
    case 'festival':
      return `/festivals/${entitySlug}`
    default:
      return '#'
  }
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
