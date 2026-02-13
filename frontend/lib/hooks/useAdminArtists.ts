'use client'

/**
 * Admin Artists Hooks
 *
 * TanStack Query hooks for admin artist operations, including
 * music discovery (Bandcamp/Spotify) and manual URL updates.
 */

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '../api'
import { queryKeys } from '../queryClient'
import type { Artist, ArtistEditRequest } from '../types/artist'

/**
 * Platform types for music discovery
 */
export type MusicPlatform = 'bandcamp' | 'spotify'

/**
 * Response from music discovery endpoint
 */
export interface DiscoverMusicResponse {
  success: boolean
  platform?: MusicPlatform
  url?: string
  artist?: Artist
  error?: string
  message?: string
  discovered_url?: string
  platforms?: {
    bandcamp: { found: boolean; url?: string; error?: string }
    spotify: { found: boolean; url?: string; error?: string }
  }
}

/**
 * Response from Bandcamp discovery endpoint (legacy alias)
 */
export interface DiscoverBandcampResponse {
  success: boolean
  bandcamp_url?: string
  artist?: Artist
  error?: string
  message?: string
  discovered_url?: string
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
 * Hook for AI-powered music discovery (admin only)
 * Searches Bandcamp and Spotify in parallel, saves both when found
 *
 * Usage:
 * ```tsx
 * const { mutate: discoverMusic, isPending } = useDiscoverMusic()
 *
 * discoverMusic(artistId, {
 *   onSuccess: (data) => {
 *     toast.success(`Found on ${data.platform}: ${data.url}`)
 *   },
 *   onError: (error) => {
 *     toast.error(error.message)
 *   }
 * })
 * ```
 */
export function useDiscoverMusic() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (artistId: number): Promise<DiscoverMusicResponse> => {
      const response = await fetch(
        `/api/admin/artists/${artistId}/discover-music`,
        {
          method: 'POST',
          credentials: 'include',
        }
      )

      const data = await response.json()

      if (!response.ok) {
        throw new Error(data.message || data.error || 'Discovery failed')
      }

      return data
    },
    onSuccess: (data, artistId) => {
      // Invalidate artist query to refresh the embed
      queryClient.invalidateQueries({
        queryKey: queryKeys.artists.detail(artistId),
      })
    },
  })
}

/**
 * Hook for AI-powered Bandcamp album discovery (admin only)
 * @deprecated Use useDiscoverMusic instead for cascading discovery
 *
 * Usage:
 * ```tsx
 * const { mutate: discoverBandcamp, isPending } = useDiscoverBandcamp()
 *
 * discoverBandcamp(artistId, {
 *   onSuccess: (data) => {
 *     toast.success(`Found: ${data.bandcamp_url}`)
 *   },
 *   onError: (error) => {
 *     toast.error(error.message)
 *   }
 * })
 * ```
 */
export function useDiscoverBandcamp() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (artistId: number): Promise<DiscoverBandcampResponse> => {
      const response = await fetch(
        `/api/admin/artists/${artistId}/discover-bandcamp`,
        {
          method: 'POST',
          credentials: 'include',
        }
      )

      const data = await response.json()

      if (!response.ok) {
        throw new Error(data.message || data.error || 'Discovery failed')
      }

      return data
    },
    onSuccess: (data, artistId) => {
      // Invalidate artist query to refresh the embed
      queryClient.invalidateQueries({
        queryKey: queryKeys.artists.detail(artistId),
      })
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
    onSuccess: (data, variables) => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.artists.detail(variables.artistId),
      })
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
    onSuccess: (data, artistId) => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.artists.detail(artistId),
      })
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
    onSuccess: (data, variables) => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.artists.detail(variables.artistId),
      })
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
    onSuccess: (data, artistId) => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.artists.detail(artistId),
      })
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
    },
  })
}
