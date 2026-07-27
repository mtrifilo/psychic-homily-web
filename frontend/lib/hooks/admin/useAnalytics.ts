'use client'

import { useQuery } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '../../api'
import { queryKeys } from '../../queryClient'

// --- Types ---
//
// Sourced from the backend's OpenAPI document, NOT hand-written (PSY-1550).
// Regenerate with `bun run api:types`; CI fails if the committed types drift
// from the backend.
//
// These were hand-written until PSY-1550, and had drifted from the wire twice.
// PSY-1547 was one instance (`crate_items` read as `collection_items`) and took
// the whole admin console down in production. The second was still live when
// these were replaced: `new_crates_30d` was declared as `new_collections_30d`,
// with a comment asserting it was "the backend API field", so the
// "New Collections (30d)" card silently rendered undefined. The tests encoded
// the same wrong name, so CI stayed green throughout.
//
// Aliasing the generated schemas keeps the exported names stable for callers
// while making the field names impossible to get wrong.

import type { components } from '../../../types/api'

export type MonthlyCount = components['schemas']['EngagementMetricResponse']
export type GrowthMetrics = components['schemas']['GetGrowthMetricsResponseBody']
export type EngagementMetrics = components['schemas']['GetEngagementMetricsResponseBody']
export type WeeklyContribution = components['schemas']['WeeklyContributionsResponse']
export type TopContributor = components['schemas']['TopContributorResponse']
export type CommunityHealth = components['schemas']['GetCommunityHealthResponseBody']
export type DataQualityTrends = components['schemas']['GetDataQualityTrendsResponseBody']

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
