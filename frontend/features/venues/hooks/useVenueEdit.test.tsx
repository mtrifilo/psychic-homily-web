import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { createWrapper } from '@/test/utils'

// Create mocks
const mockApiRequest = vi.fn()
const mockInvalidateVenues = vi.fn()

// Mock the api module
vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_BASE_URL: 'http://localhost:8080',
}))

// Mock the feature api module
vi.mock('@/features/venues/api', () => ({
  venueEndpoints: {
    UPDATE: (venueId: string | number) => `/venues/${venueId}`,
    DELETE: (venueId: string | number) => `/venues/${venueId}`,
  },
}))

// Mock queryClient module — spy on the invalidation helper so we can assert
// the post-mutation cache refresh without inspecting a real QueryClient.
vi.mock('@/lib/queryClient', () => ({
  createInvalidateQueries: () => ({
    venues: mockInvalidateVenues,
  }),
}))

// Import hooks after mocks are set up
import { useVenueUpdate, useVenueDelete } from './useVenueEdit'

describe('useVenueUpdate', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockInvalidateVenues.mockReset()
  })

  it('PUTs the edit and invalidates the venues cache on success', async () => {
    const mockVenue = {
      id: 7,
      name: 'The Rebel Lounge',
      city: 'Phoenix',
      state: 'AZ',
    }
    mockApiRequest.mockResolvedValueOnce(mockVenue)

    const { result } = renderHook(() => useVenueUpdate(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      await result.current.mutateAsync({
        venueId: 7,
        data: { name: 'The Rebel Lounge', city: 'Phoenix' },
      })
    })

    expect(mockApiRequest).toHaveBeenCalledWith('/venues/7', {
      method: 'PUT',
      body: JSON.stringify({ name: 'The Rebel Lounge', city: 'Phoenix' }),
    })
    expect(mockInvalidateVenues).toHaveBeenCalled()
    await waitFor(() => expect(result.current.data).toEqual(mockVenue))
  })

  // Server-side validation failures surface as a rejected apiRequest. The
  // mutation must expose the backend message verbatim so the edit form can
  // render it, and must NOT invalidate the cache (nothing changed).
  it('surfaces server-side validation feedback and skips cache invalidation', async () => {
    const error = new Error('name is required')
    Object.assign(error, { status: 400 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useVenueUpdate(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      try {
        await result.current.mutateAsync({
          venueId: 7,
          data: { name: '' },
        })
      } catch {
        // expected — assertions below
      }
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect((result.current.error as Error).message).toBe('name is required')
    expect(mockInvalidateVenues).not.toHaveBeenCalled()
  })

  it('exposes a 403 when a non-admin attempts a direct update', async () => {
    const error = new Error('Forbidden')
    Object.assign(error, { status: 403 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useVenueUpdate(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      try {
        await result.current.mutateAsync({
          venueId: 7,
          data: { name: 'Renamed' },
        })
      } catch {
        // expected
      }
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect((result.current.error as Error).message).toBe('Forbidden')
    expect(mockInvalidateVenues).not.toHaveBeenCalled()
  })
})

describe('useVenueDelete', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockInvalidateVenues.mockReset()
  })

  it('DELETEs the venue and invalidates the venues cache on success', async () => {
    mockApiRequest.mockResolvedValueOnce(undefined)

    const { result } = renderHook(() => useVenueDelete(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      await result.current.mutateAsync(7)
    })

    expect(mockApiRequest).toHaveBeenCalledWith('/venues/7', {
      method: 'DELETE',
    })
    expect(mockInvalidateVenues).toHaveBeenCalled()
  })

  // Venues with associated shows can't be deleted; the backend rejects with a
  // 409. The mutation should pass the message through and leave the cache alone.
  it('surfaces the constraint error when the venue has associated shows', async () => {
    const error = new Error('venue has associated shows and cannot be deleted')
    Object.assign(error, { status: 409 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useVenueDelete(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      try {
        await result.current.mutateAsync(7)
      } catch {
        // expected
      }
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect((result.current.error as Error).message).toBe(
      'venue has associated shows and cannot be deleted'
    )
    expect(mockInvalidateVenues).not.toHaveBeenCalled()
  })
})
