// Public API for the collections feature module.
// Other features should import from '@/features/collections', not from internal paths.

export type {
  Collection,
  CollectionItem,
  CollectionSubscriber,
  CollectionEntityType,
} from './types'

export { COLLECTION_ENTITY_TYPES } from './types'

// Re-export hooks and components as they are built:
// export * from './hooks'
// export * from './components'
