import { useQuery } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import { queryKeys } from '@/lib/queryClient'

interface RevisionItem {
  id: number
  user_id: number
  user_name?: string
  user_username?: string | null
  created_at: string
}

interface EntityHistoryResponse {
  revisions: RevisionItem[]
  total: number
}

export interface EntityAttribution {
  /** Resolved display name; never empty (backend resolveUserName chain). */
  userName: string
  /** URL-safe username slug; null when the user has no username set. */
  userUsername: string | null
  createdAt: string
}

/**
 * Fetches the most recent revision for an entity to show "Last edited by" attribution.
 * Returns the most recent editor's display name and (when set) linkable username.
 * Returns null data if no revisions exist.
 */
export function useEntityAttribution(
  entityType: string,
  entityId: string | number,
  options?: { enabled?: boolean }
) {
  return useQuery({
    queryKey: [...queryKeys.revisions.entity(entityType, entityId), { attribution: true }],
    queryFn: async (): Promise<EntityAttribution | null> => {
      const url = `${API_ENDPOINTS.REVISIONS.ENTITY_HISTORY(entityType, entityId)}?limit=1&offset=0`
      const data = await apiRequest<EntityHistoryResponse>(url)
      if (!data.revisions || data.revisions.length === 0) {
        return null
      }
      const revision = data.revisions[0]
      return {
        // 'Anonymous' fallback is defensive — backend should always populate.
        userName: revision.user_name || 'Anonymous',
        userUsername: revision.user_username ?? null,
        createdAt: revision.created_at,
      }
    },
    enabled: options?.enabled !== false,
  })
}
