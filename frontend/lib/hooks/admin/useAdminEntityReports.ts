'use client'

/**
 * Admin Entity Report Hooks
 *
 * TanStack Query hooks for the unified moderation queue — entity reports.
 */

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '../../api'
import { queryKeys, createInvalidateQueries } from '../../queryClient'

// ─── Types ───────────────────────────────────────────────────────────────────
//
// Aliased from the generated OpenAPI types, not hand-written (PSY-1550/1600).
// Regenerate with `bun run api:types`; the "API Types Drift" CI gate fails if
// the committed types drift from the backend. Exported names are kept stable
// for callers.
//
// `entity_slug` is populated only for entity types addressed by slug in the
// public app (currently `collection`); ID-addressed types omit it.
// `reports` is nullable on the wire: guard before iterating (PSY-1600).

import type { components } from '../../../types/api'

export type EntityReportResponse = components['schemas']['EntityReportResponse']
export type EntityReportsListResponse =
  components['schemas']['AdminListEntityReportsResponseBody']

// ─── Filters ─────────────────────────────────────────────────────────────────

export interface EntityReportFilters {
  status?: string
  entity_type?: string
  limit?: number
  offset?: number
  /** When false, the query does not fire (e.g. the admin nav badge off-route). Defaults to true. */
  enabled?: boolean
}

// ─── Hooks ───────────────────────────────────────────────────────────────────

/**
 * Hook to fetch entity reports for admin review.
 */
export function useAdminEntityReports(filters: EntityReportFilters = {}) {
  const { status = 'pending', entity_type, limit = 50, offset = 0, enabled = true } = filters

  const params = new URLSearchParams()
  if (status) params.set('status', status)
  if (entity_type) params.set('entity_type', entity_type)
  params.set('limit', limit.toString())
  params.set('offset', offset.toString())

  const endpoint = `${API_ENDPOINTS.ADMIN.ENTITY_REPORTS.LIST}?${params.toString()}`

  return useQuery({
    queryKey: queryKeys.admin.entityReports({ status, entity_type, limit, offset }),
    queryFn: async (): Promise<EntityReportsListResponse> => {
      return apiRequest<EntityReportsListResponse>(endpoint, {
        method: 'GET',
      })
    },
    staleTime: 30 * 1000, // 30 seconds
    enabled,
  })
}

/**
 * Hook to resolve an entity report.
 */
export function useResolveEntityReport() {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async ({
      reportId,
      notes,
    }: {
      reportId: number
      notes?: string
    }): Promise<EntityReportResponse> => {
      return apiRequest<EntityReportResponse>(
        API_ENDPOINTS.ADMIN.ENTITY_REPORTS.RESOLVE(reportId),
        {
          method: 'POST',
          body: JSON.stringify({ notes: notes || '' }),
        }
      )
    },
    onSuccess: () => {
      invalidateQueries.adminEntityReports()
    },
  })
}

/**
 * Hide a collection from public browse by flipping `is_public` to false via
 * the existing admin-permitted PUT /collections/{slug} endpoint. The
 * backend's UpdateCollection accepts an `is_admin` path so admins can edit
 * any collection. No new endpoint is required.
 *
 * Coupled with `useResolveEntityReport` at the call site so a single click
 * in the moderation queue both hides the collection AND clears the report
 * (the same shape `useAdminHideComment` provides for comment reports).
 *
 * SUCCEEDS WITH NO BODY. The flip is what makes the collection unreadable to
 * everyone but its creator, including the admin performing it, so the route
 * reads back nothing and answers 204. `apiRequest` resolves that to
 * `undefined`, which is why the return type is void and why nothing here reads
 * the response.
 */
export function useAdminHideCollection() {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async ({ slug }: { slug: string }): Promise<void> => {
      return apiRequest<void>(API_ENDPOINTS.COLLECTIONS.DETAIL(slug), {
        method: 'PUT',
        body: JSON.stringify({ is_public: false }),
      })
    },
    onSuccess: () => {
      invalidateQueries.adminEntityReports()
      // Detail + list pages may surface the now-private collection's flag.
      queryClient.invalidateQueries({ queryKey: ['collections'] })
    },
  })
}

/**
 * Hook to dismiss an entity report.
 */
export function useDismissEntityReport() {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async ({
      reportId,
      notes,
    }: {
      reportId: number
      notes?: string
    }): Promise<EntityReportResponse> => {
      return apiRequest<EntityReportResponse>(
        API_ENDPOINTS.ADMIN.ENTITY_REPORTS.DISMISS(reportId),
        {
          method: 'POST',
          body: JSON.stringify({ notes: notes || '' }),
        }
      )
    },
    onSuccess: () => {
      invalidateQueries.adminEntityReports()
    },
  })
}
