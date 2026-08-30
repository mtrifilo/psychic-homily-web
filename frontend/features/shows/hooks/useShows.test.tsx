import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient } from '@tanstack/react-query'
import {
  createWrapper,
  createWrapperWithClient,
  createTestQueryClient,
} from '@/test/utils'

// Create mocks
const mockApiRequest = vi.fn()

// Mock the api module
vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_BASE_URL: 'http://localhost:8080',
}))

// Mock the feature api module. The pairing between these stubs and the REAL
// constants is enforced separately by useShowsFirstScreen.test.tsx, which does
// not mock this module — a stub that drifted from it would let the assertions
// below pass against a URL production never sends.
vi.mock('@/features/shows/api', () => ({
  showEndpoints: {
    UPCOMING: '/shows/upcoming',
    CITIES: '/shows/cities',
    GET: (id: string | number) => `/shows/${id}`,
    ALSO_TONIGHT: (id: string | number) => `/shows/${id}/also-tonight`,
  },
  showQueryKeys: {
    list: (filters?: Record<string, unknown>) => ['shows', 'list', filters],
    detail: (id: string) => ['shows', 'detail', id],
    cities: () => ['shows', 'cities'],
    alsoTonight: (id: string) => ['shows', 'also-tonight', id],
  },
  SHOW_CITIES_FIRST_SCREEN_URL: '/shows/cities',
}))

// Import hooks after mocks are set up
import { useUpcomingShows, useShow, useShowAlsoTonight } from './useShows'


describe('useShows', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  describe('useUpcomingShows', () => {
    // The URL asserted below is the BARE endpoint — no query string and no
    // trailing `?`. That is what the server fetches to seed the first screen, so
    // a stray character makes the seed a different Data Cache entry from the one
    // the hook asks for and `/shows` silently stops being server-rendered.
    // Nothing per-viewer may creep back in either: a viewer zone is what
    // PSY-1678 removed, and it would land back in the cache key.
    it('fetches upcoming shows with default options', async () => {
      const mockResponse = {
        shows: [
          { id: 1, title: 'Show 1' },
          { id: 2, title: 'Show 2' },
        ],
        has_more: false,
        next_cursor: null as string | null,
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(() => useUpcomingShows(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith('/shows/upcoming', {
        method: 'GET',
      })
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
            cursor: 'page2',
            limit: 25,
            city: 'Phoenix',
            state: 'AZ',
          }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      // URL should contain all params
      const calledUrl = mockApiRequest.mock.calls[0][0]
      expect(calledUrl).toContain('cursor=page2')
      expect(calledUrl).toContain('limit=25')
      expect(calledUrl).toContain('city=Phoenix')
      expect(calledUrl).toContain('state=AZ')
    })

    it('returns has_more flag for pagination', async () => {
      mockApiRequest.mockResolvedValueOnce({
        shows: [{ id: 1 }],
        pagination: {
          has_more: true,
          next_cursor: 'next-page',
          limit: 25,
        },
      })

      const { result } = renderHook(() => useUpcomingShows(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(result.current.data?.pagination.has_more).toBe(true)
      expect(result.current.data?.pagination.next_cursor).toBe('next-page')
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

  describe('useShowAlsoTonight', () => {
    it('fetches the rail from the show sub-route', async () => {
      mockApiRequest.mockResolvedValueOnce({ date: '2026-08-12', shows: [] })

      const { result } = renderHook(() => useShowAlsoTonight('desert-doom'), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/shows/desert-doom/also-tonight',
        { method: 'GET' }
      )
    })

    it('addresses the show by numeric id too', async () => {
      mockApiRequest.mockResolvedValueOnce({ date: '2026-08-12', shows: [] })

      const { result } = renderHook(() => useShowAlsoTonight(42), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith('/shows/42/also-tonight', {
        method: 'GET',
      })
    })

    it('does not fetch when showId is falsy', async () => {
      const { result } = renderHook(() => useShowAlsoTonight(''), {
        wrapper: createWrapper(),
      })

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result.current.fetchStatus).toBe('idle')
    })

    it('lands on the also-tonight key, not the show detail key', async () => {
      // The two are addressed by the same id and invalidated independently.
      const queryClient = createTestQueryClient()
      mockApiRequest.mockResolvedValueOnce({ date: '2026-08-12', shows: [] })

      const { result } = renderHook(() => useShowAlsoTonight('42'), {
        wrapper: createWrapperWithClient(queryClient),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(
        queryClient.getQueryData(['shows', 'also-tonight', '42'])
      ).toBeDefined()
      expect(queryClient.getQueryData(['shows', 'detail', '42'])).toBeUndefined()
    })

    it('surfaces a failure rather than pretending the night was quiet', async () => {
      // The component hides the rail on error, but the hook must still report
      // one — an empty night is a 200 with an empty list, not an error.
      const error = new Error('Show not found')
      Object.assign(error, { status: 404 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => useShowAlsoTonight(999), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
      expect(result.current.data).toBeUndefined()
    })
  })
})
