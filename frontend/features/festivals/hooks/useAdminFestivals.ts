'use client'

/**
 * Admin Festival Hooks
 *
 * TanStack Query mutations for admin festival CRUD operations:
 * create, update, delete festivals, manage lineup and venues.
 */

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest } from '@/lib/api'
import { createInvalidateQueries } from '@/lib/queryClient'
import { festivalEndpoints } from '../api'
import type {
  FestivalDetail,
  FestivalArtist,
  FestivalVenue,
} from '../types'

// ============================================================================
// Request Types
// ============================================================================

export interface CreateFestivalInput {
  name: string
  series_slug: string
  edition_year: number
  description?: string | null
  location_name?: string | null
  city?: string | null
  state?: string | null
  country?: string | null
  start_date: string
  end_date: string
  website?: string | null
  ticket_url?: string | null
  flyer_url?: string | null
  status?: string
}

export interface UpdateFestivalInput {
  name?: string
  series_slug?: string
  edition_year?: number
  description?: string | null
  location_name?: string | null
  city?: string | null
  state?: string | null
  country?: string | null
  start_date?: string
  end_date?: string
  website?: string | null
  ticket_url?: string | null
  flyer_url?: string | null
  status?: string
}

export interface AddFestivalArtistInput {
  artist_id: number
  billing_tier?: string
  position?: number
  day_date?: string | null
  stage?: string | null
  set_time?: string | null
  venue_id?: number | null
}

export interface UpdateFestivalArtistInput {
  billing_tier?: string
  position?: number
  day_date?: string | null
  stage?: string | null
  set_time?: string | null
  venue_id?: number | null
}

export interface AddFestivalVenueInput {
  venue_id: number
  is_primary?: boolean
}

// ============================================================================
// Festival CRUD Mutations
// ============================================================================

/**
 * Hook for creating a new festival (admin only)
 */
export function useCreateFestival() {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async (input: CreateFestivalInput): Promise<FestivalDetail> => {
      return apiRequest<FestivalDetail>(festivalEndpoints.CREATE, {
        method: 'POST',
        body: JSON.stringify(input),
      })
    },
    onSuccess: () => {
      invalidateQueries.festivals()
    },
  })
}

/**
 * Hook for updating an existing festival (admin only)
 */
export function useUpdateFestival() {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async ({
      festivalId,
      data,
    }: {
      festivalId: number
      data: UpdateFestivalInput
    }): Promise<FestivalDetail> => {
      return apiRequest<FestivalDetail>(
        festivalEndpoints.UPDATE(festivalId),
        {
          method: 'PUT',
          body: JSON.stringify(data),
        }
      )
    },
    onSuccess: () => {
      invalidateQueries.festivals()
    },
  })
}

/**
 * Hook for deleting a festival (admin only)
 */
export function useDeleteFestival() {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async (festivalId: number): Promise<void> => {
      return apiRequest<void>(festivalEndpoints.DELETE(festivalId), {
        method: 'DELETE',
      })
    },
    onSuccess: () => {
      invalidateQueries.festivals()
    },
  })
}

// ============================================================================
// Festival Lineup Mutations
// ============================================================================

/**
 * Hook for adding an artist to a festival lineup (admin only)
 */
export function useAddFestivalArtist() {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async ({
      festivalId,
      data,
    }: {
      festivalId: number
      data: AddFestivalArtistInput
    }): Promise<FestivalArtist> => {
      return apiRequest<FestivalArtist>(
        festivalEndpoints.ADD_ARTIST(festivalId),
        {
          method: 'POST',
          body: JSON.stringify(data),
        }
      )
    },
    onSuccess: () => {
      invalidateQueries.festivals()
    },
  })
}

/**
 * Hook for updating a festival artist entry (admin only)
 */
export function useUpdateFestivalArtist() {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async ({
      festivalId,
      artistId,
      data,
    }: {
      festivalId: number
      artistId: number
      data: UpdateFestivalArtistInput
    }): Promise<FestivalArtist> => {
      return apiRequest<FestivalArtist>(
        festivalEndpoints.UPDATE_ARTIST(festivalId, artistId),
        {
          method: 'PUT',
          body: JSON.stringify(data),
        }
      )
    },
    onSuccess: () => {
      invalidateQueries.festivals()
    },
  })
}

/**
 * Hook for removing an artist from a festival lineup (admin only)
 */
export function useRemoveFestivalArtist() {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async ({
      festivalId,
      artistId,
    }: {
      festivalId: number
      artistId: number
    }): Promise<void> => {
      return apiRequest<void>(
        festivalEndpoints.REMOVE_ARTIST(festivalId, artistId),
        { method: 'DELETE' }
      )
    },
    onSuccess: () => {
      invalidateQueries.festivals()
    },
  })
}

// ============================================================================
// Festival Venue Mutations
// ============================================================================

/**
 * Hook for adding a venue to a festival (admin only)
 */
export function useAddFestivalVenue() {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async ({
      festivalId,
      data,
    }: {
      festivalId: number
      data: AddFestivalVenueInput
    }): Promise<FestivalVenue> => {
      return apiRequest<FestivalVenue>(
        festivalEndpoints.ADD_VENUE(festivalId),
        {
          method: 'POST',
          body: JSON.stringify(data),
        }
      )
    },
    onSuccess: () => {
      invalidateQueries.festivals()
    },
  })
}

/**
 * Hook for removing a venue from a festival (admin only)
 */
export function useRemoveFestivalVenue() {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async ({
      festivalId,
      venueId,
    }: {
      festivalId: number
      venueId: number
    }): Promise<void> => {
      return apiRequest<void>(
        festivalEndpoints.REMOVE_VENUE(festivalId, venueId),
        { method: 'DELETE' }
      )
    },
    onSuccess: () => {
      invalidateQueries.festivals()
    },
  })
}
