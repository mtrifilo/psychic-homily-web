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
} from './hooks'

// Components
export { ReleaseCard, ReleaseList } from './components'
// Note: ReleaseDetail component is exported from './components' directly.
// Import it as: import { ReleaseDetail } from '@/features/releases/components'
// The type ReleaseDetail is available from '@/features/releases/types'
