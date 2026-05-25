import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, act, waitFor } from '@testing-library/react'
import { QueryClient } from '@tanstack/react-query'
import { createWrapperWithClient } from '@/test/utils'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    ADMIN: {
      FEATURED_SLOTS: {
        LIST: '/admin/featured-slots',
        SET: '/admin/featured-slots',
        RETIRE: (slotType: string) => `/admin/featured-slots/${slotType}`,
      },
    },
    EXPLORE: {
      FEATURED: '/explore/featured',
    },
  },
  API_BASE_URL: 'http://localhost:8080',
}))

vi.mock('@/lib/queryClient', () => ({
  queryKeys: {
    admin: {
      featuredSlots: () => ['admin', 'featuredSlots'],
    },
    explore: {
      featured: ['explore', 'featured'],
    },
  },
}))

import {
  useExploreFeatured,
  useFeaturedSlots,
  useRetireFeaturedSlot,
  useSetFeaturedSlot,
} from './useFeaturedSlots'

function setupClient() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0 },
      mutations: { retry: false },
    },
  })
  const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')
  return { queryClient, invalidateSpy }
}

describe('useFeaturedSlots — list query', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('GETs /admin/featured-slots and surfaces the response', async () => {
    mockApiRequest.mockResolvedValueOnce({ slots: [] })
    const { queryClient } = setupClient()

    const { result } = renderHook(() => useFeaturedSlots(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith('/admin/featured-slots')
    expect(result.current.data).toEqual({ slots: [] })
  })
})

describe('useExploreFeatured', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('GETs the public /explore/featured endpoint for the active card source', async () => {
    mockApiRequest.mockResolvedValueOnce({ bill: null, collection: null })
    const { queryClient } = setupClient()

    const { result } = renderHook(() => useExploreFeatured(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith('/explore/featured')
    expect(result.current.data).toEqual({ bill: null, collection: null })
  })
})

describe('useSetFeaturedSlot', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('POSTs the slot payload and invalidates BOTH list + explore caches', async () => {
    mockApiRequest.mockResolvedValueOnce({
      body: {
        id: 5,
        slot_type: 'bill',
        entity_id: 101,
        active_from: '2026-05-24T00:00:00Z',
        created_by: 1,
        created_at: '2026-05-24T00:00:00Z',
        updated_at: '2026-05-24T00:00:00Z',
      },
    })
    const { queryClient, invalidateSpy } = setupClient()

    const { result } = renderHook(() => useSetFeaturedSlot(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await act(async () => {
      await result.current.mutateAsync({
        slot_type: 'bill',
        entity_id: 101,
        curator_note: 'Sharp bill.',
      })
    })

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/admin/featured-slots',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({
          slot_type: 'bill',
          entity_id: 101,
          curator_note: 'Sharp bill.',
        }),
      })
    )
    // Both caches must invalidate so the active card + the admin list
    // refresh; failing to invalidate either leaves the UI desynced.
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ['admin', 'featuredSlots'],
    })
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ['explore', 'featured'],
    })
  })
})

describe('useRetireFeaturedSlot', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('DELETEs by slot_type path and invalidates both caches', async () => {
    mockApiRequest.mockResolvedValueOnce({
      slot_type: 'collection',
      message: 'Featured slot retired',
    })
    const { queryClient, invalidateSpy } = setupClient()

    const { result } = renderHook(() => useRetireFeaturedSlot(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await act(async () => {
      await result.current.mutateAsync('collection')
    })

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/admin/featured-slots/collection',
      { method: 'DELETE' }
    )
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ['admin', 'featuredSlots'],
    })
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ['explore', 'featured'],
    })
  })
})
