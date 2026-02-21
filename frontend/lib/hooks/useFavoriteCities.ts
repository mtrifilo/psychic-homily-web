'use client'

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '../api'
import { queryKeys } from '../queryClient'

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
      queryClient.invalidateQueries({ queryKey: queryKeys.auth.profile })
    },
  })
}
