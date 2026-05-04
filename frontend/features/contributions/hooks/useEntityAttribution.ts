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
  /**
   * Resolved display name — never empty. Backend uses the resolveUserName
   * chain (username → first/last → email-prefix → "Anonymous"). PSY-560.
   */
  userName: string
  /**
   * Linkable username slug. Null when the user has no username set; the
   * AttributionLine renders plain text in that case rather than a broken
   * /users/:username link. Mirrors PSY-552 / PSY-353. PSY-560.
   */
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
        // Backend already resolves through the full chain; "Anonymous" is
        // the final fallback. The `|| 'Anonymous'` here is belt-and-braces
        // for old payloads or a hypothetical empty string from the wire.
        userName: revision.user_name || 'Anonymous',
        userUsername: revision.user_username ?? null,
        createdAt: revision.created_at,
      }
    },
    enabled: options?.enabled !== false,
  })
}
