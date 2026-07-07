/**
 * Scene-related TypeScript types
 *
 * These types match the backend API response structures
 * from backend/internal/services/catalog/scene_service.go
 */

export interface SceneListItem {
  city: string
  state: string
  slug: string
  venue_count: number
  upcoming_show_count: number
  total_show_count: number
  // The ≤7-day slice of upcoming_show_count (PSY-1309) — drives the globe's
  // "happening this week" pulse treatment.
  shows_this_week: number
  // Geocoded city centroid for the /atlas map (PSY-1212). Absent (undefined)
  // or null when the geocoder couldn't place the city — such scenes can't be
  // plotted on the globe.
  latitude?: number | null
  longitude?: number | null
}

export interface SceneListResponse {
  scenes: SceneListItem[]
  count: number
}

export interface SceneStats {
  venue_count: number
  artist_count: number
  upcoming_show_count: number
  festival_count: number
}

export interface ScenePulse {
  shows_this_month: number
  shows_prev_month: number
  shows_trend: number
  new_artists_30d: number
  active_venues_this_month: number
  shows_by_month: number[]
}

export interface SceneDetail {
  city: string
  state: string
  slug: string
  description: string | null
  stats: SceneStats
  pulse: ScenePulse
}

export interface SceneArtist {
  id: number
  slug: string
  name: string
  city: string
  state: string
  show_count: number
  // True when the band has an upcoming approved show or one within the active
  // window (~6mo), played anywhere (PSY-1255 step C). The roster lists every
  // band BASED in the metro; the UI highlights the active ones.
  is_active: boolean
  // The artist's embeddable Bandcamp /album|/track URL, null when the artist has
  // none. (The /atlas preview's player now uses the scene-level
  // `representative_embed` — PSY-1294 — so the preview no longer reads this
  // per-artist field, but it stays on the roster payload.)
  bandcamp_embed_url?: string | null
}

// The single band whose Bandcamp embed represents the scene (PSY-1294), chosen
// server-side over the FULL metro roster (active-first) so the preview's player
// is independent of the fetched window. Populated on the first page only
// (offset 0); null on later pages and when no band based here has one.
export interface SceneRepresentativeEmbed {
  embed_url: string
  artist_name: string
  artist_slug: string
}

export interface SceneArtistsResponse {
  artists: SceneArtist[]
  total: number
  representative_embed?: SceneRepresentativeEmbed | null
}

// One upcoming show in the scene preview's "This week" row (PSY-1309) —
// deliberately thin (a line, not the full show payload).
export interface SceneShowSummary {
  id: number
  // Canonical /shows/{slug} target; absent when the show has no slug (fall
  // back to the id — the detail route accepts either).
  slug?: string
  title: string
  event_date: string // ISO date (YYYY-MM-DD)
  venue_name?: string
  // Bill artists in position order — the row's link-text fallback when the
  // title is empty (full rationale on the backend SceneShowSummary contract,
  // PSY-1325).
  artist_names?: string[]
}

export interface SceneShowsResponse {
  shows: SceneShowSummary[]
}

export interface GenreCount {
  tag_id: number
  name: string
  slug: string
  count: number
}

export interface SceneGenreResponse {
  genres: GenreCount[]
  diversity_index: number
  diversity_label: string
}

// ──────────────────────────────────────────────
// Scene graph (PSY-367) — derived per-scene artist relationship graph
// ──────────────────────────────────────────────

export interface SceneGraphInfo {
  slug: string
  city: string
  state: string
  /** Artists in the graph response (top-N cap applied). */
  artist_count: number
  edge_count: number
  /** Full based-in metro roster before the top-N cap (PSY-1277). */
  metro_roster_total: number
  /** True when metro_roster_total > artist_count. */
  roster_truncated: boolean
}

export interface SceneGraphCluster {
  /** "v_<venue_id>" for first-class clusters, "other" for the rolled-up tail. */
  id: string
  /** Venue name, or "Other". */
  label: string
  /** Number of artists in this cluster. */
  size: number
  /** 0..7 = `--chart-{n+1}` palette index (PSY-1083). -1 = "other" (neutral grey). */
  color_index: number
}

export interface SceneGraphNode {
  id: number
  name: string
  slug: string
  city?: string
  state?: string
  upcoming_show_count: number
  /** Matches SceneGraphCluster.id. */
  cluster_id: string
  /** True when the artist has zero in-scene edges (post type-filter). */
  is_isolate: boolean
  /** True when selecting this node opens a playable embed (Bandcamp or an
   * embeddable Spotify URL) — drives the canvas playable-marker ring (PSY-1379). */
  has_playable_audio: boolean
}

export interface SceneGraphLink {
  source_id: number
  target_id: number
  type: string
  score: number
  detail?: Record<string, unknown>
  /** True when source.cluster_id !== target.cluster_id. */
  is_cross_cluster: boolean
}

export interface SceneGraphResponse {
  scene: SceneGraphInfo
  clusters: SceneGraphCluster[]
  nodes: SceneGraphNode[]
  links: SceneGraphLink[]
}
