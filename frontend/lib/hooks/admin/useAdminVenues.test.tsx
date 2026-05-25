import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { createWrapper, createWrapperWithClient, createTestQueryClient } from '@/test/utils'

// Create mocks
const mockApiRequest = vi.fn()
const mockInvalidateVenues = vi.fn()

// Mock the api module
vi.mock('../../api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    ADMIN: {
      VENUES: {
        UNVERIFIED: '/admin/venues/unverified',
        VERIFY: (venueId: number) => `/admin/venues/${venueId}/verify`,
      },
    },
  },
  API_BASE_URL: 'http://localhost:8080',
}))

// Mock queryClient module
vi.mock('../../queryClient', () => ({
  queryKeys: {
    admin: {
      unverifiedVenues: (limit: number, offset: number) =>
        ['admin', 'venues', 'unverified', { limit, offset }],
    },
  },
  createInvalidateQueries: () => ({
    venues: mockInvalidateVenues,
  }),
}))

// Import hooks after mocks are set up
import { useUnverifiedVenues, useVerifyVenue } from './useAdminVenues'


describe('useAdminVenues', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockInvalidateVenues.mockReset()
  })

  describe('useUnverifiedVenues', () => {
    it('fetches with default pagination', async () => {
      mockApiRequest.mockResolvedValueOnce({ venues: [], total: 0 })

      const { result } = renderHook(() => useUnverifiedVenues(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))
      const url = mockApiRequest.mock.calls[0][0] as string
      expect(url).toContain('/admin/venues/unverified')
      expect(url).toContain('limit=50')
      expect(url).toContain('offset=0')
    })

    it('passes custom limit and offset', async () => {
      mockApiRequest.mockResolvedValueOnce({ venues: [], total: 0 })

      const { result } = renderHook(
        () => useUnverifiedVenues({ limit: 25, offset: 50 }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))
      const url = mockApiRequest.mock.calls[0][0] as string
      expect(url).toContain('limit=25')
      expect(url).toContain('offset=50')
    })

    it('does not fetch when enabled=false', () => {
      const { result } = renderHook(
        () => useUnverifiedVenues({ enabled: false }),
        { wrapper: createWrapper() }
      )

      expect(result.current.fetchStatus).toBe('idle')
      expect(mockApiRequest).not.toHaveBeenCalled()
    })

    it('surfaces fetch errors', async () => {
      mockApiRequest.mockRejectedValueOnce(new Error('Forbidden'))

      const { result } = renderHook(() => useUnverifiedVenues(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
      expect((result.current.error as Error).message).toBe('Forbidden')
    })
  })

  describe('useVerifyVenue', () => {
    it('verifies a venue', async () => {
      const mockResponse = {
        id: 1,
        name: 'Verified Venue',
        verified: true,
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(() => useVerifyVenue(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate(1)
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith('/admin/venues/1/verify', {
        method: 'POST',
      })
    })

    it('invalidates venues, pending shows, AND the unverified list on success', async () => {
      // Verifying flips the venue out of the unverified queue — the queue
      // itself must invalidate so the row disappears without a refresh.
      mockApiRequest.mockResolvedValueOnce({ id: 20, verified: true })

      const queryClient = createTestQueryClient()
      const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

      const { result } = renderHook(() => useVerifyVenue(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      await act(async () => {
        result.current.mutate(20)
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockInvalidateVenues).toHaveBeenCalled()
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ['admin', 'shows', 'pending'],
      })
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ['admin', 'venues', 'unverified'],
      })
    })

    it('handles venue not found error', async () => {
      const error = new Error('Venue not found')
      Object.assign(error, { status: 404 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => useVerifyVenue(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate(999)
      })

      await waitFor(() => expect(result.current.isError).toBe(true))

      expect((result.current.error as Error).message).toBe('Venue not found')
    })

  })
})
