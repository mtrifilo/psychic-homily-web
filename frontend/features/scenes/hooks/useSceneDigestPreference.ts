'use client'

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import { queryKeys } from '@/lib/queryClient'

interface SetSceneDigestResponse {
  success: boolean
  notify_on_scene_digest: boolean
}

/**
 * Mutation hook to toggle the weekly scene-digest email preference (PSY-1342).
 *
 * Server default is OFF (opt-IN) — the same bulk-sender anti-spam policy as the
 * collection digest, so following a scene doesn't silently start a recurring
 * email. Mirrors useSetCollectionDigestPreference so the settings row drops in
 * identically. Invalidates the profile query on success so the toggle's
 * `checked` value reflects the new server state without a manual refetch.
 */
export const useSetSceneDigestPreference = () => {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (enabled: boolean): Promise<SetSceneDigestResponse> => {
      return apiRequest<SetSceneDigestResponse>(API_ENDPOINTS.AUTH.SCENE_DIGEST, {
        method: 'PATCH',
        body: JSON.stringify({ enabled }),
      })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.auth.profile })
    },
  })
}
