// Public API for the charts feature module.
// Other features should import from '@/features/charts', not from internal paths.

// Types
export type {
  ChartWindow,
  ChartScene,
  ChartScenesResponse,
  ChartEntityReference,
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
  ChartsSummaryResponse,
  FreshlyAddedItem,
  FreshlyAddedResponse,
} from './types'

export { CHART_WINDOWS } from './types'
export { chartEndpoints, chartQueryKeys } from './api'

// Hooks
export {
  useMostActiveArtists,
  useOnTheRadio,
  useMostAnticipated,
  useBusiestVenues,
  useNewReleases,
  useOpenersToWatch,
  useChartsSummary,
  useFreshlyAdded,
  useChartScenes,
} from './hooks'

// Components
export { ChartsPage, ChartDrilldownPage } from './components'
export {
  CHART_MODULE_CONFIG,
  CHART_MODULE_SLUGS,
  isChartModuleSlug,
} from './moduleConfig'
export type { ChartModuleSlug } from './moduleConfig'
