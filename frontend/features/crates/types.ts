// Crate types — aligned with backend contracts/crate.go response types.

export const CRATE_ENTITY_TYPES = [
  'artist',
  'release',
  'label',
  'show',
  'venue',
  'festival',
] as const

export type CrateEntityType = (typeof CRATE_ENTITY_TYPES)[number]

/** Crate list item (returned by list endpoints, without items array) */
export interface Crate {
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
  item_count: number
  subscriber_count: number
  contributor_count: number
  created_at: string
  updated_at: string
}

/** Full crate detail (returned by GET /crates/{slug}) */
export interface CrateDetail extends Crate {
  items: CrateItem[]
  is_subscribed: boolean
}

/** A single item within a crate */
export interface CrateItem {
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

/** Crate stats response */
export interface CrateStats {
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
