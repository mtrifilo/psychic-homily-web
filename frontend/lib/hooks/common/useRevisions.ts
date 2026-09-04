import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import { queryKeys } from '@/lib/queryClient'
import type { components } from '@/types/api'

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
  /**
   * The author's id, ABSENT whenever `user_name` is (PSY-1940). The two are
   * withheld together on purpose: several public payloads carry an id and a
   * display name in the same object, so a published id is a lookup key that
   * recovers the byline the backend just declined to give.
   */
  user_id?: number
  /**
   * Resolved display name, ABSENT when the backend will not name the author:
   * they hid their contributions, or their only resolvable name would come
   * from their email address (PSY-1940). Render no byline in that case rather
   * than a placeholder — the absence means "we may not say".
   */
  user_name?: string
  /** URL-safe username slug; null when there is no profile to link to. */
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

/**
 * What a rollback actually did, field by field.
 *
 * Aliased from the generated OpenAPI types rather than hand-written, so the
 * skipped list cannot drift from what the endpoint sends (PSY-1550/1600).
 *
 * A rollback restores the fields the server's apply-side gates accept and
 * refuses the rest, so `skipped_fields` is a normal outcome and not an error
 * branch: a caller that renders only `success` tells an admin an edit was
 * undone when part of it was not.
 */
export type RollbackSkippedField = components['schemas']['RollbackSkippedField']
export type RollbackResponse = components['schemas']['RollbackRevisionResponseBody']

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
