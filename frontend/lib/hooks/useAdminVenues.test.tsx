import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createWrapper, createTestQueryClient } from '@/test/utils'

// Create mocks
const mockApiRequest = vi.fn()
const mockInvalidateVenues = vi.fn()

// Mock the api module
vi.mock('../api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    ADMIN: {
      VENUES: {
        VERIFY: (venueId: number) => `/admin/venues/${venueId}/verify`,
      },
    },
  },
  API_BASE_URL: 'http://localhost:8080',
}))

// Mock queryClient module
vi.mock('../queryClient', () => ({
  createInvalidateQueries: () => ({
    venues: mockInvalidateVenues,
  }),
}))

// Import hooks after mocks are set up
import { useVerifyVenue } from './useAdminVenues'

// Helper to create wrapper with specific query client
function createWrapperWithClient(queryClient: QueryClient) {
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    )
  }
}

describe('useAdminVenues', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockInvalidateVenues.mockReset()
  })

  describe('useVerifyVenue', () => {
    it('verifies a venue', async () => {
      const mockResponse = {
        id: 1,
        name: 'Verified Venue',
        is_verified: true,
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

    it('returns verified venue data on success', async () => {
      const mockVenue = {
        id: 10,
        name: 'The Rebel Lounge',
        city: 'Phoenix',
        state: 'AZ',
        is_verified: true,
      }
      mockApiRequest.mockResolvedValueOnce(mockVenue)

      const { result } = renderHook(() => useVerifyVenue(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate(10)
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(result.current.data?.name).toBe('The Rebel Lounge')
      expect(result.current.data?.is_verified).toBe(true)
    })

    it('invalidates venues and pending shows on success', async () => {
      mockApiRequest.mockResolvedValueOnce({ id: 20, is_verified: true })

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

    it('handles unauthorized error', async () => {
      const error = new Error('Forbidden')
      Object.assign(error, { status: 403 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => useVerifyVenue(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate(1)
      })

      await waitFor(() => expect(result.current.isError).toBe(true))

      expect((result.current.error as Error).message).toBe('Forbidden')
    })

    it('handles server error', async () => {
      const error = new Error('Server error')
      Object.assign(error, { status: 500 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => useVerifyVenue(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate(1)
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
    })
  })
})
