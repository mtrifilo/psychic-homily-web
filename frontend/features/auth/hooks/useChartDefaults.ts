'use client'

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import { queryKeys } from '@/lib/queryClient'

export type ChartDefaultWindow = 'month' | 'quarter' | 'all_time'

export interface ChartDefaults {
  window: ChartDefaultWindow
  scene: string | null
}

interface SetChartDefaultsResponse {
  success: boolean
  message: string
  defaults: ChartDefaults | null
}

/**
 * Mutation hook to save / clear /charts landing defaults (PSY-1423).
 * Pass null to clear. Invalidates profile so the new defaults propagate.
 */
export const useSetChartDefaults = () => {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (
      defaults: ChartDefaults | null
    ): Promise<SetChartDefaultsResponse> => {
      return apiRequest<SetChartDefaultsResponse>(
        API_ENDPOINTS.AUTH.CHART_DEFAULTS,
        {
          method: 'PUT',
          body: JSON.stringify({ defaults }),
        }
      )
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.auth.profile })
    },
  })
}
