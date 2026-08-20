import { useQuery } from '@tanstack/react-query'
import { apiRequest, API_BASE_URL } from '@/lib/api'
import { queryKeys } from '@/lib/queryClient'
import type { DataQualitySummary, DataQualityItem } from '../types'

export const useContributeOpportunities = (options?: { enabled?: boolean }) => {
  return useQuery({
    queryKey: queryKeys.contribute.opportunities,
    queryFn: () =>
      apiRequest<DataQualitySummary>(`${API_BASE_URL}/contribute/opportunities`),
    enabled: options?.enabled ?? true,
  })
}

export const useContributeCategory = (category: string) => {
  return useQuery({
    queryKey: queryKeys.contribute.category(category),
    queryFn: () =>
      apiRequest<{ items: DataQualityItem[]; total: number }>(
        `${API_BASE_URL}/contribute/opportunities/${category}?limit=20`
      ),
    enabled: !!category,
  })
}
