// Collection types — aligned with backend contracts/collection.go response types.

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
 * Display mode for a collection.
 * - 'ranked'   → numbered positions, drag-to-reorder
 * - 'unranked' → flat grid/list, no numbers (default)
 */
export const COLLECTION_DISPLAY_MODES = ['ranked', 'unranked'] as const
export type CollectionDisplayMode = (typeof COLLECTION_DISPLAY_MODES)[number]

/** Collection list item (returned by list endpoints, without items array) */
export interface Collection {
  id: number
  title: string
  slug: string
  description: string
  creator_id: number
  creator_name: string
  collaborative: boolean
  cover_image_url?: string | null
  is_public: boolean
  is_featured: boolean
  display_mode: CollectionDisplayMode
  item_count: number
  subscriber_count: number
  contributor_count: number
  entity_type_counts?: Record<string, number> | null
  /**
   * PSY-350: number of items added to this collection by other users since
   * the viewer's last visit. Only present (>0) for collections the
   * authenticated viewer is subscribed to. Always omitted/zero on public
   * list responses where the viewer has no subscription.
   */
  new_since_last_visit?: number
  created_at: string
  updated_at: string
}

/** Full collection detail (returned by GET /collections/{slug}) */
export interface CollectionDetail extends Collection {
  items: CollectionItem[]
  is_subscribed: boolean
}

/** A single item within a collection */
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
