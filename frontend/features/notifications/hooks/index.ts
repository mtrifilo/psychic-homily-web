'use client'

/**
 * Notification Filter Hooks
 *
 * TanStack Query hooks for notification filter CRUD, quick-create, and filter checking.
 */

import { useQuery, useMutation, useQueryClient, keepPreviousData } from '@tanstack/react-query'
import { apiRequest, API_BASE_URL } from '@/lib/api'
import { useAuthContext } from '@/lib/context/AuthContext'
import { queryKeys } from '@/lib/queryClient'
import type {
  NotificationFilter,
  CreateFilterInput,
  UpdateFilterInput,
  NotifyEntityType,
} from '../types'

// ──────────────────────────────────────────────
// API endpoints (not in central API_ENDPOINTS yet)
// ──────────────────────────────────────────────

const FILTER_ENDPOINTS = {
  LIST: `${API_BASE_URL}/me/notification-filters`,
  CREATE: `${API_BASE_URL}/me/notification-filters`,
  UPDATE: (id: number) => `${API_BASE_URL}/me/notification-filters/${id}`,
  DELETE: (id: number) => `${API_BASE_URL}/me/notification-filters/${id}`,
  QUICK: `${API_BASE_URL}/me/notification-filters/quick`,
}

// ──────────────────────────────────────────────
// Queries
// ──────────────────────────────────────────────

/** Fetch all notification filters for the current user */
export function useNotificationFilters() {
  // PSY-477: gate on auth so anonymous visitors of public entity pages
  // (artist / venue / label / festival, via NotifyMeButton →
  // useNotificationFilterCheck → this hook) don't fire a 401'd request.
  // FilterList on the notification settings page is behind auth anyway,
  // so this is strictly a no-op there.
  const { isAuthenticated } = useAuthContext()
  return useQuery({
    queryKey: queryKeys.notificationFilters.all,
    queryFn: () =>
      apiRequest<{ filters: NotificationFilter[] }>(FILTER_ENDPOINTS.LIST),
    staleTime: 5 * 60 * 1000,
    placeholderData: keepPreviousData,
    enabled: isAuthenticated,
  })
}

/**
 * Check if the current user has an active notification filter
 * matching a specific entity (artist/venue/label/tag).
 * Returns the matching filter, or undefined if none.
 */
export function useNotificationFilterCheck(
  entityType: NotifyEntityType,
  entityId: number
) {
  const { data, ...rest } = useNotificationFilters()

  // Find a filter that includes this entity in its criteria
  const matchingFilter = data?.filters?.find(filter => {
    if (!filter.is_active) return false

    switch (entityType) {
      case 'artist':
        return filter.artist_ids?.includes(entityId) ?? false
      case 'venue':
        return filter.venue_ids?.includes(entityId) ?? false
      case 'label':
        return filter.label_ids?.includes(entityId) ?? false
      case 'tag':
        return filter.tag_ids?.includes(entityId) ?? false
      default:
        return false
    }
  })

  return {
    ...rest,
    data: matchingFilter,
    hasFilter: !!matchingFilter,
  }
}

// ──────────────────────────────────────────────
// Mutations
// ──────────────────────────────────────────────

/** Create a new notification filter */
export function useCreateFilter() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: CreateFilterInput) =>
      apiRequest<NotificationFilter>(FILTER_ENDPOINTS.CREATE, {
        method: 'POST',
        body: JSON.stringify(input),
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.notificationFilters.all,
      })
    },
  })
}

/** Update an existing notification filter */
export function useUpdateFilter() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, ...input }: UpdateFilterInput & { id: number }) =>
      apiRequest<NotificationFilter>(FILTER_ENDPOINTS.UPDATE(id), {
        method: 'PATCH',
        body: JSON.stringify(input),
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.notificationFilters.all,
      })
    },
  })
}

/** Delete a notification filter */
export function useDeleteFilter() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: number) =>
      apiRequest<void>(FILTER_ENDPOINTS.DELETE(id), {
        method: 'DELETE',
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.notificationFilters.all,
      })
    },
  })
}

/** Quick-create a notification filter from an entity shortcut */
export function useQuickCreateFilter() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({
      entityType,
      entityId,
    }: {
      entityType: NotifyEntityType
      entityId: number
    }) =>
      apiRequest<NotificationFilter>(FILTER_ENDPOINTS.QUICK, {
        method: 'POST',
        body: JSON.stringify({
          entity_type: entityType,
          entity_id: entityId,
        }),
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.notificationFilters.all,
      })
    },
  })
}
