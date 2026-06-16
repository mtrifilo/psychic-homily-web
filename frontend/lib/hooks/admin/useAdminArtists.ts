'use client'

/**
 * Admin Artists Hooks
 *
 * TanStack Query hooks for admin artist operations, including
 * music discovery (Bandcamp/Spotify) and manual URL updates.
 */

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '../../api'
import { queryKeys } from '../../queryClient'
import type { Artist, ArtistEditRequest, ArtistAliasesResponse, ArtistAlias, MergeArtistResult } from '@/features/artists'

/**
 * Platform types for music discovery
 */
export type MusicPlatform = 'bandcamp' | 'spotify'

/**
 * One candidate streaming-link result for a single platform.
 */
export interface DiscoveryCandidate {
  url: string
  name_as_listed: string | null
  location: string | null
  notable_release: string | null
  genres: string | null
  popularity: string | null
  confidence: 'high' | 'medium' | 'low'
  why_might_match: string | null
}

/**
 * Response from music discovery endpoint. Discovery returns CANDIDATES only;
 * the admin reviews and picks before any save happens.
 */
export interface DiscoverMusicResponse {
  bandcamp: DiscoveryCandidate[]
  spotify: DiscoveryCandidate[]
}

/**
 * Response from manual Bandcamp update endpoint
 */
export interface UpdateBandcampResponse {
  success: boolean
  artist?: Artist
  error?: string
}

/**
 * Response from manual Spotify update endpoint
 */
export interface UpdateSpotifyResponse {
  success: boolean
  artist?: Artist
  error?: string
}

/**
 * Hook for AI-powered music discovery (admin only). Returns candidate
 * Bandcamp + Spotify links for the admin to review and pick from; nothing
 * is saved by this call. Save happens via useUpdateArtistBandcamp /
 * useUpdateArtistSpotify after the admin picks a candidate.
 */
export function useDiscoverMusic() {
  return useMutation({
    mutationFn: async (artistId: number): Promise<DiscoverMusicResponse> => {
      let response: Response
      try {
        response = await fetch(
          `/api/admin/artists/${artistId}/discover-music`,
          {
            method: 'POST',
            credentials: 'include',
            // Backstop the route's own 60s bound so the spinner can't hang
            // indefinitely if the server itself becomes unreachable.
            signal: AbortSignal.timeout(70_000),
          }
        )
      } catch (err) {
        if (err instanceof DOMException && err.name === 'TimeoutError') {
          throw new Error('Discovery timed out — try again, or use manual entry.')
        }
        throw err
      }

      const data = await response.json()

      if (!response.ok) {
        throw new Error(data.message || data.error || 'Discovery failed')
      }

      return data
    },
  })
}

/**
 * Hook for manually updating artist Bandcamp URL (admin only)
 *
 * Usage:
 * ```tsx
 * const { mutate: updateBandcamp, isPending } = useUpdateArtistBandcamp()
 *
 * updateBandcamp(
 *   { artistId: 123, bandcampUrl: 'https://artist.bandcamp.com/album/test' },
 *   {
 *     onSuccess: () => toast.success('Bandcamp URL saved'),
 *     onError: (error) => toast.error(error.message)
 *   }
 * )
 * ```
 */
export function useUpdateArtistBandcamp() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async ({
      artistId,
      bandcampUrl,
    }: {
      artistId: number
      bandcampUrl: string
    }): Promise<UpdateBandcampResponse> => {
      const response = await fetch(`/api/admin/artists/${artistId}/bandcamp`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify({ bandcamp_url: bandcampUrl }),
      })

      const data = await response.json()

      if (!response.ok) {
        throw new Error(data.error || 'Update failed')
      }

      return data
    },
    onSuccess: () => {
      // Invalidate the artists prefix, NOT detail(numeric id): the artist page
      // caches under detail(slug), so a numeric-id key never matches. Mirrors
      // useArtistUpdate. (PSY-1109)
      queryClient.invalidateQueries({ queryKey: queryKeys.artists.all })
    },
  })
}

/**
 * Hook for clearing artist Bandcamp URL (admin only)
 *
 * Usage:
 * ```tsx
 * const { mutate: clearBandcamp, isPending } = useClearArtistBandcamp()
 *
 * clearBandcamp(artistId, {
 *   onSuccess: () => toast.success('Bandcamp URL cleared'),
 *   onError: (error) => toast.error(error.message)
 * })
 * ```
 */
export function useClearArtistBandcamp() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (artistId: number): Promise<UpdateBandcampResponse> => {
      const response = await fetch(`/api/admin/artists/${artistId}/bandcamp`, {
        method: 'DELETE',
        credentials: 'include',
      })

      const data = await response.json()

      if (!response.ok) {
        throw new Error(data.error || 'Clear failed')
      }

      return data
    },
    onSuccess: () => {
      // Invalidate the artists prefix, NOT detail(numeric id): the artist page
      // caches under detail(slug), so a numeric-id key never matches. Mirrors
      // useArtistUpdate. (PSY-1109)
      queryClient.invalidateQueries({ queryKey: queryKeys.artists.all })
    },
  })
}

/**
 * Hook for manually updating artist Spotify URL (admin only)
 *
 * Usage:
 * ```tsx
 * const { mutate: updateSpotify, isPending } = useUpdateArtistSpotify()
 *
 * updateSpotify(
 *   { artistId: 123, spotifyUrl: 'https://open.spotify.com/artist/abc123' },
 *   {
 *     onSuccess: () => toast.success('Spotify URL saved'),
 *     onError: (error) => toast.error(error.message)
 *   }
 * )
 * ```
 */
export function useUpdateArtistSpotify() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async ({
      artistId,
      spotifyUrl,
    }: {
      artistId: number
      spotifyUrl: string
    }): Promise<UpdateSpotifyResponse> => {
      const response = await fetch(`/api/admin/artists/${artistId}/spotify`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify({ spotify_url: spotifyUrl }),
      })

      const data = await response.json()

      if (!response.ok) {
        throw new Error(data.error || 'Update failed')
      }

      return data
    },
    onSuccess: () => {
      // Invalidate the artists prefix, NOT detail(numeric id): the artist page
      // caches under detail(slug), so a numeric-id key never matches. Mirrors
      // useArtistUpdate. (PSY-1109)
      queryClient.invalidateQueries({ queryKey: queryKeys.artists.all })
    },
  })
}

/**
 * Hook for clearing artist Spotify URL (admin only)
 *
 * Usage:
 * ```tsx
 * const { mutate: clearSpotify, isPending } = useClearArtistSpotify()
 *
 * clearSpotify(artistId, {
 *   onSuccess: () => toast.success('Spotify URL cleared'),
 *   onError: (error) => toast.error(error.message)
 * })
 * ```
 */
export function useClearArtistSpotify() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (artistId: number): Promise<UpdateSpotifyResponse> => {
      const response = await fetch(`/api/admin/artists/${artistId}/spotify`, {
        method: 'DELETE',
        credentials: 'include',
      })

      const data = await response.json()

      if (!response.ok) {
        throw new Error(data.error || 'Clear failed')
      }

      return data
    },
    onSuccess: () => {
      // Invalidate the artists prefix, NOT detail(numeric id): the artist page
      // caches under detail(slug), so a numeric-id key never matches. Mirrors
      // useArtistUpdate. (PSY-1109)
      queryClient.invalidateQueries({ queryKey: queryKeys.artists.all })
    },
  })
}

/**
 * Hook for updating artist details (admin only)
 *
 * Usage:
 * ```tsx
 * const { mutate: updateArtist, isPending } = useArtistUpdate()
 *
 * updateArtist(
 *   { artistId: 123, data: { name: 'New Name', city: 'Phoenix' } },
 *   {
 *     onSuccess: () => toast.success('Artist updated'),
 *     onError: (error) => toast.error(error.message)
 *   }
 * )
 * ```
 */
export function useArtistUpdate() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async ({
      artistId,
      data,
    }: {
      artistId: number
      data: ArtistEditRequest
    }): Promise<Artist> => {
      return apiRequest<Artist>(
        API_ENDPOINTS.ADMIN.ARTISTS.UPDATE(artistId),
        {
          method: 'PATCH',
          body: JSON.stringify(data),
        }
      )
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.artists.detail(variables.artistId),
      })
      queryClient.invalidateQueries({
        queryKey: queryKeys.artists.all,
      })
      queryClient.invalidateQueries({
        queryKey: queryKeys.shows.all,
      })
    },
  })
}

/**
 * Hook for fetching artist aliases
 */
export function useArtistAliases(artistId: number, enabled = true) {
  return useQuery({
    queryKey: queryKeys.artists.aliases(artistId),
    queryFn: () =>
      apiRequest<ArtistAliasesResponse>(API_ENDPOINTS.ARTISTS.ALIASES(artistId)),
    enabled: enabled && artistId > 0,
  })
}

/**
 * Hook for creating an artist alias (admin only)
 */
export function useCreateArtistAlias() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async ({
      artistId,
      alias,
    }: {
      artistId: number
      alias: string
    }): Promise<ArtistAlias> => {
      return apiRequest<ArtistAlias>(
        API_ENDPOINTS.ADMIN.ARTISTS.ALIASES(artistId),
        {
          method: 'POST',
          body: JSON.stringify({ alias }),
        }
      )
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.artists.aliases(variables.artistId),
      })
    },
  })
}

/**
 * Hook for deleting an artist alias (admin only)
 */
export function useDeleteArtistAlias() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async ({
      artistId,
      aliasId,
    }: {
      artistId: number
      aliasId: number
    }): Promise<void> => {
      await apiRequest(
        API_ENDPOINTS.ADMIN.ARTISTS.DELETE_ALIAS(artistId, aliasId),
        { method: 'DELETE' }
      )
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.artists.aliases(variables.artistId),
      })
    },
  })
}

/**
 * Hook for merging two artists (admin only)
 */
export function useMergeArtists() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async ({
      canonicalArtistId,
      mergeFromArtistId,
    }: {
      canonicalArtistId: number
      mergeFromArtistId: number
    }): Promise<MergeArtistResult> => {
      return apiRequest<MergeArtistResult>(
        API_ENDPOINTS.ADMIN.ARTISTS.MERGE,
        {
          method: 'POST',
          body: JSON.stringify({
            canonical_artist_id: canonicalArtistId,
            merge_from_artist_id: mergeFromArtistId,
          }),
        }
      )
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.artists.all })
      queryClient.invalidateQueries({ queryKey: queryKeys.shows.all })
    },
  })
}
