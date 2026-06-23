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
 * Region confidence tier returned by the discover-music endpoint (PSY-1191).
 * `high` = the MusicBrainz candidate's geography aligned with a PH show region;
 * `review` = region mismatch, non-US, or no PH region to compare — a touring act
 * or namesake the admin should verify before linking. `review` is NEVER a gate:
 * the candidate is still returned and the admin can still accept it.
 */
export type MusicConfidence = 'high' | 'review'

/**
 * One discovered streaming-link candidate. Mirrors the LOCKED backend wire
 * contract `contracts.MusicLinkCandidate` (PSY-1191). Discovery is review-only:
 * the admin picks a candidate and saves it via the existing bandcamp/spotify
 * paths — nothing here is persisted by the discover call.
 */
export interface MusicLinkCandidate {
  platform: MusicPlatform
  url: string
  source: string
  mb_artist_id: string
  mb_artist_name: string
  confidence: MusicConfidence
  region_match: boolean
  live: boolean
  /** Optional reviewer note (touring-act / namesake caveat, MB disambiguation). */
  notes?: string
}

/**
 * Response from the discover-music endpoint (PSY-1191). Discovery returns
 * CANDIDATES only; the admin reviews and picks before any save happens. The
 * candidates carry their own `platform` field — they are NOT pre-grouped by the
 * backend, so the UI groups them.
 */
export interface DiscoverMusicResponse {
  artist_id: number
  candidates: MusicLinkCandidate[]
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
 * Hook for MusicBrainz-backed music discovery (admin only, PSY-1191). Returns
 * candidate Bandcamp + Spotify links for the admin to review and pick from;
 * nothing is saved by this call. Save happens via the bandcamp/spotify accept
 * paths in AdminMusicControls after the admin picks a candidate.
 *
 * The POST flows through the catch-all API proxy (frontend/app/api/[...path])
 * to the admin-gated backend endpoint, which does the MusicBrainz lookups and
 * SSRF-guarded liveness probes (server-side ~55s bound). Errors are surfaced as
 * Huma `detail` strings (the backend error shape); the client backstop timeout
 * maps to friendly copy.
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
            // Backstop the backend's own ~55s bound so the spinner can't hang
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

      // The body may not be JSON on an upstream-gateway failure: the catch-all
      // proxy re-emits the backend response verbatim, so a 502/504 can carry an
      // HTML or empty body. Parse defensively so a non-ok status surfaces as a
      // status-based message instead of a masking "Unexpected token" parse error.
      const data = await response.json().catch(() => null)

      if (!response.ok) {
        // Huma errors carry `detail`; the proxy 502 carries `error`.
        throw new Error(
          data?.detail ||
            data?.message ||
            data?.error ||
            `Discovery failed (HTTP ${response.status})`
        )
      }

      return data as DiscoverMusicResponse
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
      // Invalidate the artists prefix. These hooks know the numeric id, but the
      // page caches the artist under detail(slug) — detail(id) and detail(slug)
      // are different VALUES (detail() stringifies either, so it's not a
      // key-shape difference), so an id-keyed invalidation never matches. The
      // prefix does, regardless of id-vs-slug. (PSY-1109)
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
      // Invalidate the artists prefix. These hooks know the numeric id, but the
      // page caches the artist under detail(slug) — detail(id) and detail(slug)
      // are different VALUES (detail() stringifies either, so it's not a
      // key-shape difference), so an id-keyed invalidation never matches. The
      // prefix does, regardless of id-vs-slug. (PSY-1109)
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
      // Invalidate the artists prefix. These hooks know the numeric id, but the
      // page caches the artist under detail(slug) — detail(id) and detail(slug)
      // are different VALUES (detail() stringifies either, so it's not a
      // key-shape difference), so an id-keyed invalidation never matches. The
      // prefix does, regardless of id-vs-slug. (PSY-1109)
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
      // Invalidate the artists prefix. These hooks know the numeric id, but the
      // page caches the artist under detail(slug) — detail(id) and detail(slug)
      // are different VALUES (detail() stringifies either, so it's not a
      // key-shape difference), so an id-keyed invalidation never matches. The
      // prefix does, regardless of id-vs-slug. (PSY-1109)
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
    onSuccess: () => {
      // A rename changes the artist wherever it's denormalised, so invalidate
      // the whole artists prefix (which covers detail-by-slug) and shows. No
      // detail(id) line: these hooks know the numeric id, but the page caches
      // under detail(slug) — different VALUES, so an id-keyed invalidation never
      // matches; the prefix matches regardless. (PSY-1109)
      queryClient.invalidateQueries({ queryKey: queryKeys.artists.all })
      queryClient.invalidateQueries({ queryKey: queryKeys.shows.all })
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
