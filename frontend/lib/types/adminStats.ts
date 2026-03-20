export interface AdminDashboardStats {
  pending_shows: number
  pending_venue_edits: number
  pending_reports: number
  unverified_venues: number
  total_shows: number
  total_venues: number
  total_artists: number
  total_users: number
  shows_submitted_last_7_days: number
  users_registered_last_7_days: number
  total_shows_trend: number
  total_venues_trend: number
  total_artists_trend: number
  total_users_trend: number
}

export interface ActivityEvent {
  id: number
  event_type: string
  description: string
  entity_type?: string
  entity_slug?: string
  timestamp: string
  actor_name?: string
}
