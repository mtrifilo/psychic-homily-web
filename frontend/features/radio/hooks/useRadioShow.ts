'use client'

import { useQuery } from '@tanstack/react-query'
import { apiRequest } from '@/lib/api'
import { radioEndpoints, radioQueryKeys } from '../api'
import type { RadioShowDetail } from '../types'

/**
 * Hook to fetch a single radio show by slug
 */
export function useRadioShow(slug: string) {
  return useQuery({
    queryKey: radioQueryKeys.show(slug),
    queryFn: () =>
      apiRequest<RadioShowDetail>(radioEndpoints.SHOW(slug), {
        method: 'GET',
      }),
    enabled: slug.length > 0,
    staleTime: 5 * 60 * 1000,
  })
}
