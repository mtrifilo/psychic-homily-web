// Public API for the artists feature module

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

// Components
export {
  ArtistCard,
  ArtistSearch,
  ArtistDetail,
  ArtistList,
  ArtistListSkeleton,
  ArtistShowsList,
  ReportArtistButton,
  ReportArtistDialog,
} from './components'
