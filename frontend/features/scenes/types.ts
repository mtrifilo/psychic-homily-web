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
