'use client'

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import { queryKeys } from '@/lib/queryClient'

interface SetShowRemindersResponse {
  success: boolean
  show_reminders: boolean
}

/**
 * Mutation hook to toggle show reminders.
 * Invalidates profile query on success so the setting propagates.
 */
export const useSetShowReminders = () => {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (enabled: boolean): Promise<SetShowRemindersResponse> => {
      return apiRequest<SetShowRemindersResponse>(
        API_ENDPOINTS.AUTH.SHOW_REMINDERS,
        {
          method: 'PATCH',
          body: JSON.stringify({ enabled }),
        }
      )
    },
    onSuccess: () => {
      // eslint-disable-next-line no-restricted-syntax -- runs only for a signed-in viewer, whose profile has loaded, and the cache error handlers in lib/queryClient.ts coordinate this query's refetches
      queryClient.invalidateQueries({ queryKey: queryKeys.auth.profile })
    },
  })
}
