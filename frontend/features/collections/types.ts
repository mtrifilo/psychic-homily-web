// Collection types — matches backend response types from services/contracts/collection.go

export const COLLECTION_ENTITY_TYPES = [
  'artist',
  'release',
  'label',
  'show',
  'venue',
  'festival',
] as const

export type CollectionEntityType = (typeof COLLECTION_ENTITY_TYPES)[number]

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
  item_count: number
  subscriber_count: number
  contributor_count: number
  created_at: string
  updated_at: string
}

export interface CollectionDetail extends Collection {
  items: CollectionItem[]
  is_subscribed: boolean
}

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

export interface CollectionStats {
  item_count: number
  subscriber_count: number
  contributor_count: number
  entity_type_counts: Record<string, number>
}
