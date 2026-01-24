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
        PENDING_EDITS: '/admin/venues/pending-edits',
        APPROVE_EDIT: (editId: number) =>
          `/admin/venues/pending-edits/${editId}/approve`,
        REJECT_EDIT: (editId: number) =>
          `/admin/venues/pending-edits/${editId}/reject`,
      },
    },
  },
  API_BASE_URL: 'http://localhost:8080',
}))

// Mock queryClient module
vi.mock('../queryClient', () => ({
  queryKeys: {
    admin: {
      pendingVenueEdits: (limit: number, offset: number) => [
        'admin',
        'venues',
        'pendingEdits',
        { limit, offset },
      ],
    },
  },
  createInvalidateQueries: () => ({
    venues: mockInvalidateVenues,
  }),
}))

// Import hooks after mocks are set up
import {
  usePendingVenueEdits,
  useApproveVenueEdit,
  useRejectVenueEdit,
} from './useAdminVenueEdits'

// Helper to create wrapper with specific query client
function createWrapperWithClient(queryClient: QueryClient) {
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    )
  }
}

describe('useAdminVenueEdits', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockInvalidateVenues.mockReset()
  })

  describe('usePendingVenueEdits', () => {
    it('fetches pending venue edits with default options', async () => {
      const mockResponse = {
        edits: [
          {
            id: 1,
            venue_id: 10,
            changes: { name: 'New Name' },
            status: 'pending',
          },
          {
            id: 2,
            venue_id: 20,
            changes: { address: 'New Address' },
            status: 'pending',
          },
        ],
        total: 2,
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(() => usePendingVenueEdits(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/admin/venues/pending-edits?limit=50&offset=0'
      )
      expect(result.current.data?.edits).toHaveLength(2)
    })

    it('supports custom limit and offset', async () => {
      mockApiRequest.mockResolvedValueOnce({ edits: [], total: 0 })

      const { result } = renderHook(
        () => usePendingVenueEdits({ limit: 25, offset: 50 }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/admin/venues/pending-edits?limit=25&offset=50'
      )
    })

    it('handles authentication error', async () => {
      const error = new Error('Forbidden')
      Object.assign(error, { status: 403 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => usePendingVenueEdits(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isError).toBe(true))

      expect((result.current.error as Error).message).toBe('Forbidden')
    })

    it('handles API errors', async () => {
      const error = new Error('Server error')
      Object.assign(error, { status: 500 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => usePendingVenueEdits(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
    })

    it('returns empty list when no pending edits', async () => {
      mockApiRequest.mockResolvedValueOnce({ edits: [], total: 0 })

      const { result } = renderHook(() => usePendingVenueEdits(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(result.current.data?.edits).toHaveLength(0)
    })
  })

  describe('useApproveVenueEdit', () => {
    it('approves a venue edit', async () => {
      const mockResponse = {
        id: 10,
        name: 'Updated Venue',
        city: 'Phoenix',
        state: 'AZ',
      }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(() => useApproveVenueEdit(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate(123)
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/admin/venues/pending-edits/123/approve',
        { method: 'POST' }
      )
    })

    it('returns updated venue data on success', async () => {
      const mockVenue = {
        id: 10,
        name: 'Crescent Ballroom',
        city: 'Phoenix',
        state: 'AZ',
      }
      mockApiRequest.mockResolvedValueOnce(mockVenue)

      const { result } = renderHook(() => useApproveVenueEdit(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate(456)
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(result.current.data?.name).toBe('Crescent Ballroom')
    })

    it('invalidates pending edits and venues on success', async () => {
      mockApiRequest.mockResolvedValueOnce({ id: 10 })

      const queryClient = createTestQueryClient()
      const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

      const { result } = renderHook(() => useApproveVenueEdit(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      await act(async () => {
        result.current.mutate(789)
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ['admin', 'venues', 'pendingEdits'],
      })
      expect(mockInvalidateVenues).toHaveBeenCalled()
    })

    it('handles not found error', async () => {
      const error = new Error('Edit not found')
      Object.assign(error, { status: 404 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => useApproveVenueEdit(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate(999)
      })

      await waitFor(() => expect(result.current.isError).toBe(true))

      expect((result.current.error as Error).message).toBe('Edit not found')
    })

    it('handles unauthorized error', async () => {
      const error = new Error('Forbidden')
      Object.assign(error, { status: 403 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => useApproveVenueEdit(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate(123)
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
    })
  })

  describe('useRejectVenueEdit', () => {
    it('rejects a venue edit with a reason', async () => {
      const mockResponse = { id: 1, status: 'rejected' }
      mockApiRequest.mockResolvedValueOnce(mockResponse)

      const { result } = renderHook(() => useRejectVenueEdit(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({ editId: 123, reason: 'Invalid address' })
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/admin/venues/pending-edits/123/reject',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({ reason: 'Invalid address' }),
        })
      )
    })

    it('invalidates pending edits on success', async () => {
      mockApiRequest.mockResolvedValueOnce({ id: 456 })

      const queryClient = createTestQueryClient()
      const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

      const { result } = renderHook(() => useRejectVenueEdit(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      await act(async () => {
        result.current.mutate({ editId: 456, reason: 'Spam submission' })
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ['admin', 'venues', 'pendingEdits'],
      })
    })

    it('handles not found error', async () => {
      const error = new Error('Edit not found')
      Object.assign(error, { status: 404 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => useRejectVenueEdit(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({ editId: 999, reason: 'Test' })
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
    })

    it('handles unauthorized error', async () => {
      const error = new Error('Forbidden')
      Object.assign(error, { status: 403 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => useRejectVenueEdit(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({ editId: 123, reason: 'Test' })
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
    })
  })
})
