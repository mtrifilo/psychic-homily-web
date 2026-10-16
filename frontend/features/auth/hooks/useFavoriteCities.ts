'use client'

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import { queryKeys } from '@/lib/queryClient'

interface FavoriteCity {
  city: string
  state: string
}

interface SetFavoriteCitiesResponse {
  success: boolean
  message: string
  cities: FavoriteCity[]
}

/**
 * Mutation hook to save favorite cities.
 * Invalidates profile query on success so the new favorites propagate.
 */
export const useSetFavoriteCities = () => {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (cities: FavoriteCity[]): Promise<SetFavoriteCitiesResponse> => {
      return apiRequest<SetFavoriteCitiesResponse>(
        API_ENDPOINTS.AUTH.FAVORITE_CITIES,
        {
          method: 'PUT',
          body: JSON.stringify({ cities }),
        }
      )
    },
    onSuccess: () => {
      // eslint-disable-next-line no-restricted-syntax -- runs only for a signed-in viewer, whose profile has loaded, and the cache error handlers in lib/queryClient.ts coordinate this query's refetches
      queryClient.invalidateQueries({ queryKey: queryKeys.auth.profile })
    },
  })
}
