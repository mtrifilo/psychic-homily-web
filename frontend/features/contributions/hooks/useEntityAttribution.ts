import { useQuery } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import { queryKeys } from '@/lib/queryClient'

interface RevisionItem {
  id: number
  /** Withheld together with `user_name` when the author may not be named. */
  user_id?: number
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
   * Resolved display name, or null when the backend will not name the author:
   * they hid their contributions, or their only resolvable name would come
   * from their email address (PSY-1940). Null means RENDER NO BYLINE — the
   * absence says "we may not say", and an "Anonymous" placeholder would assert
   * a person the backend deliberately declined to name.
   */
  user_name: string | null
  /** URL-safe username slug; null when there is no profile to link to. */
  user_username: string | null
  created_at: string
  /**
   * Total revision count for the entity, from the same `?limit=1` read (the
   * endpoint reports the full count regardless of page size), passed through
   * untouched.
   */
  total: number
}

/**
 * Fetches the most recent revision for an entity to show "Last edited"
 * attribution. Returns the most recent editor's display name (when the backend
 * will publish one) and, when set, a linkable username.
 *
 * Returns null data if no revisions exist. A revision WITH no publishable
 * author is not the same thing: the edit and its date are still returned, with
 * `user_name: null`.
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
        // Normalised to null, NOT to 'Anonymous'. An absent name is a decision
        // the backend made about what it may publish (PSY-1940), and the old
        // placeholder converted "we may not say" into a claim about a person.
        // An empty string collapses to null for the same reason.
        user_name: revision.user_name || null,
        user_username: revision.user_username ?? null,
        created_at: revision.created_at,
        // Passed through untouched. The backend handler always sets it, but
        // note the honest limit: `EntityHistoryResponse` above is a
        // hand-written mirror and `apiRequest<T>` is an unchecked cast, so
        // nothing HERE proves the field exists — value-level consumers guard
        // before rendering. A defensive floor was rejected because it would
        // convert a loud backend regression ("0 edits" beside a rendered
        // revision) into a quietly wrong small number nobody reports.
        total: data.total,
      }
    },
    enabled: options?.enabled !== false,
  })
}
