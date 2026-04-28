'use client'

/**
 * Artist Bill Composition Hook (PSY-364)
 *
 * Fetches the derived bill-composition payload — opens-with / closes-with tables,
 * stats, and a scoped mini-graph keyed by `shared_bills`. Mirrors the staleness +
 * key conventions of `useArtistGraph`.
 */

import { useQuery } from '@tanstack/react-query'
import { apiRequest } from '@/lib/api'
import { artistEndpoints, artistQueryKeys } from '@/features/artists/api'
import type { ArtistBillComposition } from '../types'

interface UseArtistBillCompositionOptions {
  artistId: number
  months: number // 0 = all-time
  enabled?: boolean
}

export function useArtistBillComposition(options: UseArtistBillCompositionOptions) {
  const { artistId, months, enabled = true } = options

  const params = new URLSearchParams()
  if (months > 0) {
    params.set('months', String(months))
  }
  const queryString = params.toString()
  const endpoint = queryString
    ? `${artistEndpoints.BILL_COMPOSITION(artistId)}?${queryString}`
    : artistEndpoints.BILL_COMPOSITION(artistId)

  return useQuery({
    queryKey: artistQueryKeys.billComposition(artistId, months),
    queryFn: async (): Promise<ArtistBillComposition> => {
      return apiRequest<ArtistBillComposition>(endpoint, { method: 'GET' })
    },
    enabled: enabled && artistId > 0,
    staleTime: 5 * 60 * 1000,
  })
}
