'use client'

/**
 * Artist Graph Hooks
 *
 * TanStack Query hooks for fetching artist relationship graph data and voting.
 */

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest } from '@/lib/api'
import { artistEndpoints, artistQueryKeys } from '../api'
import type { ArtistGraph } from '../types'

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

  const params = new URLSearchParams()
  if (types && types.length > 0) {
    params.set('types', types.join(','))
  }
  const queryString = params.toString()
  const endpoint = queryString
    ? `${artistEndpoints.GRAPH(artistId)}?${queryString}`
    : artistEndpoints.GRAPH(artistId)

  return useQuery({
    queryKey: artistQueryKeys.graph(artistId, types),
    queryFn: async (): Promise<ArtistGraph> => {
      return apiRequest<ArtistGraph>(endpoint, { method: 'GET' })
    },
    enabled: enabled && artistId > 0,
    staleTime: 5 * 60 * 1000, // 5 minutes
  })
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
