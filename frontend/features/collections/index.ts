// Public API for the collections feature module.
// Other features should import from '@/features/collections', not from internal paths.

export type {
  Collection,
  CollectionDetail,
  CollectionItem,
  CollectionStats,
  CollectionEntityType,
} from './types'

export { COLLECTION_ENTITY_TYPES } from './types'

export type { CollectionListParams } from './hooks'

export {
  useCollections,
  useMyCollections,
  useCollection,
  useCollectionStats,
  useSetFeatured,
  useDeleteCollection,
  useAddCollectionItem,
  useRemoveCollectionItem,
  useReorderCollectionItems,
  useUpdateCollectionItem,
  useEntityCollections,
  useUserPublicCollections,
} from './hooks'

export { EntityCollections, UserCollections } from './components'
