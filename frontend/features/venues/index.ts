// Public API for the venues feature module

// API (endpoints + query keys)
export { venueEndpoints, venueQueryKeys } from './api'

// Types
export type {
  Venue,
  VenueSearchResponse,
  VenueWithShowCount,
  VenuesListResponse,
  VenueShow,
  VenueShowsResponse,
  VenueShowYearCount,
  VenueShowYearsResponse,
  VenueCity,
  VenueCitiesResponse,
  VenueEditRequest,
  UnverifiedVenue,
  UnverifiedVenuesResponse,
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
  useVenueShowYears,
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
} from './components'
