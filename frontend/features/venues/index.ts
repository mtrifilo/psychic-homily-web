// Public API for the venues feature module

// API (endpoints + query keys)
export { venueEndpoints, venueQueryKeys } from './api'

// Types
export type {
  Venue,
  VenueSearchParams,
  VenueSearchResponse,
  VenueWithShowCount,
  VenuesListResponse,
  VenueShow,
  VenueShowsResponse,
  VenueCity,
  VenueCitiesResponse,
  VenueEditRequest,
  UnverifiedVenue,
  UnverifiedVenuesResponse,
  FavoriteVenueResponse,
  FavoriteVenuesListResponse,
  CheckFavoritedResponse,
  FavoriteVenueActionResponse,
  FavoriteVenueShow,
  FavoriteVenueShowsResponse,
  VenueGenreCount,
  VenueGenreResponse,
} from './types'

export { getVenueLocation } from './types'

// Hooks
export {
  useVenues,
  useVenue,
  type TimeFilter,
  useVenueShows,
  useVenueCities,
  useVenueGenres,
} from './hooks'

export { useVenueSearch } from './hooks'

export {
  useVenueUpdate,
  useVenueDelete,
} from './hooks'

// Components
export {
  VenueCard,
  VenueSearch,
  VenueDetail,
  VenueList,
  VenueLocationCard,
  VenueShowsList,
  DeleteVenueDialog,
  VenueDeniedDialog,
  FavoriteVenueButton,
  FavoriteVenuesTab,
} from './components'
