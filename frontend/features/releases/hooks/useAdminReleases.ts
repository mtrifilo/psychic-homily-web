'use client'

/**
 * Admin Release Hooks
 *
 * TanStack Query mutations for admin release CRUD operations:
 * create, update, delete releases, and manage external links.
 */

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest } from '@/lib/api'
import { queryKeys } from '@/lib/queryClient'
import { refetchAfterMutation } from '@/lib/refetchAfterMutation'
import { releaseEndpoints } from '@/features/releases/api'
import type { ReleaseDetail, ReleaseExternalLink } from '../types'

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

  return useMutation({
    mutationFn: async (input: CreateReleaseInput): Promise<ReleaseDetail> => {
      return apiRequest<ReleaseDetail>(releaseEndpoints.CREATE, {
        method: 'POST',
        body: JSON.stringify(input),
      })
    },
    onSuccess: () => {
      void refetchAfterMutation(queryClient, queryKeys.releases.all)
    },
  })
}

/**
 * Hook for updating an existing release (admin only)
 */
export function useUpdateRelease() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async ({
      releaseId,
      data,
    }: {
      releaseId: number
      data: UpdateReleaseInput
    }): Promise<ReleaseDetail> => {
      return apiRequest<ReleaseDetail>(releaseEndpoints.UPDATE(releaseId), {
        method: 'PUT',
        body: JSON.stringify(data),
      })
    },
    onSuccess: () => {
      void refetchAfterMutation(queryClient, queryKeys.releases.all)
    },
  })
}

/**
 * Hook for deleting a release (admin only)
 */
export function useDeleteRelease() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (releaseId: number): Promise<void> => {
      return apiRequest<void>(releaseEndpoints.DELETE(releaseId), {
        method: 'DELETE',
      })
    },
    onSuccess: () => {
      void refetchAfterMutation(queryClient, queryKeys.releases.all)
    },
  })
}

/**
 * Hook for adding an external link to a release (admin only)
 */
export function useAddReleaseLink() {
  const queryClient = useQueryClient()

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
        releaseEndpoints.ADD_LINK(releaseId),
        {
          method: 'POST',
          body: JSON.stringify({ platform, url }),
        }
      )
    },
    onSuccess: () => {
      void refetchAfterMutation(queryClient, queryKeys.releases.all)
    },
  })
}

/**
 * Hook for removing an external link from a release (admin only)
 */
export function useRemoveReleaseLink() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async ({
      releaseId,
      linkId,
    }: {
      releaseId: number
      linkId: number
    }): Promise<void> => {
      return apiRequest<void>(releaseEndpoints.REMOVE_LINK(releaseId, linkId), {
        method: 'DELETE',
      })
    },
    onSuccess: () => {
      void refetchAfterMutation(queryClient, queryKeys.releases.all)
    },
  })
}
