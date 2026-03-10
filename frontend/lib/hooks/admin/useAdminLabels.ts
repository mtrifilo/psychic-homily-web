'use client'

/**
 * Admin Label Hooks
 *
 * TanStack Query mutations for admin label CRUD operations:
 * create, update, and delete labels.
 */

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '../../api'
import { createInvalidateQueries } from '../../queryClient'
import type { LabelDetail } from '../../types/label'

// ============================================================================
// Request Types
// ============================================================================

export interface CreateLabelInput {
  name: string
  city?: string | null
  state?: string | null
  country?: string | null
  founded_year?: number | null
  status?: string
  description?: string | null
  instagram?: string | null
  facebook?: string | null
  twitter?: string | null
  youtube?: string | null
  spotify?: string | null
  soundcloud?: string | null
  bandcamp?: string | null
  website?: string | null
}

export interface UpdateLabelInput {
  name?: string
  city?: string | null
  state?: string | null
  country?: string | null
  founded_year?: number | null
  status?: string | null
  description?: string | null
  instagram?: string | null
  facebook?: string | null
  twitter?: string | null
  youtube?: string | null
  spotify?: string | null
  soundcloud?: string | null
  bandcamp?: string | null
  website?: string | null
}

// ============================================================================
// Mutations
// ============================================================================

/**
 * Hook for creating a new label (admin only)
 */
export function useCreateLabel() {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async (input: CreateLabelInput): Promise<LabelDetail> => {
      return apiRequest<LabelDetail>(API_ENDPOINTS.LABELS.CREATE, {
        method: 'POST',
        body: JSON.stringify(input),
      })
    },
    onSuccess: () => {
      invalidateQueries.labels()
    },
  })
}

/**
 * Hook for updating an existing label (admin only)
 */
export function useUpdateLabel() {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async ({
      labelId,
      data,
    }: {
      labelId: number
      data: UpdateLabelInput
    }): Promise<LabelDetail> => {
      return apiRequest<LabelDetail>(
        API_ENDPOINTS.LABELS.UPDATE(labelId),
        {
          method: 'PUT',
          body: JSON.stringify(data),
        }
      )
    },
    onSuccess: () => {
      invalidateQueries.labels()
    },
  })
}

/**
 * Hook for deleting a label (admin only)
 */
export function useDeleteLabel() {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async (labelId: number): Promise<void> => {
      return apiRequest<void>(API_ENDPOINTS.LABELS.DELETE(labelId), {
        method: 'DELETE',
      })
    },
    onSuccess: () => {
      invalidateQueries.labels()
    },
  })
}
