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
  // PSY-1022 now-playing shapes
  RadioNowPlaying,
  RadioNowPlayingShowRef,
  RadioNowPlayingTrack,
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
  // PSY-1022: live now-playing (with latest-archive fallback)
  useStationNowPlaying,
  // PSY-1049
  useRecentRadioEpisodes,
  // PSY-1050: station-page aggregations (PSY-1048 endpoints)
  useStationEpisodes,
  useStationTopArtists,
  useStationTopLabels,
  // PSY-1299: station co-occurrence graph
  useStationGraph,
} from './hooks'

// Components
export {
  RadioStationCard,
  AsHeardOn,
  NetworkTabBar,
  ArtistHops,
  // PSY-1298/1306: shared date + viewer-local time cell + its text composer
  AirDateCellContent,
  // PSY-1050: station-page rebuild (The Dial, Option A)
  StationOnAirBox,
  StationPlaylistsFeed,
  StationShowsDirectory,
  StationSidebar,
  // PSY-1299: station co-occurrence graph
  StationGraph,
} from './components'
export { airDateCellText } from './components/AirDateCell'

// PSY-1016: station-overview derivation helpers
// (pickNowPlayingShow stays un-exported here — only useStationOverview
// consumes it; PSY-1075 narrowed the public surface. formatShortAirDate and
// the PSY-1298 viewer-local helpers stay OFF the barrel — external surfaces
// consume the rendering via AirDateCellContent, and in-feature consumers
// import relatively.)
export { formatStationLocation } from './lib/stationOverview'
export type { ArtistHop } from './lib/stationOverview'

// PSY-1051: episode-archive derivation helpers + neighbors hook
// (RadioEpisodePreviewArtist is re-exported in the types block above)
export { useEpisodeNeighbors } from './hooks'
export {
  isLiveNow,
  previewToHops,
  computeArtistMatchStats,
  formatPlayTime,
  formatTimeOfDay,
  formatDurationMinutes,
  // PSY-1306: viewer-local "aired ..." body + window-aware verb for the
  // playlist detail page. The retired station-dated formatters' surfaces
  // (archive table, episode nav) now compose from airDateCellText; the
  // deliberate station-dated holdouts are the detail H1 + SSR metadata
  // (URL-keyed) and admin surfaces.
  formatViewerAiredLine,
  airedVerbForWindow,
  walkEpisodeNeighbors,
} from './lib/episodeArchive'
export type { ArtistMatchStats, EpisodeNeighbors } from './lib/episodeArchive'

// PSY-1076: New Release Radar link resolution (hub box + /radio/new-releases)
export { getNewReleaseHref } from './types'
