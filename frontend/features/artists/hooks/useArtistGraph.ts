'use client'

/**
 * Artist Graph Hooks
 *
 * TanStack Query hooks for fetching artist relationship graph data and voting.
 */

import { useCallback } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest } from '@/lib/api'
import { artistEndpoints, artistQueryKeys } from '@/features/artists/api'
import type { ArtistGraph } from '../types'

const GRAPH_STALE_TIME = 5 * 60 * 1000 // 5 minutes

function graphEndpoint(artistId: number, types?: string[]): string {
  const params = new URLSearchParams()
  if (types && types.length > 0) {
    params.set('types', types.join(','))
  }
  const queryString = params.toString()
  return queryString
    ? `${artistEndpoints.GRAPH(artistId)}?${queryString}`
    : artistEndpoints.GRAPH(artistId)
}

// Single owner of the graph cache contract — key + queryFn + staleTime in one place — so the
// reactive hook (useQuery) and the imperative expand fetcher (fetchQuery) can't drift on the
// cache KEY or the fetch, which would split the cache and refetch already-loaded artists.
function graphQueryOptions(artistId: number, types?: string[]) {
  return {
    queryKey: artistQueryKeys.graph(artistId, types),
    queryFn: (): Promise<ArtistGraph> =>
      apiRequest<ArtistGraph>(graphEndpoint(artistId, types), { method: 'GET' }),
    staleTime: GRAPH_STALE_TIME,
  }
}

interface UseArtistGraphOptions {
  artistId: number
  types?: string[]
  enabled?: boolean
}

/**
 * Hook to fetch the artist relationship graph (depth 1).
 * Returns center node, related nodes, and links between them.
 */
export function useArtistGraph(options: UseArtistGraphOptions) {
  const { artistId, types, enabled = true } = options

  return useQuery({
    ...graphQueryOptions(artistId, types),
    enabled: enabled && artistId > 0,
  })
}

/**
 * Imperative ego-graph fetcher for expand-on-demand (PSY-1259). Returns a stable
 * async function that fetches an arbitrary artist's graph on click. It shares
 * `useArtistGraph`'s query options (key + staleTime), so an already-loaded artist (the
 * base center, or a previously-expanded/-visited node) resolves instantly from cache
 * rather than re-hitting the network.
 */
export function useFetchArtistGraph() {
  const queryClient = useQueryClient()
  return useCallback(
    (artistId: number, types?: string[]): Promise<ArtistGraph> =>
      queryClient.fetchQuery(graphQueryOptions(artistId, types)),
    [queryClient],
  )
}

interface VoteRelationshipParams {
  sourceId: number
  targetId: number
  type: string
  isUpvote: boolean
  centerArtistId: number
}

/**
 * Mutation hook for voting on artist relationships.
 * Invalidates the graph query on success.
 */
export function useArtistRelationshipVote() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (params: VoteRelationshipParams) => {
      return apiRequest(
        artistEndpoints.RELATIONSHIPS.VOTE(params.sourceId, params.targetId),
        {
          method: 'POST',
          body: JSON.stringify({
            type: params.type,
            is_upvote: params.isUpvote,
          }),
        }
      )
    },
    onSuccess: (_, variables) => {
      queryClient.invalidateQueries({
        queryKey: ['artists', 'graph', String(variables.centerArtistId)],
      })
    },
  })
}

interface CreateRelationshipParams {
  sourceArtistId: number
  targetArtistId: number
  type: string
  centerArtistId: number
}

/**
 * Mutation hook for suggesting a new artist relationship.
 * Creates the relationship and casts an initial upvote.
 */
export function useCreateArtistRelationship() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (params: CreateRelationshipParams) => {
      return apiRequest(artistEndpoints.RELATIONSHIPS.CREATE, {
        method: 'POST',
        body: JSON.stringify({
          source_artist_id: params.sourceArtistId,
          target_artist_id: params.targetArtistId,
          type: params.type,
        }),
      })
    },
    onSuccess: (_, variables) => {
      queryClient.invalidateQueries({
        queryKey: ['artists', 'graph', String(variables.centerArtistId)],
      })
    },
  })
}
