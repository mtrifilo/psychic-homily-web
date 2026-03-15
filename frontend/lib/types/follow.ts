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

export interface Follower {
  user_id: number
  username: string
  display_name?: string
}

export interface FollowersListResponse {
  followers: Follower[]
  total: number
  limit: number
  offset: number
}
