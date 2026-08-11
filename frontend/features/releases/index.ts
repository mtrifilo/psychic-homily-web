// Public API for the releases feature module

// API (endpoints + query keys)
export { releaseEndpoints, releaseQueryKeys } from './api'

// Types
export type {
  ReleaseType,
  ReleaseSortOption,
  ReleaseArtist,
  ReleaseExternalLink,
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
// Note: the ReleaseDetail COMPONENT is barrel-exported from nowhere (PSY-1772).
// Its only consumer, `app/releases/[slug]/page.tsx`, imports it via `dynamic()`
// straight from '@/features/releases/components/ReleaseDetail' so Turbopack
// keeps it out of the global shared client chunk. Any new consumer must import
// the file directly too — re-adding a barrel export re-hoists it.
// The TYPE ReleaseDetail is available from '@/features/releases/types'.
