import { useQuery } from '@tanstack/react-query'
import { apiRequest, API_BASE_URL } from '@/lib/api'
import type { DataQualitySummary, DataQualityItem } from '../types'

export const useContributeOpportunities = () => {
  return useQuery({
    queryKey: ['contribute', 'opportunities'],
    queryFn: () =>
      apiRequest<DataQualitySummary>(`${API_BASE_URL}/contribute/opportunities`),
  })
}

export const useContributeCategory = (category: string) => {
  return useQuery({
    queryKey: ['contribute', 'opportunities', category],
    queryFn: () =>
      apiRequest<{ items: DataQualityItem[]; total: number }>(
        `${API_BASE_URL}/contribute/opportunities/${category}?limit=20`
      ),
    enabled: !!category,
  })
}
