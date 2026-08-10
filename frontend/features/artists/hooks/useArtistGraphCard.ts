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
  /**
   * Artist numeric ID OR slug. Collection-graph nodes carry collection_item
   * IDs (not artist IDs), so mixed-type surfaces pass the node slug instead
   * (the graph-card handler already resolves either — PSY-1473).
   */
  artistId: number | string | null
  enabled?: boolean
}

export function useArtistGraphCard(options: UseArtistGraphCardOptions) {
  const { artistId, enabled = true } = options
  const idOrSlug = artistId

  return useQuery({
    // Nothing per-viewer in the key OR the request: the next-show cutoff is
    // made in the show's own venue-local day, so one answer is the right answer
    // for every reader.
    queryKey: artistQueryKeys.graphCard(idOrSlug ?? 0),
    queryFn: () =>
      apiRequest<ArtistGraphCard>(artistEndpoints.GRAPH_CARD(idOrSlug ?? 0), {
        method: 'GET',
      }),
    enabled:
      enabled &&
      idOrSlug !== null &&
      (typeof idOrSlug === 'string' ? idOrSlug.length > 0 : idOrSlug > 0),
    staleTime: 5 * 60 * 1000,
  })
}
