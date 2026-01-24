import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

// Create mocks
const mockApiRequest = vi.fn()
const mockInvalidateVenues = vi.fn()

// Mock the api module
vi.mock('../api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    VENUES: {
      UPDATE: (id: number) => `/venues/${id}`,
      DELETE: (id: number) => `/venues/${id}`,
      MY_PENDING_EDIT: (id: number) => `/venues/${id}/my-pending-edit`,
    },
  },
}))

// Mock queryClient module
vi.mock('../queryClient', () => ({
  queryKeys: {
    venues: {
      myPendingEdit: (venueId: number) => ['venues', 'myPendingEdit', venueId],
    },
  },
  createInvalidateQueries: () => ({
    venues: mockInvalidateVenues,
  }),
}))

// Import hooks after mocks are set up
import {
  useVenueUpdate,
  useMyPendingVenueEdit,
  useCancelPendingVenueEdit,
  useVenueDelete,
} from './useVenueEdit'

// Helper to create wrapper with query client
function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        gcTime: 0,
      },
      mutations: {
        retry: false,
      },
    },
  })
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    )
  }
}

describe('useVenueUpdate', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockInvalidateVenues.mockReset()
  })

  it('updates a venue with correct endpoint and payload', async () => {
    const mockResponse = {
      venue: { id: 1, name: 'Updated Venue', city: 'Phoenix', state: 'AZ' },
      status: 'updated',
      message: 'Venue updated successfully',
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useVenueUpdate(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({
        venueId: 1,
        data: { name: 'Updated Venue', address: '123 Main St' },
      })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/venues/1',
      expect.objectContaining({
        method: 'PUT',
        body: JSON.stringify({ name: 'Updated Venue', address: '123 Main St' }),
      })
    )
  })

  it('returns updated venue data on success', async () => {
    const mockResponse = {
      venue: {
        id: 2,
        name: 'New Name',
        address: '456 Oak Ave',
        city: 'Tempe',
        state: 'AZ',
      },
      status: 'updated',
      message: 'Venue updated',
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useVenueUpdate(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({
        venueId: 2,
        data: { name: 'New Name' },
      })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(result.current.data?.venue?.name).toBe('New Name')
    expect(result.current.data?.status).toBe('updated')
  })

  it('returns pending status for non-admin updates', async () => {
    const mockResponse = {
      pending_edit: {
        id: 10,
        venue_id: 3,
        submitted_by: 5,
        status: 'pending',
        name: 'Pending Name Change',
      },
      status: 'pending',
      message: 'Edit submitted for approval',
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useVenueUpdate(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({
        venueId: 3,
        data: { name: 'Pending Name Change' },
      })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(result.current.data?.status).toBe('pending')
    expect(result.current.data?.pending_edit?.name).toBe('Pending Name Change')
  })

  it('invalidates venues on success', async () => {
    mockApiRequest.mockResolvedValueOnce({
      venue: { id: 4 },
      status: 'updated',
      message: 'Updated',
    })

    const { result } = renderHook(() => useVenueUpdate(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({
        venueId: 4,
        data: { city: 'Scottsdale' },
      })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockInvalidateVenues).toHaveBeenCalled()
  })

  it('handles update errors', async () => {
    const error = new Error('Not authorized')
    Object.assign(error, { status: 403 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useVenueUpdate(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({
        venueId: 999,
        data: { name: 'Unauthorized Update' },
      })
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect(result.current.error).toBeDefined()
  })

  it('supports updating social links', async () => {
    mockApiRequest.mockResolvedValueOnce({
      venue: { id: 5 },
      status: 'updated',
      message: 'Updated',
    })

    const { result } = renderHook(() => useVenueUpdate(), {
      wrapper: createWrapper(),
    })

    const socialData = {
      instagram: 'venue_instagram',
      website: 'https://venue.com',
      facebook: 'venue.facebook',
    }

    await act(async () => {
      result.current.mutate({
        venueId: 5,
        data: socialData,
      })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/venues/5',
      expect.objectContaining({
        body: JSON.stringify(socialData),
      })
    )
  })
})

describe('useMyPendingVenueEdit', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches pending edit for a venue', async () => {
    const mockResponse = {
      pending_edit: {
        id: 1,
        venue_id: 10,
        submitted_by: 5,
        status: 'pending',
        name: 'Pending Name',
        created_at: '2025-01-15T00:00:00Z',
      },
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useMyPendingVenueEdit(10), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/venues/10/my-pending-edit',
      expect.objectContaining({
        method: 'GET',
      })
    )
    expect(result.current.data?.pending_edit?.name).toBe('Pending Name')
  })

  it('returns null pending_edit when none exists', async () => {
    mockApiRequest.mockResolvedValueOnce({
      pending_edit: null,
    })

    const { result } = renderHook(() => useMyPendingVenueEdit(20), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(result.current.data?.pending_edit).toBeNull()
  })

  it('does not fetch when venueId is 0', async () => {
    const { result } = renderHook(() => useMyPendingVenueEdit(0), {
      wrapper: createWrapper(),
    })

    // Should not be fetching due to enabled condition
    expect(result.current.isFetching).toBe(false)
    expect(mockApiRequest).not.toHaveBeenCalled()
  })

  it('does not fetch when enabled is false', async () => {
    const { result } = renderHook(() => useMyPendingVenueEdit(10, false), {
      wrapper: createWrapper(),
    })

    expect(result.current.isFetching).toBe(false)
    expect(mockApiRequest).not.toHaveBeenCalled()
  })

  it('handles fetch errors', async () => {
    const error = new Error('Not found')
    Object.assign(error, { status: 404 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useMyPendingVenueEdit(999), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect(result.current.error).toBeDefined()
  })
})

describe('useCancelPendingVenueEdit', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('cancels a pending edit with correct endpoint', async () => {
    mockApiRequest.mockResolvedValueOnce({
      message: 'Pending edit cancelled',
    })

    const { result } = renderHook(() => useCancelPendingVenueEdit(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(15)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/venues/15/my-pending-edit',
      expect.objectContaining({
        method: 'DELETE',
      })
    )
  })

  it('returns success message on cancellation', async () => {
    mockApiRequest.mockResolvedValueOnce({
      message: 'Edit successfully cancelled',
    })

    const { result } = renderHook(() => useCancelPendingVenueEdit(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(25)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(result.current.data?.message).toBe('Edit successfully cancelled')
  })

  it('handles cancellation errors', async () => {
    const error = new Error('No pending edit found')
    Object.assign(error, { status: 404 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useCancelPendingVenueEdit(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(999)
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect(result.current.error).toBeDefined()
  })
})

describe('useVenueDelete', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockInvalidateVenues.mockReset()
  })

  it('deletes a venue with correct endpoint', async () => {
    mockApiRequest.mockResolvedValueOnce(undefined)

    const { result } = renderHook(() => useVenueDelete(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(30)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/venues/30',
      expect.objectContaining({
        method: 'DELETE',
      })
    )
  })

  it('invalidates venues on success', async () => {
    mockApiRequest.mockResolvedValueOnce(undefined)

    const { result } = renderHook(() => useVenueDelete(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(40)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockInvalidateVenues).toHaveBeenCalled()
  })

  it('handles deletion errors', async () => {
    const error = new Error('Cannot delete venue with associated shows')
    Object.assign(error, { status: 400 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useVenueDelete(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(50)
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect((result.current.error as Error).message).toBe(
      'Cannot delete venue with associated shows'
    )
  })

  it('handles unauthorized deletion', async () => {
    const error = new Error('Not authorized to delete this venue')
    Object.assign(error, { status: 403 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useVenueDelete(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(60)
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect(result.current.error).toBeDefined()
  })
})
