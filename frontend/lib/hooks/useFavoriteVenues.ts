'use client'

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '../api'
import { queryKeys, createInvalidateQueries } from '../queryClient'
import type {
  FavoriteVenuesListResponse,
  FavoriteVenueActionResponse,
  CheckFavoritedResponse,
  FavoriteVenueShowsResponse,
} from '../types/venue'

interface UseFavoriteVenuesOptions {
  limit?: number
  offset?: number
  enabled?: boolean
}

/**
 * Hook to fetch user's favorite venues
 * Requires authentication
 */
export const useFavoriteVenues = (options: UseFavoriteVenuesOptions = {}) => {
  const { limit = 50, offset = 0, enabled = true } = options

  const params = new URLSearchParams()
  params.set('limit', limit.toString())
  params.set('offset', offset.toString())

  const endpoint = `${API_ENDPOINTS.FAVORITE_VENUES.LIST}?${params.toString()}`

  return useQuery({
    queryKey: queryKeys.favoriteVenues.list(),
    queryFn: async (): Promise<FavoriteVenuesListResponse> => {
      return apiRequest<FavoriteVenuesListResponse>(endpoint, {
        method: 'GET',
      })
    },
    enabled,
    staleTime: 5 * 60 * 1000, // 5 minutes
  })
}

/**
 * Hook to check if a specific venue is favorited
 * Requires authentication
 */
export const useIsVenueFavorited = (venueId: number | string | null, isAuthenticated: boolean) => {
  return useQuery({
    queryKey: queryKeys.favoriteVenues.check(String(venueId)),
    queryFn: async (): Promise<CheckFavoritedResponse> => {
      return apiRequest<CheckFavoritedResponse>(
        API_ENDPOINTS.FAVORITE_VENUES.CHECK(venueId!),
        {
          method: 'GET',
        }
      )
    },
    enabled: Boolean(venueId) && isAuthenticated,
    staleTime: 30 * 1000, // 30 seconds (shorter since favorite state can change)
  })
}

/**
 * Hook to favorite a venue
 * Requires authentication
 */
export const useFavoriteVenue = () => {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async (venueId: number): Promise<FavoriteVenueActionResponse> => {
      return apiRequest<FavoriteVenueActionResponse>(
        API_ENDPOINTS.FAVORITE_VENUES.FAVORITE(venueId),
        {
          method: 'POST',
        }
      )
    },
    onSuccess: (_data, venueId) => {
      // Invalidate favorite venues list
      invalidateQueries.favoriteVenues()
      // Invalidate the specific venue's favorited status
      queryClient.invalidateQueries({
        queryKey: queryKeys.favoriteVenues.check(String(venueId)),
      })
    },
  })
}

/**
 * Hook to unfavorite (remove) a venue from user's favorites
 * Requires authentication
 */
export const useUnfavoriteVenue = () => {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async (venueId: number): Promise<FavoriteVenueActionResponse> => {
      return apiRequest<FavoriteVenueActionResponse>(
        API_ENDPOINTS.FAVORITE_VENUES.UNFAVORITE(venueId),
        {
          method: 'DELETE',
        }
      )
    },
    onSuccess: (_data, venueId) => {
      // Invalidate favorite venues list
      invalidateQueries.favoriteVenues()
      // Invalidate the specific venue's favorited status
      queryClient.invalidateQueries({
        queryKey: queryKeys.favoriteVenues.check(String(venueId)),
      })
    },
  })
}

/**
 * Combined hook that provides favorite/unfavorite toggle functionality
 * Includes optimistic updates for better UX
 */
export const useFavoriteVenueToggle = (venueId: number, isAuthenticated: boolean) => {
  const queryClient = useQueryClient()
  const { data: favoritedStatus } = useIsVenueFavorited(venueId, isAuthenticated)
  const favoriteVenue = useFavoriteVenue()
  const unfavoriteVenue = useUnfavoriteVenue()

  const isFavorited = favoritedStatus?.is_favorited ?? false
  const isLoading = favoriteVenue.isPending || unfavoriteVenue.isPending

  const toggle = async () => {
    // Optimistic update
    const checkQueryKey = queryKeys.favoriteVenues.check(String(venueId))

    queryClient.setQueryData(checkQueryKey, {
      is_favorited: !isFavorited,
    })

    try {
      if (isFavorited) {
        await unfavoriteVenue.mutateAsync(venueId)
      } else {
        await favoriteVenue.mutateAsync(venueId)
      }
    } catch (error) {
      // Rollback on error
      queryClient.setQueryData(checkQueryKey, {
        is_favorited: isFavorited,
      })
      throw error
    }
  }

  return {
    isFavorited,
    isLoading,
    toggle,
    error: favoriteVenue.error || unfavoriteVenue.error,
  }
}

interface UseFavoriteVenueShowsOptions {
  timezone?: string
  limit?: number
  offset?: number
  enabled?: boolean
}

/**
 * Hook to fetch upcoming shows from user's favorite venues
 * Requires authentication
 */
export const useFavoriteVenueShows = (
  options: UseFavoriteVenueShowsOptions = {}
) => {
  const {
    timezone = Intl.DateTimeFormat().resolvedOptions().timeZone,
    limit = 50,
    offset = 0,
    enabled = true,
  } = options

  const params = new URLSearchParams()
  params.set('timezone', timezone)
  params.set('limit', limit.toString())
  params.set('offset', offset.toString())

  const endpoint = `${API_ENDPOINTS.FAVORITE_VENUES.SHOWS}?${params.toString()}`

  return useQuery({
    queryKey: queryKeys.favoriteVenues.shows({ timezone, limit, offset }),
    queryFn: async (): Promise<FavoriteVenueShowsResponse> => {
      return apiRequest<FavoriteVenueShowsResponse>(endpoint, {
        method: 'GET',
      })
    },
    enabled,
    staleTime: 5 * 60 * 1000, // 5 minutes
  })
}
