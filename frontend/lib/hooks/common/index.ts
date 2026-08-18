export { useCommandPalette, openCommandPalette } from './useCommandPalette'

export { useHydrated } from './useHydrated'

// `usePreAttachImageFailureRef` is deliberately NOT re-exported here. Its
// callers are leaf presentational components on image-heavy routes, and this
// barrel also re-exports the react-query-backed follow / revisions / search
// hooks — importing it would pull that graph into every route they reach.
// Import the hook file directly.

export { useGeoDefaultScene } from './useGeoDefaultScene'

export { type Density, useDensity } from './useDensity'

export { useFilterNavigation } from './useFilterNavigation'
export { usePrefetchRoutes } from './usePrefetchRoutes'

export {
  useAutoDismissBanner,
  useAutoDismissFlag,
} from './useAutoDismissBanner'

export { useDismissTimer } from './useDismissTimer'

export {
  useEntityRevisions,
  useRevision,
  useUserRevisions,
  useRollbackRevision,
  type RevisionItem,
  type FieldChange,
} from './useRevisions'

export {
  useFollowStatus,
  useBatchFollowStatus,
  useFollow,
  useUnfollow,
  useMyFollowing,
  useAllMyFollowing,
  useLibraryFollowing,
  useLibraryFollowingCounts,
} from './useFollow'

export {
  useUserFollowStatus,
  useUserFollow,
  useUserUnfollow,
} from './useUserFollow'

export {
  useEntitySearch,
  type EntitySearchResult,
  type EntitySearchResults,
} from './useEntitySearch'
