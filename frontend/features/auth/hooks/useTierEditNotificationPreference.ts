'use client'

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import { queryKeys } from '@/lib/queryClient'

/**
 * Update payload for the tier-change / edit-review email preferences.
 *
 * Both fields are optional so a caller can flip one toggle without touching
 * the other — the backend `PATCH /auth/preferences/tier-edit-notifications`
 * endpoint applies one update per non-undefined field (PSY-756).
 */
export interface TierEditNotificationUpdate {
  notify_on_tier_notifications?: boolean
  notify_on_edit_notifications?: boolean
}

interface SetTierEditNotificationsResponse {
  success: boolean
  notify_on_tier_notifications: boolean
  notify_on_edit_notifications: boolean
}

/**
 * Mutation hook to toggle the tier-change and edit-review email preferences.
 *
 * PSY-756 (backend) / PSY-807 (this UI). Both server columns default to TRUE
 * (opt-OUT) — these emails are one per discrete action, so users opt OUT from
 * the notification-settings page (or the one-click unsubscribe link in the
 * email). Mirrors `useSetCollectionDigestPreference`, but the request body
 * carries the named preference field(s) instead of a single `enabled` flag
 * because the endpoint updates two independent preferences.
 *
 * Invalidates the profile query on success so each toggle's `checked` value
 * reflects the new server state without a manual refetch.
 */
export const useSetTierEditNotificationPreference = () => {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (
      update: TierEditNotificationUpdate
    ): Promise<SetTierEditNotificationsResponse> => {
      return apiRequest<SetTierEditNotificationsResponse>(
        API_ENDPOINTS.AUTH.TIER_EDIT_NOTIFICATIONS,
        {
          method: 'PATCH',
          body: JSON.stringify(update),
        }
      )
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.auth.profile })
    },
  })
}
