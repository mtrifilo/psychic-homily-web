// Leaderboard types

export type LeaderboardDimension = 'overall' | 'shows' | 'venues' | 'tags' | 'edits' | 'requests'
export type LeaderboardPeriod = 'all_time' | 'month' | 'week'

export interface LeaderboardEntry {
  rank: number
  user_id: number
  username: string
  avatar_url?: string
  user_tier: string
  count: number
}

export interface LeaderboardResponse {
  entries: LeaderboardEntry[]
  dimension: string
  period: string
  user_rank?: number
}

export const DIMENSION_LABELS: Record<LeaderboardDimension, string> = {
  overall: 'Overall',
  shows: 'Shows',
  venues: 'Venues',
  tags: 'Tags',
  edits: 'Edits',
  requests: 'Requests',
}

export const PERIOD_LABELS: Record<LeaderboardPeriod, string> = {
  all_time: 'All Time',
  month: 'This Month',
  week: 'This Week',
}
