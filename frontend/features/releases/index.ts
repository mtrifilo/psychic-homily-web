// Public API for the releases feature module

// API (endpoints + query keys)
export { releaseEndpoints, releaseQueryKeys } from './api'

// Types
export type {
  ReleaseType,
  ReleaseSortOption,
  ReleaseArtist,
  ReleaseExternalLink,
  // TYPE only. The component of the same name is not exported anywhere — see
  // the note further down and in ./components/index.ts.
  ReleaseDetail,
  ReleaseListArtist,
  ReleaseListItem,
  ReleasesListResponse,
  ArtistReleaseListItem,
  ArtistReleasesResponse,
  SavedReleaseResponse,
  SavedReleasesListResponse,
  ReleaseSaveResponse,
  ReleaseSaveCount,
  ReleaseSaveCountEntry,
  BatchReleaseSaveCountsResponse,
} from './types'

export {
  RELEASE_TYPES,
  RELEASE_TYPE_LABELS,
  RELEASE_SORT_OPTIONS,
  getReleaseTypeLabel,
} from './types'

// Hooks
export {
  useReleases,
  useRelease,
  useArtistReleases,
  useSavedReleases,
  useReleaseSaveCount,
  useReleaseSaveCountBatch,
  useReleaseSaveToggle,
} from './hooks'

// Components
export { ReleaseCard, ReleaseList } from './components'
// Note: the ReleaseDetail COMPONENT is barrel-exported from nowhere — import
// '@/features/releases/components/ReleaseDetail' directly. See the note in
// ./components/index.ts.
// The TYPE ReleaseDetail is available from '@/features/releases/types'.
