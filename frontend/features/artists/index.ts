// Public API for the artists feature module

// API (endpoints + query keys)
export { artistEndpoints, artistQueryKeys } from './api'

// Types
export type {
  ArtistSocial,
  Artist,
  ArtistEditRequest,
  ArtistCity,
  ArtistCitiesResponse,
  ArtistListItem,
  ArtistsListResponse,
  ArtistSearchResponse,
  ArtistShowVenue,
  ArtistShowArtist,
  ArtistShow,
  ArtistShowsResponse,
  ArtistTimeFilter,
  ArtistReportType,
  ArtistReportStatus,
  ArtistReportArtistInfo,
  ArtistReportResponse,
  CreateArtistReportRequest,
  MyArtistReportResponse,
  ArtistReportsListResponse,
  ArtistAlias,
  ArtistAliasesResponse,
  MergeArtistResult,
  ArtistGraph,
  ArtistGraphNode,
  ArtistGraphLink,
} from './types'

export { getArtistLocation } from './types'

// Hooks
export {
  useArtists,
  useArtistCities,
  useArtist,
  useArtistShows,
} from './hooks'

export { useArtistSearch } from './hooks'

export {
  useMyArtistReport,
  useReportArtist,
} from './hooks'

export {
  useArtistGraph,
  useFetchArtistGraph,
  useArtistRelationshipVote,
  useCreateArtistRelationship,
} from './hooks'

export { useReducedMotion } from './hooks'
export { useArtistGraphCard } from './hooks'

// Components
// NOTE: ArtistDetail is intentionally omitted (PSY-950). The route page imports it
// directly via `dynamic()` from '@/features/artists/components/ArtistDetail' so
// Turbopack evicts it from the global shared client chunk (loaded on every route).
// Re-adding it here re-hoists ArtistDetail.tsx (~40 KB) into the shared chunk.
export {
  ArtistCard,
  ArtistSearch,
  ArtistList,
  ArtistListSkeleton,
  ArtistShowsList,
  ArtistSimilarSidebar,
  ArtistGraphDialog,
  ArtistGraphVisualization,
  ReportArtistButton,
  ReportArtistDialog,
} from './components'

export type { ArtistGraphSelection } from './components'
