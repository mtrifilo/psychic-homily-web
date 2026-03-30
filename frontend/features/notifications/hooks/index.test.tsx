import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { createWrapper } from '@/test/utils'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_BASE_URL: 'http://localhost:8080',
}))

vi.mock('@/lib/queryClient', () => ({
  queryKeys: {
    notificationFilters: {
      all: ['notificationFilters'],
    },
  },
}))

import {
  useNotificationFilters,
  useNotificationFilterCheck,
  useCreateFilter,
  useUpdateFilter,
  useDeleteFilter,
  useQuickCreateFilter,
} from './index'


describe('useNotificationFilters', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches notification filters', async () => {
    const mockFilters = {
      filters: [
        { id: 1, name: 'My Filter', is_active: true, artist_ids: [1, 2] },
      ],
    }
    mockApiRequest.mockResolvedValueOnce(mockFilters)

    const { result } = renderHook(() => useNotificationFilters(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      'http://localhost:8080/me/notification-filters'
    )
  })

  it('handles empty filters', async () => {
    mockApiRequest.mockResolvedValueOnce({ filters: [] })

    const { result } = renderHook(() => useNotificationFilters(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
  })
})

describe('useNotificationFilterCheck', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('finds matching filter for artist entity', async () => {
    mockApiRequest.mockResolvedValueOnce({
      filters: [
        { id: 1, name: 'Artists', is_active: true, artist_ids: [10, 20], venue_ids: null, label_ids: null, tag_ids: null },
        { id: 2, name: 'Venues', is_active: true, artist_ids: null, venue_ids: [30], label_ids: null, tag_ids: null },
      ],
    })

    const { result } = renderHook(
      () => useNotificationFilterCheck('artist', 10),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.hasFilter).toBe(true))
    expect(result.current.data?.id).toBe(1)
  })

  it('finds matching filter for venue entity', async () => {
    mockApiRequest.mockResolvedValueOnce({
      filters: [
        { id: 1, name: 'Venues', is_active: true, artist_ids: null, venue_ids: [30, 40], label_ids: null, tag_ids: null },
      ],
    })

    const { result } = renderHook(
      () => useNotificationFilterCheck('venue', 30),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.hasFilter).toBe(true))
  })

  it('finds matching filter for label entity', async () => {
    mockApiRequest.mockResolvedValueOnce({
      filters: [
        { id: 1, name: 'Labels', is_active: true, artist_ids: null, venue_ids: null, label_ids: [50], tag_ids: null },
      ],
    })

    const { result } = renderHook(
      () => useNotificationFilterCheck('label', 50),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.hasFilter).toBe(true))
  })

  it('finds matching filter for tag entity', async () => {
    mockApiRequest.mockResolvedValueOnce({
      filters: [
        { id: 1, name: 'Tags', is_active: true, artist_ids: null, venue_ids: null, label_ids: null, tag_ids: [60] },
      ],
    })

    const { result } = renderHook(
      () => useNotificationFilterCheck('tag', 60),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.hasFilter).toBe(true))
  })

  it('returns hasFilter false when no matching filter exists', async () => {
    mockApiRequest.mockResolvedValueOnce({
      filters: [
        { id: 1, name: 'Filter', is_active: true, artist_ids: [1, 2], venue_ids: null, label_ids: null, tag_ids: null },
      ],
    })

    const { result } = renderHook(
      () => useNotificationFilterCheck('artist', 999),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.hasFilter).toBe(false)
    expect(result.current.data).toBeUndefined()
  })

  it('ignores inactive filters', async () => {
    mockApiRequest.mockResolvedValueOnce({
      filters: [
        { id: 1, name: 'Inactive', is_active: false, artist_ids: [10], venue_ids: null, label_ids: null, tag_ids: null },
      ],
    })

    const { result } = renderHook(
      () => useNotificationFilterCheck('artist', 10),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.hasFilter).toBe(false)
  })
})

describe('useCreateFilter', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('creates a filter with POST', async () => {
    mockApiRequest.mockResolvedValueOnce({ id: 1, name: 'New Filter' })

    const { result } = renderHook(() => useCreateFilter(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ name: 'New Filter', artist_ids: [1] } as any)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      'http://localhost:8080/me/notification-filters',
      expect.objectContaining({ method: 'POST' })
    )
  })
})

describe('useUpdateFilter', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('updates a filter with PATCH', async () => {
    mockApiRequest.mockResolvedValueOnce({ id: 1, name: 'Updated' })

    const { result } = renderHook(() => useUpdateFilter(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ id: 1, name: 'Updated' } as any)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      'http://localhost:8080/me/notification-filters/1',
      expect.objectContaining({ method: 'PATCH' })
    )
  })
})

describe('useDeleteFilter', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('deletes a filter with DELETE', async () => {
    mockApiRequest.mockResolvedValueOnce(undefined)

    const { result } = renderHook(() => useDeleteFilter(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(1)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      'http://localhost:8080/me/notification-filters/1',
      expect.objectContaining({ method: 'DELETE' })
    )
  })
})

describe('useQuickCreateFilter', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('quick-creates a filter for an entity', async () => {
    mockApiRequest.mockResolvedValueOnce({ id: 1, name: 'Quick Filter' })

    const { result } = renderHook(() => useQuickCreateFilter(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ entityType: 'artist', entityId: 42 })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      'http://localhost:8080/me/notification-filters/quick',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ entity_type: 'artist', entity_id: 42 }),
      })
    )
  })
})
