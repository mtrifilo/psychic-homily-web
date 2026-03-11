// Public API for the festivals feature module.
// Other features should import from this file only.

// Types
export type {
  FestivalStatus,
  BillingTier,
  FestivalSocial,
  FestivalDetail,
  FestivalListItem,
  FestivalsListResponse,
  FestivalArtist,
  FestivalArtistsResponse,
  FestivalVenue,
  FestivalVenuesResponse,
  ArtistFestivalListItem,
  ArtistFestival,
  ArtistFestivalsResponse,
} from './types'

export {
  FESTIVAL_STATUS_LABELS,
  FESTIVAL_STATUSES,
  getFestivalStatusVariant,
  getFestivalStatusLabel,
  BILLING_TIER_LABELS,
  BILLING_TIERS,
  BILLING_TIER_ORDER,
  getBillingTierLabel,
  formatFestivalLocation,
  formatFestivalDateRange,
  formatFestivalDates,
} from './types'

// Hooks
export {
  useFestivals,
  useFestival,
  useFestivalArtists,
  useFestivalLineup,
  useFestivalVenues,
  useArtistFestivals,
} from './hooks'

// Components
// Note: FestivalDetail component is not re-exported here to avoid
// name collision with FestivalDetail type. Import it directly from
// '@/features/festivals/components' when needed.
export { FestivalCard, FestivalLineup, FestivalList } from './components'
