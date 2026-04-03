'use client'

import { useQuery } from '@tanstack/react-query'
import { apiRequest } from '@/lib/api'
import { radioEndpoints, radioQueryKeys } from '../api'
import type { RadioTopLabelsResponse } from '../types'

interface UseRadioTopLabelsOptions {
  showSlug: string
  period?: number
  limit?: number
  enabled?: boolean
}

/**
 * Hook to fetch top labels for a radio show
 */
export function useRadioTopLabels({
  showSlug,
  period = 90,
  limit = 20,
  enabled = true,
}: UseRadioTopLabelsOptions) {
  const params = new URLSearchParams()
  if (period) params.set('period', period.toString())
  if (limit) params.set('limit', limit.toString())

  const queryString = params.toString()
  const endpoint = queryString
    ? `${radioEndpoints.SHOW_TOP_LABELS(showSlug)}?${queryString}`
    : radioEndpoints.SHOW_TOP_LABELS(showSlug)

  return useQuery({
    queryKey: radioQueryKeys.topLabels(showSlug, { period, limit }),
    queryFn: () =>
      apiRequest<RadioTopLabelsResponse>(endpoint, {
        method: 'GET',
      }),
    enabled: enabled && showSlug.length > 0,
    staleTime: 5 * 60 * 1000,
  })
}
