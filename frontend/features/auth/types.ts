/**
 * Auth, Profile, and Contributor TypeScript types
 *
 * Contributor profile types match the backend API response structures
 * from the PSY-63 contributor profile endpoints.
 * Auth types are used by the auth hooks.
 */

// ============================================================================
// Contributor Profile Types
// ============================================================================

export type ProfileVisibility = 'public' | 'private'

export type PrivacyLevel = 'visible' | 'count_only' | 'hidden'

export type UserTier = 'new_user' | 'contributor' | 'trusted_contributor' | 'local_ambassador'

export interface PrivacySettings {
  contributions: PrivacyLevel
  saved_shows: PrivacyLevel
  attendance: PrivacyLevel
  following: PrivacyLevel
  collections: PrivacyLevel
  last_active: 'visible' | 'hidden'
  profile_sections: 'visible' | 'hidden'
}

export interface ContributionStats {
  // Content creation
  shows_submitted: number
  venues_submitted: number
  venue_edits_submitted: number
  releases_created: number
  labels_created: number
  festivals_created: number
  artists_edited: number
  revisions_made: number
  pending_edits_submitted: number

  // Community participation
  tag_votes_cast: number
  relationship_votes_cast: number
  request_votes_cast: number
  collection_items_added: number
  collection_subscriptions: number
  shows_attended: number

  // Reports
  reports_filed: number
  reports_resolved: number

  // Social
  followers_count: number
  following_count: number

  // Moderation
  moderation_actions: number

  // Computed
  approval_rate?: number
  total_contributions: number
}

export interface PublicProfileResponse {
  username: string
  bio?: string
  avatar_url?: string
  first_name?: string
  profile_visibility: ProfileVisibility
  user_tier: UserTier
  privacy_settings?: PrivacySettings
  joined_at: string
  last_active?: string
  stats?: ContributionStats
  stats_count?: number
  sections?: ProfileSectionResponse[]
}

export interface ContributionEntry {
  id: number
  action: string
  entity_type: string
  entity_id: number
  entity_name?: string
  metadata?: Record<string, unknown>
  created_at: string
  source: string
}

export interface ContributionsResponse {
  contributions: ContributionEntry[]
  total: number
  limit: number
  offset: number
}

export interface ActivityDay {
  date: string  // "2026-03-31"
  count: number
}

export interface ActivityHeatmapResponse {
  days: ActivityDay[]
}

export interface ProfileSectionResponse {
  id: number
  title: string
  content: string
  position: number
  is_visible: boolean
  created_at: string
  updated_at: string
}

export interface ProfileSectionsResponse {
  sections: ProfileSectionResponse[]
}

export interface CreateSectionInput {
  title: string
  content: string
  position: number
}

export interface UpdateSectionInput {
  title?: string
  content?: string
  position?: number
  is_visible?: boolean
}

export interface UpdateVisibilityInput {
  visibility: ProfileVisibility
}

export interface UpdatePrivacyInput {
  contributions?: PrivacyLevel
  saved_shows?: PrivacyLevel
  attendance?: PrivacyLevel
  following?: PrivacyLevel
  collections?: PrivacyLevel
  last_active?: 'visible' | 'hidden'
  profile_sections?: 'visible' | 'hidden'
}

// ============================================================================
// API Token Types (exported from useAuth)
// ============================================================================

export interface APIToken {
  id: number
  description: string | null
  scope: string
  created_at: string
  expires_at: string
  last_used_at: string | null
  is_expired: boolean
}
