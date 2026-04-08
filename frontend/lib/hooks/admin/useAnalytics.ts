'use client'

import { useQuery } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '../../api'
import { queryKeys } from '../../queryClient'

// --- Types ---

export interface MonthlyCount {
  month: string
  count: number
}

export interface GrowthMetrics {
  shows: MonthlyCount[]
  artists: MonthlyCount[]
  venues: MonthlyCount[]
  releases: MonthlyCount[]
  labels: MonthlyCount[]
  users: MonthlyCount[]
}

export interface EngagementMetrics {
  bookmarks: MonthlyCount[]
  tags_added: MonthlyCount[]
  tag_votes: MonthlyCount[]
  collection_items: MonthlyCount[] // backend API field; displayed as "Collection Items"
  requests: MonthlyCount[]
  request_votes: MonthlyCount[]
  revisions: MonthlyCount[]
  follows: MonthlyCount[]
  attendance: MonthlyCount[]
}

export interface WeeklyContribution {
  week: string
  count: number
}

export interface TopContributor {
  user_id: number
  username: string
  display_name?: string
  count: number
}

export interface CommunityHealth {
  active_contributors_30d: number
  contributions_per_week: WeeklyContribution[]
  request_fulfillment_rate: number
  new_collections_30d: number // backend API field; displayed as "New Collections (30d)"
  top_contributors: TopContributor[]
}

export interface DataQualityTrends {
  shows_approved: MonthlyCount[]
  shows_rejected: MonthlyCount[]
  pending_review_count: number
  artists_without_releases: number
  inactive_venues_90d: number
}

// --- Hooks ---

/**
 * Hook to fetch growth metrics (entity creation trends over time)
 */
export const useGrowthMetrics = (months: number = 6) => {
  return useQuery({
    queryKey: queryKeys.admin.analytics.growth(months),
    queryFn: async (): Promise<GrowthMetrics> => {
      const url = `${API_ENDPOINTS.ADMIN.ANALYTICS.GROWTH}?months=${months}`
      return apiRequest<GrowthMetrics>(url, { method: 'GET' })
    },
    staleTime: 5 * 60 * 1000, // 5 minutes
  })
}

/**
 * Hook to fetch engagement metrics (user activity trends over time)
 */
export const useEngagementMetrics = (months: number = 6) => {
  return useQuery({
    queryKey: queryKeys.admin.analytics.engagement(months),
    queryFn: async (): Promise<EngagementMetrics> => {
      const url = `${API_ENDPOINTS.ADMIN.ANALYTICS.ENGAGEMENT}?months=${months}`
      return apiRequest<EngagementMetrics>(url, { method: 'GET' })
    },
    staleTime: 5 * 60 * 1000,
  })
}

/**
 * Hook to fetch community health snapshot
 */
export const useCommunityHealth = () => {
  return useQuery({
    queryKey: queryKeys.admin.analytics.community,
    queryFn: async (): Promise<CommunityHealth> => {
      return apiRequest<CommunityHealth>(
        API_ENDPOINTS.ADMIN.ANALYTICS.COMMUNITY,
        { method: 'GET' }
      )
    },
    staleTime: 5 * 60 * 1000,
  })
}

/**
 * Hook to fetch data quality trends (approval/rejection over time + snapshots)
 */
export const useDataQualityTrends = (months: number = 6) => {
  return useQuery({
    queryKey: queryKeys.admin.analytics.dataQualityTrends(months),
    queryFn: async (): Promise<DataQualityTrends> => {
      const url = `${API_ENDPOINTS.ADMIN.ANALYTICS.DATA_QUALITY}?months=${months}`
      return apiRequest<DataQualityTrends>(url, { method: 'GET' })
    },
    staleTime: 5 * 60 * 1000,
  })
}
