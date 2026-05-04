import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import { queryKeys } from '@/lib/queryClient'

// --- Types ---

export interface FieldChange {
  field: string
  old_value: unknown
  new_value: unknown
}

export interface RevisionItem {
  id: number
  entity_type: string
  entity_id: number
  user_id: number
  /**
   * Resolved display name — never empty when the User row exists. Backend
   * uses the resolveUserName chain (username → first/last → email-prefix →
   * "Anonymous"). PSY-560.
   */
  user_name?: string
  /**
   * Linkable username slug. Null when the user has no username set; in that
   * case the frontend should render `user_name` as plain text rather than a
   * /users/:username link. Mirrors comment author_username (PSY-552). PSY-560.
   */
  user_username?: string | null
  changes: FieldChange[]
  summary?: string
  created_at: string
}

interface EntityHistoryResponse {
  revisions: RevisionItem[]
  total: number
}

interface UserRevisionsResponse {
  revisions: RevisionItem[]
  total: number
}

interface RollbackResponse {
  success: boolean
}

// --- Hooks ---

/**
 * Fetch revision history for a specific entity.
 */
export function useEntityRevisions(
  entityType: string,
  entityId: string | number,
  options?: { enabled?: boolean; limit?: number; offset?: number }
) {
  const limit = options?.limit ?? 20
  const offset = options?.offset ?? 0

  return useQuery({
    queryKey: [...queryKeys.revisions.entity(entityType, entityId), { limit, offset }],
    queryFn: () => {
      const params = new URLSearchParams()
      if (limit != null) params.set('limit', String(limit))
      if (offset != null) params.set('offset', String(offset))
      const qs = params.toString()
      const url = `${API_ENDPOINTS.REVISIONS.ENTITY_HISTORY(entityType, entityId)}${qs ? `?${qs}` : ''}`
      return apiRequest<EntityHistoryResponse>(url)
    },
    enabled: options?.enabled !== false,
  })
}

/**
 * Fetch a single revision by ID.
 */
export function useRevision(
  revisionId: number,
  options?: { enabled?: boolean }
) {
  return useQuery({
    queryKey: queryKeys.revisions.detail(revisionId),
    queryFn: () =>
      apiRequest<RevisionItem>(API_ENDPOINTS.REVISIONS.DETAIL(revisionId)),
    enabled: options?.enabled !== false && revisionId > 0,
  })
}

/**
 * Fetch revision history for a specific user.
 */
export function useUserRevisions(
  userId: string | number,
  options?: { enabled?: boolean; limit?: number; offset?: number }
) {
  const limit = options?.limit ?? 20
  const offset = options?.offset ?? 0

  return useQuery({
    queryKey: [...queryKeys.revisions.user(userId), { limit, offset }],
    queryFn: () => {
      const params = new URLSearchParams()
      if (limit != null) params.set('limit', String(limit))
      if (offset != null) params.set('offset', String(offset))
      const qs = params.toString()
      const url = `${API_ENDPOINTS.REVISIONS.USER_REVISIONS(userId)}${qs ? `?${qs}` : ''}`
      return apiRequest<UserRevisionsResponse>(url)
    },
    enabled: options?.enabled !== false,
  })
}

/**
 * Rollback a revision (admin only).
 */
export function useRollbackRevision() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (revisionId: number) =>
      apiRequest<RollbackResponse>(API_ENDPOINTS.REVISIONS.ROLLBACK(revisionId), {
        method: 'POST',
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.revisions.all })
    },
  })
}
