'use client'

import { useQuery } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '../../api'
import { queryKeys } from '../../queryClient'

// --- Types ---
//
// Aliased from the generated OpenAPI types, not hand-written (PSY-1550/1600).
// Regenerate with `bun run api:types`; the "API Types Drift" CI gate fails if
// the committed types drift from the backend.
//
// Exported names are kept stable for callers. Note the generated schema
// `DataQualityCategoryResponse` is the per-category descriptor (what this
// module has always called `DataQualityCategory`), while this module's
// `DataQualityCategoryResponse` is the paginated item envelope — the two names
// collide, so the aliases below deliberately do NOT mirror the generated names.

import type { components } from '../../../types/api'

export type DataQualityCategory =
  components['schemas']['DataQualityCategoryResponse']
export type DataQualitySummary =
  components['schemas']['GetDataQualitySummaryResponseBody']
export type DataQualityItem = components['schemas']['DataQualityItemResponse']
export type DataQualityCategoryResponse =
  components['schemas']['GetDataQualityCategoryResponseBody']

/**
 * Hook to fetch data quality summary (counts per category)
 */
export const useDataQualitySummary = () => {
  return useQuery({
    queryKey: queryKeys.admin.dataQuality.summary,
    queryFn: async (): Promise<DataQualitySummary> => {
      return apiRequest<DataQualitySummary>(
        API_ENDPOINTS.ADMIN.DATA_QUALITY.SUMMARY,
        { method: 'GET' }
      )
    },
    staleTime: 60 * 1000, // 1 minute
  })
}

/**
 * Hook to fetch paginated items for a specific data quality category
 */
export const useDataQualityCategory = (
  category: string,
  limit: number = 50,
  offset: number = 0,
  options?: { enabled?: boolean }
) => {
  return useQuery({
    queryKey: queryKeys.admin.dataQuality.category(category, limit, offset),
    queryFn: async (): Promise<DataQualityCategoryResponse> => {
      const url = `${API_ENDPOINTS.ADMIN.DATA_QUALITY.CATEGORY(category)}?limit=${limit}&offset=${offset}`
      return apiRequest<DataQualityCategoryResponse>(url, { method: 'GET' })
    },
    enabled: options?.enabled !== false && !!category,
    staleTime: 60 * 1000, // 1 minute
  })
}
