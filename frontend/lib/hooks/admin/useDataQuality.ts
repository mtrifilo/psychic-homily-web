'use client'

import { useQuery } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '../../api'
import { queryKeys } from '../../queryClient'

// Types

export interface DataQualityCategory {
  key: string
  label: string
  entity_type: string
  count: number
  description: string
}

export interface DataQualitySummary {
  categories: DataQualityCategory[]
  total_items: number
}

export interface DataQualityItem {
  entity_type: string
  entity_id: number
  name: string
  slug: string
  reason: string
  show_count: number
}

export interface DataQualityCategoryResponse {
  items: DataQualityItem[]
  total: number
}

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
