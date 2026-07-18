/** Rolling windows shown as masthead tabs on the live Broadsheet. */
export const CHART_WINDOWS = ['month', 'quarter', 'all_time'] as const
export type RollingChartWindow = (typeof CHART_WINDOWS)[number]

/**
 * Chart window query value: rolling tabs plus calendar archives
 * (`YYYY` / `YYYY-q1..q4`, PSY-1421 grammar).
 */
export type ChartWindow = RollingChartWindow | string

export interface ChartScene {
  metro: string
  name: string
  city: string
  state: string
  show_count: number
  artist_count: number
  venue_count: number
}

export interface ChartScenesResponse {
  window: ChartWindow
  scenes: ChartScene[]
}

export interface PersonalTopVenue {
  venue_id: number
  name: string
  slug: string
  saved_show_count: number
}

export interface PersonalChartsStats {
  saved_shows: number
  artists_followed: number
  top_venue: PersonalTopVenue | null
  first_activity_at: string | null
}

export interface ChartEntityReference {
  id: number
  name: string
  slug: string
}

interface RankedArtistBase {
  artist_id: number
  name: string
  slug: string
  city: string
  state: string
  rank: number
}

export interface MostActiveArtist extends RankedArtistBase {
  show_count: number
  headline_pct: number
  last_show_date: string | null
  last_show_slug: string
  last_show_venue: string
}

export interface OnTheRadioArtist extends RankedArtistBase {
  play_count: number
  station_count: number
  is_new: boolean
}

export interface OpenerToWatch extends RankedArtistBase {
  support_slot_count: number
}

export interface MostAnticipatedShow {
  show_id: number
  title: string
  slug: string
  date: string
  venue_name: string
  venue_slug: string
  city: string
  artist_names: string[]
  save_count?: number
  rank?: number
}

export interface BusiestVenue {
  venue_id: number
  name: string
  slug: string
  city: string
  state: string
  show_count: number
  rank: number
}

export interface NewRelease {
  release_id: number
  title: string
  slug: string
  release_type: string
  release_date: string | null
  added_at: string
  artists: ChartEntityReference[]
  labels: ChartEntityReference[]
  rank: number
}

export interface WindowedChartResponse {
  window: ChartWindow
  scene: string
  total: number
}

export interface MostActiveArtistsResponse extends WindowedChartResponse {
  artists: MostActiveArtist[]
}

export interface OnTheRadioResponse extends WindowedChartResponse {
  artists: OnTheRadioArtist[]
}

export interface MostAnticipatedResponse {
  mode: 'ranked' | 'soonest_upcoming'
  scene: string
  total: number
  shows: MostAnticipatedShow[]
}

export interface BusiestVenuesResponse extends WindowedChartResponse {
  venues: BusiestVenue[]
}

export interface NewReleasesResponse extends WindowedChartResponse {
  releases: NewRelease[]
}

export interface OpenersToWatchResponse extends WindowedChartResponse {
  artists: OpenerToWatch[]
}

export interface TopTag {
  tag_id: number
  name: string
  slug: string
  category: string
  weighted_saves: number
  show_count: number
  rank: number
}

export interface TopTagsResponse extends WindowedChartResponse {
  tags: TopTag[]
}

export interface ChartsSummaryResponse {
  window: ChartWindow
  scene: string
  shows_added: number
  new_artists: number
  new_releases: number
  radio_plays: number
  active_scenes: number
}

export type FreshlyAddedEntityType = 'artist' | 'venue' | 'release' | 'station'

export interface FreshlyAddedItem {
  entity_type: FreshlyAddedEntityType
  entity_id: number
  name: string
  slug: string
  added_at: string
}

export interface FreshlyAddedResponse {
  scene: string
  items: FreshlyAddedItem[]
}

/** Entity types accepted by GET /charts/rank (PSY-1419). */
export type ChartRankEntityType = 'show' | 'artist' | 'venue' | 'release'

/**
 * Module identity echoed on a rank lookup so the badge can deep-link
 * without re-deriving the entity→module map.
 */
export type ChartRankModule =
  | 'most-anticipated'
  | 'most-active-artists'
  | 'busiest-venues'
  | 'new-releases'

/**
 * Per-entity chart placement for a window. Rank is null when the entity
 * has no placement — below floor, out of window, or most-anticipated
 * fallback mode — never 0.
 */
export interface ChartEntityRank {
  entity_type: ChartRankEntityType
  entity_id: number
  window: string
  module: ChartRankModule
  rank: number | null
}
