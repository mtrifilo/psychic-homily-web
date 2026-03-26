import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { createWrapper } from '@/test/utils'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    CRATES: {
      LIST: '/crates',
      DETAIL: (slug: string) => `/crates/${slug}`,
      STATS: (slug: string) => `/crates/${slug}/stats`,
      ITEMS: (slug: string) => `/crates/${slug}/items`,
      ITEM: (slug: string, itemId: number) => `/crates/${slug}/items/${itemId}`,
      SUBSCRIBE: (slug: string) => `/crates/${slug}/subscribe`,
      FEATURE: (slug: string) => `/crates/${slug}/feature`,
      MY: '/auth/crates',
    },
  },
  API_BASE_URL: 'http://localhost:8080',
}))

vi.mock('@/lib/queryClient', () => ({
  queryKeys: {
    crates: {
      all: ['crates'],
      detail: (slug: string) => ['crates', 'detail', slug],
      stats: (slug: string) => ['crates', 'stats', slug],
      my: ['crates', 'my'],
    },
  },
  createInvalidateQueries: () => ({
    crates: vi.fn(),
  }),
}))

import {
  useCrates,
  useCrate,
  useCrateStats,
  useMyCrates,
  useSetFeatured,
  useCreateCrate,
  useUpdateCrate,
  useDeleteCrate,
  useAddCrateItem,
  useRemoveCrateItem,
  useSubscribeCrate,
  useUnsubscribeCrate,
} from './index'


describe('Crate query hooks', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  describe('useCrates', () => {
    it('fetches crates list', async () => {
      const mockResponse = {
        crates: [{ id: 1, title: 'Test Crate', slug: 'test' }],
        total: 1,
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(() => useCrates(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith('/crates')
      expect(result.current.data?.crates).toHaveLength(1)
    })

    it('handles empty crates list', async () => {
      mockApiRequest.mockResolvedValueOnce({ crates: [], total: 0 })

      const { result } = renderHook(() => useCrates(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))
      expect(result.current.data?.total).toBe(0)
    })
  })

  describe('useCrate', () => {
    it('fetches a single crate by slug', async () => {
      const mockDetail = { id: 1, title: 'My Crate', slug: 'my-crate', items: [] }
      mockApiRequest.mockResolvedValueOnce(mockDetail)

      const { result } = renderHook(() => useCrate('my-crate'), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))
      expect(mockApiRequest).toHaveBeenCalledWith('/crates/my-crate')
    })

    it('does not fetch when slug is empty', () => {
      const { result } = renderHook(() => useCrate(''), {
        wrapper: createWrapper(),
      })

      expect(result.current.fetchStatus).toBe('idle')
      expect(mockApiRequest).not.toHaveBeenCalled()
    })

    it('does not fetch when enabled is false', () => {
      const { result } = renderHook(
        () => useCrate('my-slug', { enabled: false }),
        { wrapper: createWrapper() }
      )

      expect(result.current.fetchStatus).toBe('idle')
      expect(mockApiRequest).not.toHaveBeenCalled()
    })
  })

  describe('useCrateStats', () => {
    it('fetches stats for a crate', async () => {
      const mockStats = { item_count: 5, subscriber_count: 10 }
      mockApiRequest.mockResolvedValueOnce(mockStats)

      const { result } = renderHook(() => useCrateStats('my-crate'), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))
      expect(mockApiRequest).toHaveBeenCalledWith('/crates/my-crate/stats')
    })

    it('does not fetch when slug is empty', () => {
      const { result } = renderHook(() => useCrateStats(''), {
        wrapper: createWrapper(),
      })

      expect(result.current.fetchStatus).toBe('idle')
    })
  })

  describe('useMyCrates', () => {
    it('fetches user crates', async () => {
      mockApiRequest.mockResolvedValueOnce({ crates: [], total: 0 })

      const { result } = renderHook(() => useMyCrates(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))
      expect(mockApiRequest).toHaveBeenCalledWith('/auth/crates')
    })
  })
})

describe('Crate mutation hooks', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  describe('useCreateCrate', () => {
    it('creates a crate with POST', async () => {
      mockApiRequest.mockResolvedValueOnce({ id: 1, title: 'New', slug: 'new' })

      const { result } = renderHook(() => useCreateCrate(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({
          title: 'New',
          is_public: true,
          collaborative: false,
        })
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/crates',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({ title: 'New', is_public: true, collaborative: false }),
        })
      )
    })
  })

  describe('useUpdateCrate', () => {
    it('updates a crate with PUT', async () => {
      mockApiRequest.mockResolvedValueOnce({ id: 1, title: 'Updated', slug: 'test' })

      const { result } = renderHook(() => useUpdateCrate(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({ slug: 'test', title: 'Updated' })
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/crates/test',
        expect.objectContaining({
          method: 'PUT',
          body: JSON.stringify({ title: 'Updated' }),
        })
      )
    })
  })

  describe('useDeleteCrate', () => {
    it('deletes a crate with DELETE', async () => {
      mockApiRequest.mockResolvedValueOnce(undefined)

      const { result } = renderHook(() => useDeleteCrate(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({ slug: 'to-delete' })
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/crates/to-delete',
        expect.objectContaining({ method: 'DELETE' })
      )
    })
  })

  describe('useSetFeatured', () => {
    it('sets featured status with PUT', async () => {
      mockApiRequest.mockResolvedValueOnce(undefined)

      const { result } = renderHook(() => useSetFeatured(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({ slug: 'test', featured: true })
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/crates/test/feature',
        expect.objectContaining({
          method: 'PUT',
          body: JSON.stringify({ featured: true }),
        })
      )
    })
  })

  describe('useAddCrateItem', () => {
    it('adds an item to a crate', async () => {
      mockApiRequest.mockResolvedValueOnce(undefined)

      const { result } = renderHook(() => useAddCrateItem(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({
          slug: 'my-crate',
          entityType: 'artist',
          entityId: 42,
          notes: 'Great artist',
        })
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/crates/my-crate/items',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({
            entity_type: 'artist',
            entity_id: 42,
            notes: 'Great artist',
          }),
        })
      )
    })
  })

  describe('useRemoveCrateItem', () => {
    it('removes an item from a crate', async () => {
      mockApiRequest.mockResolvedValueOnce(undefined)

      const { result } = renderHook(() => useRemoveCrateItem(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({ slug: 'my-crate', itemId: 5 })
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/crates/my-crate/items/5',
        expect.objectContaining({ method: 'DELETE' })
      )
    })
  })

  describe('useSubscribeCrate', () => {
    it('subscribes with POST', async () => {
      mockApiRequest.mockResolvedValueOnce(undefined)

      const { result } = renderHook(() => useSubscribeCrate(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({ slug: 'my-crate' })
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/crates/my-crate/subscribe',
        expect.objectContaining({ method: 'POST' })
      )
    })
  })

  describe('useUnsubscribeCrate', () => {
    it('unsubscribes with DELETE', async () => {
      mockApiRequest.mockResolvedValueOnce(undefined)

      const { result } = renderHook(() => useUnsubscribeCrate(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({ slug: 'my-crate' })
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/crates/my-crate/subscribe',
        expect.objectContaining({ method: 'DELETE' })
      )
    })
  })

  describe('mutation error handling', () => {
    it('handles create errors', async () => {
      const error = new Error('Server error')
      Object.assign(error, { status: 500 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => useCreateCrate(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({
          title: 'Fail',
          is_public: true,
          collaborative: false,
        })
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
      expect(result.current.error).toBeDefined()
    })
  })
})
