/**
 * Follow-related TypeScript types.
 *
 * These types match the backend API response structures
 * from backend/internal/api/handlers/follow.go and
 * backend/internal/services/contracts/engagement.go.
 */

export interface FollowStatus {
  entity_type: string
  entity_id: number
  follower_count: number
  is_following: boolean
  /** Scene follows only (PSY-1341): the viewer's new-show notify mode. */
  notify_mode?: string
}

/** GET /users/{username}/followers — count + viewer follow state (no list). */
export interface UserFollowStatus {
  username: string
  follower_count: number
  is_following: boolean
}

export interface BatchFollowEntry {
  follower_count: number
  is_following: boolean
}

export interface BatchFollowResponse {
  follows: Record<string, BatchFollowEntry>
}

/**
 * Geographic reach of an artist follow's new-show alerts (PSY-1893).
 * `near_me` matches the viewer's home metro; `everywhere` ignores geography.
 * Venue follows have no scope axis, because a venue sits in one place.
 */
export type FollowAlertScope = 'near_me' | 'everywhere'

/**
 * One alert type's RESOLVED settings for a follow: shipped defaults, then the
 * account matrix, then the per-follow override, narrowest wins. The server
 * resolves all three, so every field here is a concrete value rather than the
 * tri-state that is stored.
 */
export interface FollowAlertPreference {
  enabled: boolean
  in_app: boolean
  email: boolean
  /** Artist show alerts only; absent for releases and for venue follows. */
  scope?: FollowAlertScope
}

/** GET /{entity_type}/{entity_id}/follow/alerts (PSY-1893). */
export interface FollowAlertSettings {
  entity_type: string
  entity_id: number
  shows: FollowAlertPreference
  /** Artist follows only: a venue does not put out records. */
  releases?: FollowAlertPreference
}

/** PATCH body: every axis optional, and an omitted axis is left untouched. */
export interface FollowAlertPreferenceUpdate {
  enabled?: boolean
  in_app?: boolean
  email?: boolean
  scope?: FollowAlertScope
}

export interface FollowAlertUpdate {
  shows?: FollowAlertPreferenceUpdate
  releases?: FollowAlertPreferenceUpdate
}

export interface FollowingEntity {
  entity_type: string
  entity_id: number
  name: string
  slug: string
  followed_at: string
  /**
   * The follow's resolved alert subscription, served with the Library row so
   * the per-row control renders without a request per row (PSY-1893). Absent
   * for follow types that carry no alert subscription, and absent entirely on
   * the older /me/following list, which shares this type.
   */
  alerts?: FollowAlertSettings
}

export interface FollowingListResponse {
  following: FollowingEntity[]
  total: number
  limit: number
  offset: number
}

export interface LibraryFollowingCounts {
  artists: number
  venues: number
  scenes: number
  labels: number
  festivals: number
  tags: number
}

export interface LibraryFollowingPage {
  following: FollowingEntity[]
  limit: number
  next_cursor?: string
}

export interface Follower {
  user_id: number
  username: string
  display_name?: string
}
