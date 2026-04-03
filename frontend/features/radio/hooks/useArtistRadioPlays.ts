'use client'

import { useQuery } from '@tanstack/react-query'
import { apiRequest } from '@/lib/api'
import { radioEndpoints, radioQueryKeys } from '../api'
import type { RadioAsHeardOnResponse } from '../types'

/**
 * Hook to fetch radio plays for an artist ("As Heard On")
 */
export function useArtistRadioPlays(artistSlug: string, enabled = true) {
  return useQuery({
    queryKey: radioQueryKeys.artistPlays(artistSlug),
    queryFn: () =>
      apiRequest<RadioAsHeardOnResponse>(
        radioEndpoints.ARTIST_RADIO_PLAYS(artistSlug),
        { method: 'GET' }
      ),
    enabled: enabled && artistSlug.length > 0,
    staleTime: 5 * 60 * 1000,
  })
}
