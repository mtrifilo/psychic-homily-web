import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createWrapper, createTestQueryClient } from '@/test/utils'

// Create mocks
const mockApiRequest = vi.fn()

// Mock the api module
vi.mock('../api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    SHOWS: {
      UPCOMING: '/shows/upcoming',
      GET: (id: string | number) => `/shows/${id}`,
    },
  },
  API_BASE_URL: 'http://localhost:8080',
}))

// Mock queryClient module
vi.mock('../queryClient', () => ({
  queryKeys: {
    shows: {
      list: (filters?: Record<string, unknown>) => ['shows', 'list', filters],
      detail: (id: string) => ['shows', 'detail', id],
    },
  },
}))

// Import hooks after mocks are set up
import { useUpcomingShows, useShow } from './useShows'

// Helper to create wrapper with specific query client
function createWrapperWithClient(queryClient: QueryClient) {
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    )
  }
}

describe('useShows', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  describe('useUpcomingShows', () => {
    it('fetches upcoming shows with default options', async () => {
      const mockResponse = {
        shows: [
          { id: 1, title: 'Show 1' },
          { id: 2, title: 'Show 2' },
        ],
        has_more: false,
        next_cursor: null,
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(() => useUpcomingShows(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith('/shows/upcoming', {
        method: 'GET',
      })
      expect(result.current.data?.shows).toHaveLength(2)
    })

    it('includes timezone in query params when provided', async () => {
      mockApiRequest.mockResolvedValueOnce({ shows: [], has_more: false })

      const { result } = renderHook(
        () => useUpcomingShows({ timezone: 'America/Phoenix' }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/shows/upcoming?timezone=America%2FPhoenix',
        { method: 'GET' }
      )
    })

    it('includes cursor for pagination', async () => {
      mockApiRequest.mockResolvedValueOnce({ shows: [], has_more: false })

      const { result } = renderHook(
        () => useUpcomingShows({ cursor: 'abc123' }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/shows/upcoming?cursor=abc123',
        { method: 'GET' }
      )
    })

    it('includes limit in query params', async () => {
      mockApiRequest.mockResolvedValueOnce({ shows: [], has_more: false })

      const { result } = renderHook(() => useUpcomingShows({ limit: 10 }), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/shows/upcoming?limit=10',
        { method: 'GET' }
      )
    })

    it('combines multiple query params', async () => {
      mockApiRequest.mockResolvedValueOnce({ shows: [], has_more: false })

      const { result } = renderHook(
        () =>
          useUpcomingShows({
            timezone: 'America/Phoenix',
            cursor: 'page2',
            limit: 25,
          }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      // URL should contain all params
      const calledUrl = mockApiRequest.mock.calls[0][0]
      expect(calledUrl).toContain('timezone=America%2FPhoenix')
      expect(calledUrl).toContain('cursor=page2')
      expect(calledUrl).toContain('limit=25')
    })

    it('handles API errors', async () => {
      const error = new Error('Server error')
      Object.assign(error, { status: 500 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => useUpcomingShows(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isError).toBe(true))

      expect(result.current.error).toBeDefined()
    })

    it('returns has_more flag for pagination', async () => {
      mockApiRequest.mockResolvedValueOnce({
        shows: [{ id: 1 }],
        has_more: true,
        next_cursor: 'next-page',
      })

      const { result } = renderHook(() => useUpcomingShows(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(result.current.data?.has_more).toBe(true)
      expect(result.current.data?.next_cursor).toBe('next-page')
    })
  })

  describe('useShow', () => {
    it('fetches a single show by ID', async () => {
      const mockShow = {
        id: 123,
        title: 'Test Show',
        event_date: '2025-03-15T20:00:00Z',
        venues: [{ id: 1, name: 'The Venue' }],
        artists: [{ id: 1, name: 'The Artist' }],
      }
      mockApiRequest.mockResolvedValueOnce(mockShow)

      const { result } = renderHook(() => useShow(123), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith('/shows/123', {
        method: 'GET',
      })
      expect(result.current.data?.title).toBe('Test Show')
    })

    it('accepts string show ID', async () => {
      mockApiRequest.mockResolvedValueOnce({ id: 456, title: 'Show' })

      const { result } = renderHook(() => useShow('456'), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith('/shows/456', {
        method: 'GET',
      })
    })

    it('does not fetch when showId is falsy', async () => {
      const { result } = renderHook(() => useShow(''), {
        wrapper: createWrapper(),
      })

      // Should remain in loading state without making a request
      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result.current.isLoading).toBe(false)
      expect(result.current.fetchStatus).toBe('idle')
    })

    it('handles show not found error', async () => {
      const error = new Error('Show not found')
      Object.assign(error, { status: 404 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => useShow(999), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isError).toBe(true))

      expect((result.current.error as Error).message).toBe('Show not found')
    })
  })
})
