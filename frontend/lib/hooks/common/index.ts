export { useCommandPalette, openCommandPalette } from './useCommandPalette'

export { useHydrated } from './useHydrated'

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

// NOT exported here on purpose: `useAuthGatedAction` and `useAuthRouteGuard`.
// Their consumers are auth-gated leaf controls and route guards, and this
// barrel re-exports the react-query-backed follow / revisions / search hooks
// above, so importing either through it would pull that graph into every route
// they reach. Same reasoning as the note below. Import the hook files
// directly.

// NOT exported here on purpose: `usePreAttachImageFailureRef`. Its callers are
// leaf image components, and this barrel also re-exports the react-query-backed
// follow / revisions / search hooks above, so importing it would pull that
// graph into every route they reach. Precautionary rather than measured, by
// analogy with the Turbopack barrel note in
// features/collections/components/index.ts. Import the hook file directly.
