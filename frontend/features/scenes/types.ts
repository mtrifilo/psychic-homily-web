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
}

export interface SceneArtistsResponse {
  artists: SceneArtist[]
  total: number
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
  artist_count: number
  edge_count: number
}

export interface SceneGraphCluster {
  /** "v_<venue_id>" for first-class clusters, "other" for the rolled-up tail. */
  id: string
  /** Venue name, or "Other". */
  label: string
  /** Number of artists in this cluster. */
  size: number
  /** 0..7 = Okabe-Ito palette index. -1 = "other" (use neutral grey). */
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
