/**
 * Radio API Configuration
 *
 * Co-located endpoint definitions and query keys for the radio feature.
 * Imported by radio hooks and re-exported from lib/queryClient.ts.
 */

import { API_BASE_URL } from '@/lib/api-base'

// ============================================================================
// Endpoints
// ============================================================================

export const radioEndpoints = {
  // Stations
  STATIONS: `${API_BASE_URL}/radio-stations`,
  STATION: (slug: string) => `${API_BASE_URL}/radio-stations/${slug}`,
  // Station-scoped aggregations (PSY-1048; consumed by the PSY-1050 station page)
  STATION_EPISODES: (slug: string) =>
    `${API_BASE_URL}/radio-stations/${slug}/episodes`,
  // PSY-1022: live now-playing (with latest-archive fallback)
  STATION_NOW_PLAYING: (slug: string) =>
    `${API_BASE_URL}/radio-stations/${slug}/now-playing`,
  STATION_TOP_ARTISTS: (slug: string) =>
    `${API_BASE_URL}/radio-stations/${slug}/top-artists`,
  STATION_TOP_LABELS: (slug: string) =>
    `${API_BASE_URL}/radio-stations/${slug}/top-labels`,
  // PSY-1299: within-station co-occurrence subgraph (backbone-filtered per PSY-1295)
  STATION_GRAPH: (slug: string) =>
    `${API_BASE_URL}/radio-stations/${slug}/graph`,

  // Shows
  SHOWS: `${API_BASE_URL}/radio-shows`,
  SHOW: (slug: string) => `${API_BASE_URL}/radio-shows/${slug}`,
  SHOW_EPISODES: (slug: string) => `${API_BASE_URL}/radio-shows/${slug}/episodes`,
  SHOW_EPISODE_BY_DATE: (slug: string, date: string) =>
    `${API_BASE_URL}/radio-shows/${slug}/episodes/${date}`,
  SHOW_TOP_ARTISTS: (slug: string) => `${API_BASE_URL}/radio-shows/${slug}/top-artists`,
  SHOW_TOP_LABELS: (slug: string) => `${API_BASE_URL}/radio-shows/${slug}/top-labels`,

  // Cross-entity
  ARTIST_RADIO_PLAYS: (slug: string) => `${API_BASE_URL}/artists/${slug}/radio-plays`,
  RELEASE_RADIO_PLAYS: (slug: string) => `${API_BASE_URL}/releases/${slug}/radio-plays`,

  // Aggregation
  NEW_RELEASES: `${API_BASE_URL}/radio/new-releases`,
  STATS: `${API_BASE_URL}/radio/stats`,
  // PSY-1048: dial-wide latest-playlists feed
  RECENT_EPISODES: `${API_BASE_URL}/radio/episodes/recent`,
} as const

// ============================================================================
// Query Keys
// ============================================================================

export const radioQueryKeys = {
  stations: () => ['radio-stations'] as const,
  station: (slug: string) => ['radio-stations', slug] as const,
  stationEpisodes: (slug: string, params?: object) =>
    ['radio-stations', slug, 'episodes', params] as const,
  stationNowPlaying: (slug: string) =>
    ['radio-stations', slug, 'now-playing'] as const,
  stationTopArtists: (slug: string, params?: object) =>
    ['radio-stations', slug, 'top-artists', params] as const,
  stationTopLabels: (slug: string, params?: object) =>
    ['radio-stations', slug, 'top-labels', params] as const,
  stationGraph: (slug: string) =>
    ['radio-stations', slug, 'graph'] as const,
  // `sort` is omitted from the key object when absent: an explicit
  // `sort: undefined` property would break React Query's partial matching,
  // so invalidateQueries(shows(stationId)) — e.g. the admin radio mutations —
  // would silently stop matching the station page's sort=latest queries.
  shows: (stationId?: number, sort?: string) =>
    sort
      ? (['radio-shows', { stationId, sort }] as const)
      : (['radio-shows', { stationId }] as const),
  show: (slug: string) => ['radio-shows', slug] as const,
  episodes: (showSlug: string, params?: object) =>
    ['radio-shows', showSlug, 'episodes', params] as const,
  episode: (showSlug: string, date: string) =>
    ['radio-shows', showSlug, 'episodes', date] as const,
  // PSY-1051: prev/next neighbors derived from the episodes list
  episodeNeighbors: (showSlug: string, date: string) =>
    ['radio-shows', showSlug, 'episode-neighbors', date] as const,
  topArtists: (showSlug: string, params?: object) =>
    ['radio-shows', showSlug, 'top-artists', params] as const,
  topLabels: (showSlug: string, params?: object) =>
    ['radio-shows', showSlug, 'top-labels', params] as const,
  artistPlays: (artistSlug: string) => ['artists', artistSlug, 'radio-plays'] as const,
  releasePlays: (releaseSlug: string) => ['releases', releaseSlug, 'radio-plays'] as const,
  newReleases: (params?: object) => ['radio', 'new-releases', params] as const,
  stats: () => ['radio', 'stats'] as const,
  recentEpisodes: (params?: object) => ['radio', 'episodes', 'recent', params] as const,
} as const
