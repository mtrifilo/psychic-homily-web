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
  ArtistSearchParams,
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
  useArtistRelationshipVote,
  useCreateArtistRelationship,
} from './hooks'

// Components
export {
  ArtistCard,
  ArtistSearch,
  ArtistDetail,
  ArtistList,
  ArtistListSkeleton,
  ArtistShowsList,
  RelatedArtists,
  ArtistGraphVisualization,
  ReportArtistButton,
  ReportArtistDialog,
} from './components'
