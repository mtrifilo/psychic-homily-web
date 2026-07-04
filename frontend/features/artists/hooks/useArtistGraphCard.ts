'use client'

/**
 * Artist Graph Card Hook (PSY-1345)
 *
 * Fetches the node-select summary card for graph surfaces — the homepage
 * scene graph today, intended for the /graph Observatory (unshipped). One
 * small request per selected node; cached per artist so re-selecting a
 * node within the session is free.
 */

import { useQuery } from '@tanstack/react-query'
import { apiRequest } from '@/lib/api'
import { artistEndpoints, artistQueryKeys } from '@/features/artists/api'
import type { ArtistGraphCard } from '../types'

interface UseArtistGraphCardOptions {
  artistId: number | null
  enabled?: boolean
}

export function useArtistGraphCard(options: UseArtistGraphCardOptions) {
  const { artistId, enabled = true } = options

  return useQuery({
    // Timezone is deliberately NOT in the key: it's constant for a browser
    // session, and keying on it would only fork cache entries.
    queryKey: artistQueryKeys.graphCard(artistId ?? 0),
    queryFn: async (): Promise<ArtistGraphCard> => {
      const params = new URLSearchParams()
      if (typeof window !== 'undefined') {
        params.set('timezone', Intl.DateTimeFormat().resolvedOptions().timeZone)
      }
      const queryString = params.toString()
      const endpoint = queryString
        ? `${artistEndpoints.GRAPH_CARD(artistId ?? 0)}?${queryString}`
        : artistEndpoints.GRAPH_CARD(artistId ?? 0)
      return apiRequest<ArtistGraphCard>(endpoint, { method: 'GET' })
    },
    enabled: enabled && artistId !== null && artistId > 0,
    staleTime: 5 * 60 * 1000,
  })
}
