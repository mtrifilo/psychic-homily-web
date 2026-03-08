'use client'

/**
 * Admin Release Hooks
 *
 * TanStack Query mutations for admin release CRUD operations:
 * create, update, delete releases, and manage external links.
 */

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '../api'
import { createInvalidateQueries } from '../queryClient'
import type { ReleaseDetail, ReleaseExternalLink } from '../types/release'

// ============================================================================
// Request Types
// ============================================================================

export interface CreateReleaseArtistInput {
  artist_id: number
  role: string
}

export interface CreateReleaseLinkInput {
  platform: string
  url: string
}

export interface CreateReleaseInput {
  title: string
  release_type?: string
  release_year?: number | null
  release_date?: string | null
  cover_art_url?: string | null
  description?: string | null
  artists?: CreateReleaseArtistInput[]
  external_links?: CreateReleaseLinkInput[]
}

export interface UpdateReleaseInput {
  title?: string
  release_type?: string
  release_year?: number | null
  release_date?: string | null
  cover_art_url?: string | null
  description?: string | null
}

// ============================================================================
// Mutations
// ============================================================================

/**
 * Hook for creating a new release (admin only)
 */
export function useCreateRelease() {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async (input: CreateReleaseInput): Promise<ReleaseDetail> => {
      return apiRequest<ReleaseDetail>(API_ENDPOINTS.RELEASES.CREATE, {
        method: 'POST',
        body: JSON.stringify(input),
      })
    },
    onSuccess: () => {
      invalidateQueries.releases()
    },
  })
}

/**
 * Hook for updating an existing release (admin only)
 */
export function useUpdateRelease() {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async ({
      releaseId,
      data,
    }: {
      releaseId: number
      data: UpdateReleaseInput
    }): Promise<ReleaseDetail> => {
      return apiRequest<ReleaseDetail>(
        API_ENDPOINTS.RELEASES.UPDATE(releaseId),
        {
          method: 'PUT',
          body: JSON.stringify(data),
        }
      )
    },
    onSuccess: () => {
      invalidateQueries.releases()
    },
  })
}

/**
 * Hook for deleting a release (admin only)
 */
export function useDeleteRelease() {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async (releaseId: number): Promise<void> => {
      return apiRequest<void>(API_ENDPOINTS.RELEASES.DELETE(releaseId), {
        method: 'DELETE',
      })
    },
    onSuccess: () => {
      invalidateQueries.releases()
    },
  })
}

/**
 * Hook for adding an external link to a release (admin only)
 */
export function useAddReleaseLink() {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async ({
      releaseId,
      platform,
      url,
    }: {
      releaseId: number
      platform: string
      url: string
    }): Promise<ReleaseExternalLink> => {
      return apiRequest<ReleaseExternalLink>(
        API_ENDPOINTS.RELEASES.ADD_LINK(releaseId),
        {
          method: 'POST',
          body: JSON.stringify({ platform, url }),
        }
      )
    },
    onSuccess: () => {
      invalidateQueries.releases()
    },
  })
}

/**
 * Hook for removing an external link from a release (admin only)
 */
export function useRemoveReleaseLink() {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async ({
      releaseId,
      linkId,
    }: {
      releaseId: number
      linkId: number
    }): Promise<void> => {
      return apiRequest<void>(
        API_ENDPOINTS.RELEASES.REMOVE_LINK(releaseId, linkId),
        {
          method: 'DELETE',
        }
      )
    },
    onSuccess: () => {
      invalidateQueries.releases()
    },
  })
}
