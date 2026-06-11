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
  // PSY-1048 aggregation shapes (PSY-1049/1050/1051)
  RadioEpisodePreviewArtist,
  RadioStationEpisodeRow,
  RadioRecentEpisodesResponse,
  RadioStationEpisodesResponse,
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
  // PSY-1016 (consumed by the Dial strips since PSY-1049)
  useStationOverview,
  // PSY-1049
  useRecentRadioEpisodes,
  // PSY-1050: station-page aggregations (PSY-1048 endpoints)
  useStationEpisodes,
  useStationTopArtists,
  useStationTopLabels,
} from './hooks'

// Components
export {
  RadioStationCard,
  AsHeardOn,
  NetworkTabBar,
  ArtistHops,
  // PSY-1050: station-page rebuild (The Dial, Option A)
  StationOnAirBox,
  StationPlaylistsFeed,
  StationShowsDirectory,
  StationSidebar,
} from './components'

// PSY-1016: station-overview derivation helpers
export {
  pickNowPlayingShow,
  formatShortAirDate,
  formatStationLocation,
} from './lib/stationOverview'
export type { ArtistHop, NowPlaying } from './lib/stationOverview'

// PSY-1051: episode-archive derivation helpers + neighbors hook
// (RadioEpisodePreviewArtist is re-exported in the types block above)
export { useEpisodeNeighbors } from './hooks'
export {
  isAirDateToday,
  previewToHops,
  computeArtistMatchStats,
  formatPlayTime,
  formatTimeOfDay,
  formatDurationMinutes,
  formatArchiveDate,
  formatShortNavDate,
  walkEpisodeNeighbors,
} from './lib/episodeArchive'
export type { ArtistMatchStats, EpisodeNeighbors } from './lib/episodeArchive'
