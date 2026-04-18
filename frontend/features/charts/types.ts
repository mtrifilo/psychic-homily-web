/**
 * Charts-related TypeScript types
 *
 * These types match the backend API response structures
 * from backend/internal/api/handlers/charts.go
 */

export interface TrendingShow {
  show_id: number
  title: string
  slug: string
  date: string
  venue_name: string
  venue_slug: string
  city: string
  artist_names: string[]
  going_count: number
  interested_count: number
  total_attendance: number
}

export interface PopularArtist {
  artist_id: number
  name: string
  slug: string
  image_url: string
  follow_count: number
  upcoming_show_count: number
  score: number
}

export interface ActiveVenue {
  venue_id: number
  name: string
  slug: string
  city: string
  state: string
  upcoming_show_count: number
  follow_count: number
  score: number
}

export interface HotRelease {
  release_id: number
  title: string
  slug: string
  release_date: string | null
  artist_names: string[]
  bookmark_count: number
}

export interface TrendingShowsResponse {
  shows: TrendingShow[]
}

export interface PopularArtistsResponse {
  artists: PopularArtist[]
}

export interface ActiveVenuesResponse {
  venues: ActiveVenue[]
}

export interface HotReleasesResponse {
  releases: HotRelease[]
}

export interface ChartsOverviewResponse {
  trending_shows: TrendingShow[]
  popular_artists: PopularArtist[]
  active_venues: ActiveVenue[]
  hot_releases: HotRelease[]
}

export type ChartView = 'overview' | 'trending-shows' | 'popular-artists' | 'active-venues' | 'hot-releases'
