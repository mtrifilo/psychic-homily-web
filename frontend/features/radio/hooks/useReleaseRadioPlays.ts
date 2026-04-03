'use client'

import { useQuery } from '@tanstack/react-query'
import { apiRequest } from '@/lib/api'
import { radioEndpoints, radioQueryKeys } from '../api'
import type { RadioAsHeardOnResponse } from '../types'

/**
 * Hook to fetch radio plays for a release ("As Heard On")
 */
export function useReleaseRadioPlays(releaseSlug: string, enabled = true) {
  return useQuery({
    queryKey: radioQueryKeys.releasePlays(releaseSlug),
    queryFn: () =>
      apiRequest<RadioAsHeardOnResponse>(
        radioEndpoints.RELEASE_RADIO_PLAYS(releaseSlug),
        { method: 'GET' }
      ),
    enabled: enabled && releaseSlug.length > 0,
    staleTime: 5 * 60 * 1000,
  })
}
