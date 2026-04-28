// Collection types — aligned with backend contracts/collection.go response types.

/**
 * Maximum length, in characters, for collection description and per-item notes.
 * Mirrors backend `contracts.MaxCollectionDescriptionLength` /
 * `MaxCollectionItemNotesLength`, both aliased to `models.MaxCommentBodyLength`.
 * Update both sides together if the comment limit ever changes (PSY-349).
 */
export const MAX_COLLECTION_MARKDOWN_LENGTH = 10000

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
  collaborative: boolean
  cover_image_url?: string | null
  is_public: boolean
  is_featured: boolean
  item_count: number
  subscriber_count: number
  contributor_count: number
  entity_type_counts?: Record<string, number> | null
  created_at: string
  updated_at: string
}

/** Full collection detail (returned by GET /collections/{slug}) */
export interface CollectionDetail extends Collection {
  items: CollectionItem[]
  is_subscribed: boolean
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
