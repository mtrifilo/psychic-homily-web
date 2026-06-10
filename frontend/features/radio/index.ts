// Public API for the radio feature module

// API (endpoints + query keys)
export { radioEndpoints, radioQueryKeys } from './api'

// Types
export type {
  RadioStationListItem,
  RadioStationDetail,
  RadioStationsListResponse,
  RadioNetworkInfo,
  RadioSiblingStation,
  RadioShowListItem,
  RadioShowDetail,
  RadioShowsListResponse,
  RadioEpisodeListItem,
  RadioEpisodeDetail,
  RadioEpisodesListResponse,
  RadioPlay,
  RadioTopArtist,
  RadioTopLabel,
  RadioAsHeardOn,
  RadioNewReleaseRadarEntry,
  RadioStats,
  RadioTopArtistsResponse,
  RadioTopLabelsResponse,
  RadioAsHeardOnResponse,
  RadioNewReleasesResponse,
  // PSY-1048 aggregation shapes (PSY-1049)
  RadioEpisodePreviewArtist,
  RadioStationEpisodeRow,
  RadioRecentEpisodesResponse,
} from './types'

export {
  BROADCAST_TYPE_LABELS,
  ROTATION_STATUS_LABELS,
  ROTATION_STATUS_COLORS,
  getBroadcastTypeLabel,
  getRotationStatusLabel,
  getRotationStatusColor,
  isStationVisibleOnIndex,
  getStationDetailUrl,
} from './types'

// Hooks
export {
  useRadioStations,
  useRadioStation,
  useRadioShows,
  useRadioShow,
  useRadioEpisodes,
  useRadioEpisode,
  useRadioTopArtists,
  useRadioTopLabels,
  useArtistRadioPlays,
  useReleaseRadioPlays,
  useNewReleaseRadar,
  useRadioStats,
  // PSY-1016
  useShowLatestEpisode,
  useStationOverview,
  // PSY-1049
  useRecentRadioEpisodes,
} from './hooks'

// Components
export {
  RadioStationCard,
  RadioShowCard,
  RadioEpisodeRow,
  RadioPlayRow,
  AsHeardOn,
  NetworkTabBar,
  // PSY-1016
  RadioPanel,
  RadioStationList,
  RadioStationOverview,
  RecentShowRow,
  ArtistHops,
} from './components'

// PSY-1016: station-overview derivation helpers
export {
  pickNowPlayingShow,
  orderRecentShows,
  recentArtistsFromEpisode,
  deriveNowPlaying,
  formatShortAirDate,
  formatStationLocation,
} from './lib/stationOverview'
export type { ArtistHop, NowPlaying } from './lib/stationOverview'
