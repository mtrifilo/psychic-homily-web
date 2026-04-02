'use client'

import { useQuery } from '@tanstack/react-query'
import { apiRequest } from '@/lib/api'
import { radioEndpoints, radioQueryKeys } from '../api'
import type { RadioStats } from '../types'

/**
 * Hook to fetch overall radio stats
 */
export function useRadioStats() {
  return useQuery({
    queryKey: radioQueryKeys.stats(),
    queryFn: () =>
      apiRequest<RadioStats>(radioEndpoints.STATS, {
        method: 'GET',
      }),
    staleTime: 5 * 60 * 1000,
  })
}
