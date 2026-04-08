import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { createWrapper } from '@/test/utils'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    COLLECTIONS: {
      LIST: '/collections',
      DETAIL: (slug: string) => `/collections/${slug}`,
      STATS: (slug: string) => `/collections/${slug}/stats`,
      ITEMS: (slug: string) => `/collections/${slug}/items`,
      ITEM: (slug: string, itemId: number) => `/collections/${slug}/items/${itemId}`,
      SUBSCRIBE: (slug: string) => `/collections/${slug}/subscribe`,
      FEATURE: (slug: string) => `/collections/${slug}/feature`,
      MY: '/auth/collections',
    },
  },
  API_BASE_URL: 'http://localhost:8080',
}))

vi.mock('@/lib/queryClient', () => ({
  queryKeys: {
    collections: {
      all: ['collections'],
      detail: (slug: string) => ['collections', 'detail', slug],
      stats: (slug: string) => ['collections', 'stats', slug],
      my: ['collections', 'my'],
    },
  },
  createInvalidateQueries: () => ({
    collections: vi.fn(),
  }),
}))

import {
  useCollections,
  useCollection,
  useCollectionStats,
  useMyCollections,
  useSetFeatured,
  useCreateCollection,
  useUpdateCollection,
  useDeleteCollection,
  useAddCollectionItem,
  useRemoveCollectionItem,
  useSubscribeCollection,
  useUnsubscribeCollection,
} from './index'


describe('Collection query hooks', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  describe('useCollections', () => {
    it('fetches collections list', async () => {
      // Backend returns { crates, total }; hook normalizes to { collections, total }
      const mockResponse = {
        crates: [{ id: 1, title: 'Test Collection', slug: 'test' }],
        total: 1,
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(() => useCollections(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith('/collections')
      expect(result.current.data?.collections).toHaveLength(1)
      expect(result.current.data?.total).toBe(1)
    })

    it('handles empty collections list', async () => {
      mockApiRequest.mockResolvedValueOnce({ crates: [], total: 0 })

      const { result } = renderHook(() => useCollections(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))
      expect(result.current.data?.collections).toEqual([])
    })
  })

  describe('useCollection', () => {
    it('fetches a single collection by slug', async () => {
      const mockDetail = { id: 1, title: 'My Collection', slug: 'my-collection', items: [] }
      mockApiRequest.mockResolvedValueOnce(mockDetail)

      const { result } = renderHook(() => useCollection('my-collection'), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))
      expect(mockApiRequest).toHaveBeenCalledWith('/collections/my-collection')
    })

    it('does not fetch when slug is empty', () => {
      const { result } = renderHook(() => useCollection(''), {
        wrapper: createWrapper(),
      })

      expect(result.current.fetchStatus).toBe('idle')
      expect(mockApiRequest).not.toHaveBeenCalled()
    })

    it('does not fetch when enabled is false', () => {
      const { result } = renderHook(
        () => useCollection('my-slug', { enabled: false }),
        { wrapper: createWrapper() }
      )

      expect(result.current.fetchStatus).toBe('idle')
      expect(mockApiRequest).not.toHaveBeenCalled()
    })
  })

  describe('useCollectionStats', () => {
    it('fetches stats for a collection', async () => {
      const mockStats = { item_count: 5, subscriber_count: 10 }
      mockApiRequest.mockResolvedValueOnce(mockStats)

      const { result } = renderHook(() => useCollectionStats('my-collection'), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))
      expect(mockApiRequest).toHaveBeenCalledWith('/collections/my-collection/stats')
    })

    it('does not fetch when slug is empty', () => {
      const { result } = renderHook(() => useCollectionStats(''), {
        wrapper: createWrapper(),
      })

      expect(result.current.fetchStatus).toBe('idle')
    })
  })

  describe('useMyCollections', () => {
    it('fetches user collections', async () => {
      mockApiRequest.mockResolvedValueOnce({ crates: [], total: 0 })

      const { result } = renderHook(() => useMyCollections(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))
      expect(mockApiRequest).toHaveBeenCalledWith('/auth/collections')
    })
  })
})

describe('Collection mutation hooks', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  describe('useCreateCollection', () => {
    it('creates a collection with POST', async () => {
      mockApiRequest.mockResolvedValueOnce({ id: 1, title: 'New', slug: 'new' })

      const { result } = renderHook(() => useCreateCollection(), {
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
        '/collections',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({ title: 'New', is_public: true, collaborative: false }),
        })
      )
    })
  })

  describe('useUpdateCollection', () => {
    it('updates a collection with PUT', async () => {
      mockApiRequest.mockResolvedValueOnce({ id: 1, title: 'Updated', slug: 'test' })

      const { result } = renderHook(() => useUpdateCollection(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({ slug: 'test', title: 'Updated' })
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/collections/test',
        expect.objectContaining({
          method: 'PUT',
          body: JSON.stringify({ title: 'Updated' }),
        })
      )
    })
  })

  describe('useDeleteCollection', () => {
    it('deletes a collection with DELETE', async () => {
      mockApiRequest.mockResolvedValueOnce(undefined)

      const { result } = renderHook(() => useDeleteCollection(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({ slug: 'to-delete' })
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/collections/to-delete',
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
        '/collections/test/feature',
        expect.objectContaining({
          method: 'PUT',
          body: JSON.stringify({ featured: true }),
        })
      )
    })
  })

  describe('useAddCollectionItem', () => {
    it('adds an item to a collection', async () => {
      mockApiRequest.mockResolvedValueOnce(undefined)

      const { result } = renderHook(() => useAddCollectionItem(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({
          slug: 'my-collection',
          entityType: 'artist',
          entityId: 42,
          notes: 'Great artist',
        })
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/collections/my-collection/items',
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

  describe('useRemoveCollectionItem', () => {
    it('removes an item from a collection', async () => {
      mockApiRequest.mockResolvedValueOnce(undefined)

      const { result } = renderHook(() => useRemoveCollectionItem(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({ slug: 'my-collection', itemId: 5 })
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/collections/my-collection/items/5',
        expect.objectContaining({ method: 'DELETE' })
      )
    })
  })

  describe('useSubscribeCollection', () => {
    it('subscribes with POST', async () => {
      mockApiRequest.mockResolvedValueOnce(undefined)

      const { result } = renderHook(() => useSubscribeCollection(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({ slug: 'my-collection' })
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/collections/my-collection/subscribe',
        expect.objectContaining({ method: 'POST' })
      )
    })
  })

  describe('useUnsubscribeCollection', () => {
    it('unsubscribes with DELETE', async () => {
      mockApiRequest.mockResolvedValueOnce(undefined)

      const { result } = renderHook(() => useUnsubscribeCollection(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({ slug: 'my-collection' })
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/collections/my-collection/subscribe',
        expect.objectContaining({ method: 'DELETE' })
      )
    })
  })

  describe('mutation error handling', () => {
    it('handles create errors', async () => {
      const error = new Error('Server error')
      Object.assign(error, { status: 500 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => useCreateCollection(), {
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
