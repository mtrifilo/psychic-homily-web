/**
 * Radio-related TypeScript types
 *
 * These types match the backend API response structures
 * for radio endpoints.
 */

// ============================================================================
// Radio Station
// ============================================================================

// PSY-669 / PSY-673: per-station network metadata. `is_flagship` is the
// bool on the station itself (radio_stations.is_flagship): true means
// THIS station is the network's primary/default broadcast. Frontend uses
// it to (a) hide non-flagship siblings from the /radio index, and (b)
// render the network tab bar (PSY-674) with the flagship as default.
export interface RadioNetworkInfo {
  slug: string
  name: string
  is_flagship: boolean
}

// PSY-669: other stations in the same network. Excludes self.
// Flagship-first sort; alphabetical within is_flagship buckets.
export interface RadioSiblingStation {
  id: number
  slug: string
  name: string
  broadcast_type: string
  frequency_mhz: number | null
  is_flagship: boolean
}

export interface RadioStationListItem {
  id: number
  name: string
  slug: string
  city: string | null
  state: string | null
  country: string | null
  broadcast_type: string
  frequency_mhz: number | null
  logo_url: string | null
  is_active: boolean
  network: RadioNetworkInfo | null
  sibling_stations: RadioSiblingStation[]
  show_count: number
}

// PSY-673: returns true when a station should appear on the /radio index.
// Network-less stations always render. Stations in a network render only
// when they're the flagship; non-flagship siblings reach their pages via
// the flagship's tab bar (PSY-674) or via their direct /radio/{slug} URL.
export function isStationVisibleOnIndex(station: {
  network: RadioNetworkInfo | null
}): boolean {
  if (!station.network) return true
  return station.network.is_flagship
}

/**
 * Editorial order for The Dial strips on /radio (PSY-1459).
 *
 * `ListStations` remains `name ASC` for generic consumers; the dial pins
 * preferred flagships first, then falls back to name for everyone else.
 * Lower priority number = earlier on the dial. Unlisted slugs sort after
 * all pinned stations.
 */
const DIAL_STATION_PRIORITY: Readonly<Record<string, number>> = {
  wfmu: 0,
}

export function sortDialStations<T extends { slug: string; name: string }>(
  stations: T[]
): T[] {
  return [...stations].sort((a, b) => {
    const pa = DIAL_STATION_PRIORITY[a.slug] ?? Number.MAX_SAFE_INTEGER
    const pb = DIAL_STATION_PRIORITY[b.slug] ?? Number.MAX_SAFE_INTEGER
    if (pa !== pb) return pa - pb
    return a.name.localeCompare(b.name)
  })
}

// PSY-674: returns the canonical /radio detail URL for a station.
//   Network-less or flagship → /radio/{slug}
//   Sub-stream                → /radio/{network-slug}/channel/{local-slug}
// where local-slug = station.slug with the "{network-slug}-" prefix stripped.
// The "channel" literal segment disambiguates sub-streams from show pages,
// which live at /radio/{station-slug}/{show-slug}. Sub-stream slugs follow
// the network-prefix convention (wfmu-drummer, wfmu-rocknsoulradio,
// wfmu-sheena); a slug without the prefix falls back to the full slug
// rather than producing a malformed URL.
export function getStationDetailUrl(
  slug: string,
  network: { slug: string; is_flagship: boolean } | null
): string {
  if (!network || network.is_flagship) {
    return `/radio/${slug}`
  }
  const prefix = `${network.slug}-`
  const localSlug = slug.startsWith(prefix) ? slug.slice(prefix.length) : slug
  return `/radio/${network.slug}/channel/${localSlug}`
}

export interface RadioStationDetail {
  id: number
  name: string
  slug: string
  description: string | null
  city: string | null
  state: string | null
  country: string | null
  timezone: string | null
  stream_url: string | null
  stream_urls: Record<string, string> | null
  website: string | null
  donation_url: string | null
  donation_embed_url: string | null
  logo_url: string | null
  social: Record<string, string> | null
  broadcast_type: string
  frequency_mhz: number | null
  playlist_source: string | null
  playlist_config: Record<string, unknown> | null
  last_playlist_fetch_at: string | null
  is_active: boolean
  network: RadioNetworkInfo | null
  sibling_stations: RadioSiblingStation[]
  show_count: number
  created_at: string
  updated_at: string
}

export interface RadioStationsListResponse {
  stations: RadioStationListItem[]
  count: number
}

// ============================================================================
// Radio Show
// ============================================================================

/** Closed enum (DB CHECK constraint + huma enum tag): the show
 * active-vs-historical signal. */
export type RadioLifecycleState = 'active' | 'dormant' | 'retired'

export interface RadioShowListItem {
  id: number
  station_id: number
  station_name: string
  name: string
  slug: string
  host_name: string | null
  /** Human-readable air slot ("Mon 9pm-12am"); PSY-1050 shows directory. */
  schedule_display: string | null
  genre_tags: string[] | null
  image_url: string | null
  /** Legacy operational flag (true for nearly every row — it keeps dormant
   * shows polling). NOT the active-vs-historical signal; use
   * lifecycle_state for anything user-facing (PSY-1326). */
  is_active: boolean
  /** The janitor-maintained signal (PSY-1155) behind the directory's
   * active count, sort bucket, and dimming. */
  lifecycle_state: RadioLifecycleState
  episode_count: number
  /** Air date (YYYY-MM-DD) of the show's most recent episode (PSY-1048). */
  latest_air_date: string | null
  /**
   * Frozen air window of that same latest visible episode (PSY-1306); null
   * when the show has no episodes or its latest is windowless. The directory
   * LAST column renders viewer-local from these, agreeing with the feed.
   */
  latest_starts_at: string | null
  latest_ends_at: string | null
}

export interface RadioShowDetail {
  id: number
  station_id: number
  station_name: string
  station_slug: string
  name: string
  slug: string
  host_name: string | null
  description: string | null
  schedule_display: string | null
  schedule: Record<string, unknown> | null
  genre_tags: string[] | null
  archive_url: string | null
  image_url: string | null
  is_active: boolean
  episode_count: number
  created_at: string
  updated_at: string
}

export interface RadioShowsListResponse {
  shows: RadioShowListItem[]
  count: number
}

// ============================================================================
// Radio Episode
// ============================================================================

export interface RadioEpisodeListItem {
  id: number
  show_id: number
  title: string | null
  air_date: string
  air_time: string | null
  duration_minutes: number | null
  archive_url: string | null
  /**
   * Frozen air window (PSY-1152), ISO instants; null when the provider has no
   * time (WFMU until PSY-1159). "live now" is computed from these, never from
   * air_date equality (which produced the PSY-1128 false-ON-AIR bug).
   */
  starts_at: string | null
  ends_at: string | null
  /**
   * Coarse server-computed lifecycle state. NOT rendered today (reserved for a
   * future status label) and NOT a live signal: for "is it live RIGHT NOW" call
   * isLiveNow() with the window below, which re-evaluates against the VIEWER's
   * clock — a shipped status='live' is a server-time snapshot that goes stale.
   */
  status: 'scheduled' | 'live' | 'aired' | 'archived'
  /**
   * True when air_date is still in the future in the station's local timezone — a
   * not-yet-aired broadcast (PSY-1205). Windowless providers (WFMU) can't express
   * this via `status` (a null window settles to "aired"), so the archive uses this
   * to label upcoming rows instead of showing an empty, aired-looking playlist.
   */
  is_upcoming: boolean
  play_count: number
  created_at: string
}

export interface RadioEpisodeDetail {
  id: number
  show_id: number
  show_name: string
  show_slug: string
  station_name: string
  station_slug: string
  title: string | null
  air_date: string
  air_time: string | null
  /**
   * Frozen air window (PSY-1238) + the station's IANA zone (PSY-1306); the
   * detail page renders its "aired ..." line viewer-local from the window,
   * with a station-local aside via the timezone. All null-able.
   */
  starts_at: string | null
  ends_at: string | null
  station_timezone: string | null
  /** Not-yet-aired (air_date > station-local today), PSY-1205 — the detail page
   *  labels it "upcoming" instead of "aired {date}". */
  is_upcoming: boolean
  duration_minutes: number | null
  description: string | null
  archive_url: string | null
  mixcloud_url: string | null
  genre_tags: string[] | null
  mood_tags: string[] | null
  play_count: number
  plays: RadioPlay[]
  created_at: string
}

export interface RadioEpisodesListResponse {
  episodes: RadioEpisodeListItem[]
  total: number
}

// ============================================================================
// Radio Play
// ============================================================================

export interface RadioPlay {
  id: number
  episode_id: number
  position: number
  artist_name: string
  track_title: string | null
  album_title: string | null
  label_name: string | null
  release_year: number | null
  is_new: boolean
  rotation_status: string | null
  dj_comment: string | null
  is_live_performance: boolean
  is_request: boolean
  artist_id: number | null
  artist_slug: string | null
  release_id: number | null
  release_slug: string | null
  label_id: number | null
  label_slug: string | null
  musicbrainz_artist_id: string | null
  musicbrainz_recording_id: string | null
  musicbrainz_release_id: string | null
  air_timestamp: string | null
}

// ============================================================================
// Aggregation / Stats
// ============================================================================

export interface RadioTopArtist {
  artist_name: string
  artist_id: number | null
  artist_slug: string | null
  play_count: number
  episode_count: number
}

export interface RadioTopLabel {
  label_name: string
  label_id: number | null
  label_slug: string | null
  play_count: number
}

export interface RadioAsHeardOn {
  station_id: number
  station_name: string
  station_slug: string
  show_id: number
  show_name: string
  show_slug: string
  play_count: number
  last_played: string
}

// New Release Radar link resolution (PSY-1076, extracted from the hub's
// sidebar box): link the "Artist — Album" line to the release when matched,
// else to the artist, else nowhere (no dead links). Shared by the hub box
// and the /radio/new-releases full view — keep them in lockstep.
export function getNewReleaseHref(
  entry: Pick<RadioNewReleaseRadarEntry, 'release_slug' | 'artist_slug'>
): string | null {
  if (entry.release_slug) return `/releases/${entry.release_slug}`
  if (entry.artist_slug) return `/artists/${entry.artist_slug}`
  return null
}

export interface RadioNewReleaseRadarEntry {
  artist_name: string
  artist_id: number | null
  artist_slug: string | null
  album_title: string | null
  label_name: string | null
  release_id: number | null
  release_slug: string | null
  label_id: number | null
  label_slug: string | null
  play_count: number
  station_count: number
}

export interface RadioStats {
  total_stations: number
  total_shows: number
  total_episodes: number
  total_plays: number
  matched_plays: number
  unique_artists: number
}

// ============================================================================
// List response wrappers
// ============================================================================

export interface RadioTopArtistsResponse {
  artists: RadioTopArtist[]
  count: number
}

export interface RadioTopLabelsResponse {
  labels: RadioTopLabel[]
  count: number
}

export interface RadioAsHeardOnResponse {
  stations: RadioAsHeardOn[]
  count: number
}

export interface RadioNewReleasesResponse {
  releases: RadioNewReleaseRadarEntry[]
  count: number
}

// ============================================================================
// Constants
// ============================================================================

export const BROADCAST_TYPE_LABELS: Record<string, string> = {
  terrestrial: 'FM/AM',
  internet: 'Internet',
  both: 'FM/AM + Internet',
}

export const ROTATION_STATUS_LABELS: Record<string, string> = {
  heavy: 'Heavy Rotation',
  medium: 'Medium Rotation',
  light: 'Light Rotation',
  recommended_new: 'Recommended New',
  library: 'Library',
}

/**
 * Rotation-status tint, bound to the DS categorical palette (PSY-943). The
 * tokens map to a rough play-intensity gradient — heavy = chart-1 (orange,
 * hottest), medium = chart-3 (gold), light = chart-6 (denim, cooler),
 * recommended_new = chart-2 (green, "fresh"), library = muted (archival).
 * `--chart-4` (== --destructive) is intentionally avoided: a red rotation
 * pill would read as an error, not "heavy rotation".
 */
export const ROTATION_STATUS_COLORS: Record<string, string> = {
  heavy: 'bg-chart-1/15 text-chart-1 border-chart-1/20',
  medium: 'bg-chart-3/15 text-chart-3 border-chart-3/20',
  light: 'bg-chart-6/15 text-chart-6 border-chart-6/20',
  recommended_new: 'bg-chart-2/15 text-chart-2 border-chart-2/20',
  library: 'bg-muted text-muted-foreground border-border',
}

export function getBroadcastTypeLabel(type: string): string {
  return BROADCAST_TYPE_LABELS[type] ?? type
}

export function getRotationStatusLabel(status: string): string {
  return ROTATION_STATUS_LABELS[status] ?? status
}

export function getRotationStatusColor(status: string): string {
  return ROTATION_STATUS_COLORS[status] ?? 'bg-muted text-muted-foreground border-border'
}

// ============================================================================
// PSY-1048 aggregation shapes (PSY-1049 + PSY-1050 + PSY-1051)
//
// Mirrors backend/internal/services/contracts/radio.go.
// ============================================================================

/**
 * One artist in an episode row's short "played" preview: the raw playlist
 * name plus a knowledge-graph link when the matching engine resolved it.
 * Unmatched artists (null id/slug) render as plain text — never a dead link.
 */
export interface RadioEpisodePreviewArtist {
  artist_name: string
  artist_id: number | null
  artist_slug: string | null
}

/**
 * An episode row in the station-scoped and dial-wide latest-playlists feeds:
 * episode fields plus show and station attribution. Station-scoped feeds are
 * strictly per-station (PSY-1074); the station_* fields exist for the
 * dial-wide hub feed's STATION column.
 */
export interface RadioStationEpisodeRow {
  id: number
  title: string | null
  air_date: string
  /**
   * Frozen air window (PSY-1238), ISO instants; null for windowless rows
   * (no-slot pop-ups, providers without times). The feed renders these in the
   * VIEWER's timezone — date AND time derive from starts_at when present
   * (PSY-1298); windowless rows fall back to date-only air_date.
   */
  starts_at: string | null
  ends_at: string | null
  play_count: number
  archive_url: string | null
  show_id: number
  show_name: string
  show_slug: string
  station_id: number
  station_name: string
  station_slug: string
  /** Always an array (may be empty); matched artists carry id + slug. */
  artist_preview: RadioEpisodePreviewArtist[]
}

/** Response shape for GET /radio/episodes/recent (and station episode feeds). */
export interface RadioRecentEpisodesResponse {
  episodes: RadioStationEpisodeRow[]
  total: number
}

/** Response shape for GET /radio-stations/{slug}/episodes. */
export interface RadioStationEpisodesResponse {
  episodes: RadioStationEpisodeRow[]
  total: number
}

// Declaration-merges into RadioEpisodeListItem above: episode list rows now
// carry the first few distinct playlist artists (PSY-1048).
export interface RadioEpisodeListItem {
  artist_preview: RadioEpisodePreviewArtist[]
}

// ============================================================================
// PSY-1022 now-playing shapes
//
// Mirrors RadioNowPlayingResponse in backend/internal/services/contracts/radio.go.
// ============================================================================

/** The matched our-DB show behind a now-playing payload (null = unmatched). */
export interface RadioNowPlayingShowRef {
  id: number
  name: string
  slug: string
  host_name: string | null
}

/**
 * The current track of a now-playing payload. Field names mirror RadioPlay
 * (minus id/episode_id/position) so track renderers work on both shapes.
 */
export interface RadioNowPlayingTrack {
  artist_name: string
  track_title: string | null
  album_title: string | null
  label_name: string | null
  release_year: number | null
  rotation_status: string | null
  dj_comment: string | null
  artist_id: number | null
  artist_slug: string | null
  release_id: number | null
  release_slug: string | null
  label_id: number | null
  label_slug: string | null
}

/**
 * GET /radio-stations/{slug}/now-playing. `source: 'live'` implies
 * `on_air: true` (the backend serves the archive fallback instead of a
 * half-live payload); `show` is null when the live show name couldn't be
 * matched to exactly one of the station's shows — render `show_name` as
 * plain text, never a dead link (PSY-1073).
 */
export interface RadioNowPlaying {
  source: 'live' | 'latest_archive'
  on_air: boolean
  show: RadioNowPlayingShowRef | null
  show_name: string | null
  /** Live-reported host (may be set even when `show` is unmatched). */
  host_name: string | null
  current_track: RadioNowPlayingTrack | null
  /** Up to 4 distinct previously-played artists, most recent first. */
  recent_artists: RadioEpisodePreviewArtist[]
  /** Air date (YYYY-MM-DD) of the fallback episode; null for live payloads. */
  episode_air_date: string | null
  /**
   * The archive episode's frozen air window (PSY-1306); null for live
   * payloads and windowless episodes. The ON AIR box renders its "Latest
   * playlist" date viewer-local from these.
   */
  episode_starts_at: string | null
  episode_ends_at: string | null
}

/**
 * GET /radio-stations/{slug}/graph (PSY-1299; backend shipped in PSY-1295).
 *
 * Within-station artist co-occurrence subgraph: nodes are the station's
 * top-N artists by play count, edges are same-episode co-occurrence pairs
 * filtered through the global disparity-filter backbone. Field shapes are
 * structural supersets of the shared `GraphNode` / `GraphLink` /
 * `GraphCluster` interfaces in `components/graph/ForceGraphView`.
 */
export interface RadioStationGraphInfo {
  id: number
  slug: string
  name: string
  /** Nodes in the response (top-N cap applied server-side). */
  artist_count: number
  /** Co-occurrence pairs above the min threshold. */
  edge_count: number
  /** Active time window ('last_12m' is the server default). */
  window: 'last_12m' | 'all_time'
}

/** Artists grouped by the station show they are most played on. */
export interface RadioStationGraphCluster {
  /** "rs_<show_id>" or "other". */
  id: string
  label: string
  size: number
  /** 0-7 = Okabe-Ito palette index; -1 = "other" (grey). */
  color_index: number
}

export interface RadioStationGraphNode {
  id: number
  name: string
  slug: string
  city?: string
  state?: string
  upcoming_show_count: number
  /** Matches RadioStationGraphCluster.id. */
  cluster_id: string
  is_isolate: boolean
  /** Play count on this station within the active window. */
  play_count: number
}

export interface RadioStationGraphLink {
  source_id: number
  target_id: number
  /** Always 'radio_cooccurrence'. */
  type: string
  score: number
  /** Carries co_occurrence_count and last_co_occurrence (YYYY-MM-DD). */
  detail?: unknown
  is_cross_cluster: boolean
}

export interface RadioStationGraphResponse {
  station: RadioStationGraphInfo
  clusters: RadioStationGraphCluster[]
  nodes: RadioStationGraphNode[]
  links: RadioStationGraphLink[]
}

/** One scheduled airing on the /radio hub guide (PSY-1053). */
export interface RadioGuideRow {
  station: { slug: string; name: string }
  show: { id: number; slug: string; name: string; host_name?: string | null }
  /**
   * Concrete slot instants (ISO). Rendered viewer-local (PSY-1298) with a
   * station-local aside when the viewer is in a different zone (PSY-1306).
   */
  starts_at: string
  ends_at: string
  station_timezone: string
}

/**
 * Response shape for GET /radio/guide. Schedule-derived only: stations
 * without scraped schedules (KEXP, NTS) contribute no rows by design.
 */
export interface RadioGuideResponse {
  on_now: RadioGuideRow[] | null
  up_next: RadioGuideRow[] | null
  generated_at: string
}
