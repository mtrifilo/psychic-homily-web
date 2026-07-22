// Public API for the charts feature module.
// Other features should import from '@/features/charts', not from internal paths.

// Types
export type {
  ChartWindow,
  ChartScene,
  ChartScenesResponse,
  ChartEntityReference,
  ChartRankEntityType,
  ChartRankModule,
  ChartEntityRank,
  MostActiveArtist,
  MostActiveArtistsResponse,
  OnTheRadioArtist,
  OnTheRadioResponse,
  MostAnticipatedShow,
  MostAnticipatedResponse,
  BusiestVenue,
  BusiestVenuesResponse,
  NewRelease,
  NewReleasesResponse,
  OpenerToWatch,
  OpenersToWatchResponse,
  TopTag,
  TopTagsResponse,
  ChartsSummaryResponse,
  FreshlyAddedItem,
  FreshlyAddedResponse,
  FeaturedCollectionRun,
  FeaturedCollectionResponse,
  FeaturedCollectionHistoryResponse,
} from './types'

export { CHART_WINDOWS } from './types'
export type { RollingChartWindow } from './types'
export { chartEndpoints, chartQueryKeys } from './api'

// Hooks
export {
  useMostActiveArtists,
  useOnTheRadio,
  useMostAnticipated,
  useBusiestVenues,
  useNewReleases,
  useOpenersToWatch,
  useTopTags,
  useChartsSummary,
  useFreshlyAdded,
  useChartScenes,
  useChartEntityRank,
  useFeaturedCollection,
  useFeaturedCollectionHistory,
} from './hooks'

// Components
export {
  ChartsPage,
  ChartDrilldownPage,
  EntityChartRankBadge,
  ArchiveMasthead,
  FeaturedCollectionCard,
} from './components'
export {
  CHART_MODULE_CONFIG,
  CHART_MODULE_SLUGS,
  isChartModuleSlug,
} from './moduleConfig'
export type { ChartModuleSlug } from './moduleConfig'
