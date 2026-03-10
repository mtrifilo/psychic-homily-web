// Collection types — will be populated when PSY-66 (collection service) ships.
// Placeholder types based on the data model design (PSY-65).

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
  user_id: number
  name: string
  slug: string
  description: string
  entity_type: CollectionEntityType
  is_public: boolean
  is_collaborative: boolean
  item_count: number
  subscriber_count: number
  created_at: string
  updated_at: string
}

export interface CollectionItem {
  id: number
  collection_id: number
  entity_type: CollectionEntityType
  entity_id: number
  position: number
  notes: string
  added_by: number
  created_at: string
}

export interface CollectionSubscriber {
  id: number
  collection_id: number
  user_id: number
  created_at: string
}
