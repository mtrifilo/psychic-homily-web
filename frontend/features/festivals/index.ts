// Public API for the festivals feature module.
// Other features should import from this file only.

// API (endpoints + query keys)
export { festivalEndpoints, festivalQueryKeys } from './api'

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
  // Intelligence types
  FestivalSummary,
  ArtistSummary,
  SharedArtist,
  SimilarFestival,
  SimilarFestivalsResponse,
  FestivalOverlap,
  TrajectoryEntry,
  ArtistBreakout,
  ArtistMilestone,
  FestivalBreakouts,
  ArtistTrajectory,
  SeriesEdition,
  ReturningArtist,
  SeriesNewcomer,
  SeriesComparison,
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
  getTierBarWidth,
  getMilestoneLabel,
} from './types'

// Hooks
export {
  useFestivals,
  useFestival,
  useFestivalArtists,
  useFestivalLineup,
  useFestivalVenues,
  useArtistFestivals,
  useSimilarFestivals,
  useFestivalBreakouts,
  useArtistFestivalTrajectory,
  useSeriesComparison,
} from './hooks'

// Components
// Note: FestivalDetail component is not re-exported here to avoid
// name collision with FestivalDetail type. Import it directly from
// '@/features/festivals/components' when needed.
export { FestivalCard, FestivalLineup, FestivalList } from './components'
