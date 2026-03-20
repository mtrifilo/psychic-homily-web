import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createWrapper, createTestQueryClient } from '@/test/utils'

// Create mocks
const mockApiRequest = vi.fn()

// Mock the api module
vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    COLLECTIONS: {
      LIST: '/collections',
      DETAIL: (slug: string) => `/collections/${slug}`,
      STATS: (slug: string) => `/collections/${slug}/stats`,
      ITEMS: (slug: string) => `/collections/${slug}/items`,
      ITEM: (slug: string, itemId: number) =>
        `/collections/${slug}/items/${itemId}`,
      SUBSCRIBE: (slug: string) => `/collections/${slug}/subscribe`,
      FEATURE: (slug: string) => `/collections/${slug}/feature`,
      MY: '/auth/collections',
    },
  },
  API_BASE_URL: 'http://localhost:8080',
}))

// Mock queryClient module
vi.mock('@/lib/queryClient', () => ({
  queryKeys: {
    collections: {
      all: ['collections'],
      detail: (slug: string) => ['collections', 'detail', slug],
      stats: (slug: string) => ['collections', 'stats', slug],
      my: ['collections', 'my'],
    },
  },
}))

// Import hooks after mocks are set up
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

describe('Collection Hooks', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  // ──────────────────────────────────────────────
  // Query hooks
  // ──────────────────────────────────────────────

  describe('useCollections', () => {
    it('fetches the public collections list', async () => {
      const mockResponse = {
        collections: [
          { id: 1, title: 'Best Of', slug: 'best-of' },
          { id: 2, title: 'New Finds', slug: 'new-finds' },
        ],
        total: 2,
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(() => useCollections(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith('/collections')
      expect(result.current.data?.collections).toHaveLength(2)
      expect(result.current.data?.total).toBe(2)
    })

    it('handles API errors', async () => {
      const error = new Error('Server error')
      Object.assign(error, { status: 500 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => useCollections(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
      expect(result.current.error).toBeDefined()
    })

    it('returns empty collections list', async () => {
      mockApiRequest.mockResolvedValueOnce({ collections: [], total: 0 })

      const { result } = renderHook(() => useCollections(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(result.current.data?.collections).toHaveLength(0)
      expect(result.current.data?.total).toBe(0)
    })
  })

  describe('useCollection', () => {
    it('fetches a single collection by slug', async () => {
      const mockDetail = {
        id: 1,
        title: 'Best Of',
        slug: 'best-of',
        items: [{ id: 1, entity_type: 'artist', entity_id: 10 }],
      }
      mockApiRequest.mockResolvedValueOnce(mockDetail)

      const { result } = renderHook(() => useCollection('best-of'), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith('/collections/best-of')
      expect(result.current.data?.title).toBe('Best Of')
    })

    it('does not fetch when slug is empty', async () => {
      const { result } = renderHook(() => useCollection(''), {
        wrapper: createWrapper(),
      })

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result.current.fetchStatus).toBe('idle')
    })

    it('does not fetch when enabled is false', async () => {
      const { result } = renderHook(
        () => useCollection('best-of', { enabled: false }),
        { wrapper: createWrapper() }
      )

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result.current.fetchStatus).toBe('idle')
    })

    it('handles collection not found error', async () => {
      const error = new Error('Collection not found')
      Object.assign(error, { status: 404 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => useCollection('nonexistent'), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
      expect((result.current.error as Error).message).toBe(
        'Collection not found'
      )
    })
  })

  describe('useCollectionStats', () => {
    it('fetches collection stats by slug', async () => {
      const mockStats = {
        item_count: 10,
        subscriber_count: 5,
        entity_type_counts: { artist: 6, release: 4 },
      }
      mockApiRequest.mockResolvedValueOnce(mockStats)

      const { result } = renderHook(() => useCollectionStats('best-of'), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith('/collections/best-of/stats')
      expect(result.current.data?.item_count).toBe(10)
    })

    it('does not fetch when slug is empty', async () => {
      const { result } = renderHook(() => useCollectionStats(''), {
        wrapper: createWrapper(),
      })

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result.current.fetchStatus).toBe('idle')
    })

    it('does not fetch when enabled is false', async () => {
      const { result } = renderHook(
        () => useCollectionStats('best-of', { enabled: false }),
        { wrapper: createWrapper() }
      )

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result.current.fetchStatus).toBe('idle')
    })

    it('handles API errors', async () => {
      const error = new Error('Server error')
      Object.assign(error, { status: 500 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => useCollectionStats('best-of'), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
    })
  })

  describe('useMyCollections', () => {
    it('fetches authenticated user collections', async () => {
      const mockResponse = {
        collections: [
          { id: 1, title: 'My Faves', slug: 'my-faves' },
        ],
        total: 1,
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(() => useMyCollections(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith('/auth/collections')
      expect(result.current.data?.collections).toHaveLength(1)
    })

    it('handles unauthorized error', async () => {
      const error = new Error('Unauthorized')
      Object.assign(error, { status: 401 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => useMyCollections(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
    })
  })

  // ──────────────────────────────────────────────
  // Mutation hooks
  // ──────────────────────────────────────────────

  describe('useSetFeatured', () => {
    it('sets featured status on a collection', async () => {
      mockApiRequest.mockResolvedValueOnce(undefined)

      const queryClient = createTestQueryClient()
      const wrapper = ({ children }: { children: React.ReactNode }) => (
        <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
      )

      const { result } = renderHook(() => useSetFeatured(), { wrapper })

      await act(async () => {
        await result.current.mutateAsync({ slug: 'best-of', featured: true })
      })

      expect(mockApiRequest).toHaveBeenCalledWith('/collections/best-of/feature', {
        method: 'PUT',
        body: JSON.stringify({ featured: true }),
      })
    })

    it('handles mutation errors', async () => {
      const error = new Error('Forbidden')
      Object.assign(error, { status: 403 })
      mockApiRequest.mockRejectedValueOnce(error)

      const queryClient = createTestQueryClient()
      const wrapper = ({ children }: { children: React.ReactNode }) => (
        <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
      )

      const { result } = renderHook(() => useSetFeatured(), { wrapper })

      await expect(
        act(async () => {
          await result.current.mutateAsync({ slug: 'best-of', featured: true })
        })
      ).rejects.toThrow('Forbidden')
    })
  })

  describe('useCreateCollection', () => {
    it('creates a new collection', async () => {
      const mockCreated = {
        id: 3,
        title: 'New Collection',
        slug: 'new-collection',
        is_public: true,
        collaborative: false,
      }
      mockApiRequest.mockResolvedValueOnce(mockCreated)

      const queryClient = createTestQueryClient()
      const wrapper = ({ children }: { children: React.ReactNode }) => (
        <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
      )

      const { result } = renderHook(() => useCreateCollection(), { wrapper })

      let data: unknown
      await act(async () => {
        data = await result.current.mutateAsync({
          title: 'New Collection',
          description: 'A great collection',
          is_public: true,
          collaborative: false,
        })
      })

      expect(mockApiRequest).toHaveBeenCalledWith('/collections', {
        method: 'POST',
        body: JSON.stringify({
          title: 'New Collection',
          description: 'A great collection',
          is_public: true,
          collaborative: false,
        }),
      })
      expect(data).toEqual(mockCreated)
    })
  })

  describe('useUpdateCollection', () => {
    it('updates an existing collection', async () => {
      const mockUpdated = {
        id: 1,
        title: 'Updated Title',
        slug: 'best-of',
      }
      mockApiRequest.mockResolvedValueOnce(mockUpdated)

      const queryClient = createTestQueryClient()
      const wrapper = ({ children }: { children: React.ReactNode }) => (
        <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
      )

      const { result } = renderHook(() => useUpdateCollection(), { wrapper })

      await act(async () => {
        await result.current.mutateAsync({
          slug: 'best-of',
          title: 'Updated Title',
        })
      })

      expect(mockApiRequest).toHaveBeenCalledWith('/collections/best-of', {
        method: 'PUT',
        body: JSON.stringify({ title: 'Updated Title' }),
      })
    })
  })

  describe('useDeleteCollection', () => {
    it('deletes a collection', async () => {
      mockApiRequest.mockResolvedValueOnce(undefined)

      const queryClient = createTestQueryClient()
      const wrapper = ({ children }: { children: React.ReactNode }) => (
        <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
      )

      const { result } = renderHook(() => useDeleteCollection(), { wrapper })

      await act(async () => {
        await result.current.mutateAsync({ slug: 'old-collection' })
      })

      expect(mockApiRequest).toHaveBeenCalledWith('/collections/old-collection', {
        method: 'DELETE',
      })
    })
  })

  describe('useAddCollectionItem', () => {
    it('adds an item to a collection', async () => {
      mockApiRequest.mockResolvedValueOnce(undefined)

      const queryClient = createTestQueryClient()
      const wrapper = ({ children }: { children: React.ReactNode }) => (
        <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
      )

      const { result } = renderHook(() => useAddCollectionItem(), { wrapper })

      await act(async () => {
        await result.current.mutateAsync({
          slug: 'best-of',
          entityType: 'artist',
          entityId: 42,
          notes: 'Great artist',
        })
      })

      expect(mockApiRequest).toHaveBeenCalledWith('/collections/best-of/items', {
        method: 'POST',
        body: JSON.stringify({
          entity_type: 'artist',
          entity_id: 42,
          notes: 'Great artist',
        }),
      })
    })

    it('adds an item without notes', async () => {
      mockApiRequest.mockResolvedValueOnce(undefined)

      const queryClient = createTestQueryClient()
      const wrapper = ({ children }: { children: React.ReactNode }) => (
        <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
      )

      const { result } = renderHook(() => useAddCollectionItem(), { wrapper })

      await act(async () => {
        await result.current.mutateAsync({
          slug: 'best-of',
          entityType: 'release',
          entityId: 7,
        })
      })

      expect(mockApiRequest).toHaveBeenCalledWith('/collections/best-of/items', {
        method: 'POST',
        body: JSON.stringify({
          entity_type: 'release',
          entity_id: 7,
        }),
      })
    })
  })

  describe('useRemoveCollectionItem', () => {
    it('removes an item from a collection', async () => {
      mockApiRequest.mockResolvedValueOnce(undefined)

      const queryClient = createTestQueryClient()
      const wrapper = ({ children }: { children: React.ReactNode }) => (
        <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
      )

      const { result } = renderHook(() => useRemoveCollectionItem(), {
        wrapper,
      })

      await act(async () => {
        await result.current.mutateAsync({ slug: 'best-of', itemId: 5 })
      })

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/collections/best-of/items/5',
        { method: 'DELETE' }
      )
    })
  })

  describe('useSubscribeCollection', () => {
    it('subscribes to a collection', async () => {
      mockApiRequest.mockResolvedValueOnce(undefined)

      const queryClient = createTestQueryClient()
      const wrapper = ({ children }: { children: React.ReactNode }) => (
        <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
      )

      const { result } = renderHook(() => useSubscribeCollection(), {
        wrapper,
      })

      await act(async () => {
        await result.current.mutateAsync({ slug: 'best-of' })
      })

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/collections/best-of/subscribe',
        { method: 'POST' }
      )
    })
  })

  describe('useUnsubscribeCollection', () => {
    it('unsubscribes from a collection', async () => {
      mockApiRequest.mockResolvedValueOnce(undefined)

      const queryClient = createTestQueryClient()
      const wrapper = ({ children }: { children: React.ReactNode }) => (
        <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
      )

      const { result } = renderHook(() => useUnsubscribeCollection(), {
        wrapper,
      })

      await act(async () => {
        await result.current.mutateAsync({ slug: 'best-of' })
      })

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/collections/best-of/subscribe',
        { method: 'DELETE' }
      )
    })
  })
})
