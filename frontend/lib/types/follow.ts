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

export interface FollowingEntity {
  entity_type: string
  entity_id: number
  name: string
  slug: string
  followed_at: string
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
