'use client'

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import { queryKeys } from '@/lib/queryClient'

interface SetCollectionDigestResponse {
  success: boolean
  notify_on_collection_digest: boolean
}

/**
 * Mutation hook to toggle the weekly collection-digest email preference.
 *
 * PSY-350 / PSY-515. Defaults to OFF on the server (opt-IN) — users discover
 * and enable this from the notification-preferences page. Mirrors the shape
 * of useSetShowReminders so the UI can drop in identically.
 *
 * Invalidates the profile query on success so the toggle's `checked` value
 * reflects the new server state without a manual refetch.
 */
export const useSetCollectionDigestPreference = () => {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (
      enabled: boolean
    ): Promise<SetCollectionDigestResponse> => {
      return apiRequest<SetCollectionDigestResponse>(
        API_ENDPOINTS.AUTH.COLLECTION_DIGEST,
        {
          method: 'PATCH',
          body: JSON.stringify({ enabled }),
        }
      )
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.auth.profile })
    },
  })
}
