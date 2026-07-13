export const CHART_WINDOWS = ['month', 'quarter', 'all_time'] as const
export type ChartWindow = (typeof CHART_WINDOWS)[number]

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
