import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

const mockApiRequest = vi.fn()
const mockInvalidateFavoriteVenues = vi.fn()

vi.mock('../api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    FAVORITE_VENUES: {
      LIST: '/favorite-venues',
      FAVORITE: (venueId: string | number) => `/favorite-venues/${venueId}`,
      UNFAVORITE: (venueId: string | number) => `/favorite-venues/${venueId}`,
      CHECK: (venueId: string | number) => `/favorite-venues/${venueId}/check`,
      SHOWS: '/favorite-venues/shows',
    },
  },
}))

vi.mock('../queryClient', () => ({
  queryKeys: {
    favoriteVenues: {
      all: ['favoriteVenues'],
      list: () => ['favoriteVenues', 'list'],
      check: (venueId: string) => ['favoriteVenues', 'check', venueId],
      shows: (params: unknown) => ['favoriteVenues', 'shows', params],
    },
  },
  createInvalidateQueries: () => ({
    favoriteVenues: mockInvalidateFavoriteVenues,
  }),
}))

import {
  useIsVenueFavorited,
  useFavoriteVenue,
  useUnfavoriteVenue,
  useFavoriteVenueToggle,
} from './useFavoriteVenues'

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0 },
      mutations: { retry: false },
    },
  })
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    )
  }
}

describe('useIsVenueFavorited', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('returns favorited status when authenticated', async () => {
    mockApiRequest.mockResolvedValueOnce({ is_favorited: true })

    const { result } = renderHook(
      () => useIsVenueFavorited(42, true),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data?.is_favorited).toBe(true)
  })

  it('does not fetch when not authenticated', () => {
    const { result } = renderHook(
      () => useIsVenueFavorited(42, false),
      { wrapper: createWrapper() }
    )

    expect(result.current.fetchStatus).toBe('idle')
    expect(mockApiRequest).not.toHaveBeenCalled()
  })

  it('does not fetch when venueId is null', () => {
    const { result } = renderHook(
      () => useIsVenueFavorited(null, true),
      { wrapper: createWrapper() }
    )

    expect(result.current.fetchStatus).toBe('idle')
  })
})

describe('useFavoriteVenue', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('calls favorite API and invalidates queries on success', async () => {
    mockApiRequest.mockResolvedValueOnce({ success: true })

    const { result } = renderHook(() => useFavoriteVenue(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      await result.current.mutateAsync(42)
    })

    expect(mockApiRequest).toHaveBeenCalledWith('/favorite-venues/42', {
      method: 'POST',
    })
  })
})

describe('useUnfavoriteVenue', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('calls unfavorite API', async () => {
    mockApiRequest.mockResolvedValueOnce({ success: true })

    const { result } = renderHook(() => useUnfavoriteVenue(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      await result.current.mutateAsync(42)
    })

    expect(mockApiRequest).toHaveBeenCalledWith('/favorite-venues/42', {
      method: 'DELETE',
    })
  })
})

describe('useFavoriteVenueToggle', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('returns isFavorited false when no data', () => {
    // Don't resolve the check query — leave it pending
    mockApiRequest.mockReturnValue(new Promise(() => {}))

    const { result } = renderHook(
      () => useFavoriteVenueToggle(42, true),
      { wrapper: createWrapper() }
    )

    expect(result.current.isFavorited).toBe(false)
  })

  it('returns isFavorited true when API returns favorited', async () => {
    mockApiRequest.mockResolvedValueOnce({ is_favorited: true })

    const { result } = renderHook(
      () => useFavoriteVenueToggle(42, true),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isFavorited).toBe(true))
  })

  it('returns isFavorited false for unauthenticated user', () => {
    const { result } = renderHook(
      () => useFavoriteVenueToggle(42, false),
      { wrapper: createWrapper() }
    )

    expect(result.current.isFavorited).toBe(false)
    expect(mockApiRequest).not.toHaveBeenCalled()
  })

  it('toggle calls favorite mutation when not favorited', async () => {
    // Check query returns not favorited
    mockApiRequest.mockResolvedValueOnce({ is_favorited: false })

    const { result } = renderHook(
      () => useFavoriteVenueToggle(42, true),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isFavorited).toBe(false))

    // Resolve the favorite mutation
    mockApiRequest.mockResolvedValueOnce({ success: true })

    await act(async () => {
      await result.current.toggle()
    })

    // Should have called POST (favorite)
    expect(mockApiRequest).toHaveBeenCalledWith('/favorite-venues/42', {
      method: 'POST',
    })
  })

  it('toggle calls unfavorite mutation when favorited', async () => {
    mockApiRequest.mockResolvedValueOnce({ is_favorited: true })

    const { result } = renderHook(
      () => useFavoriteVenueToggle(42, true),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isFavorited).toBe(true))

    mockApiRequest.mockResolvedValueOnce({ success: true })

    await act(async () => {
      await result.current.toggle()
    })

    expect(mockApiRequest).toHaveBeenCalledWith('/favorite-venues/42', {
      method: 'DELETE',
    })
  })

  it('performs optimistic update on toggle', async () => {
    mockApiRequest.mockResolvedValueOnce({ is_favorited: false })

    const { result } = renderHook(
      () => useFavoriteVenueToggle(42, true),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isFavorited).toBe(false))

    // Make mutation hang to observe optimistic state
    let resolveMutation: (v: unknown) => void
    mockApiRequest.mockReturnValueOnce(
      new Promise(resolve => {
        resolveMutation = resolve
      })
    )

    act(() => {
      result.current.toggle()
    })

    // Optimistic update should flip to true immediately
    await waitFor(() => expect(result.current.isFavorited).toBe(true))

    // Resolve mutation
    await act(async () => {
      resolveMutation!({ success: true })
    })
  })

  it('rolls back optimistic update on mutation error', async () => {
    mockApiRequest.mockResolvedValueOnce({ is_favorited: false })

    const { result } = renderHook(
      () => useFavoriteVenueToggle(42, true),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isFavorited).toBe(false))

    // Make mutation fail
    mockApiRequest.mockRejectedValueOnce(new Error('Network error'))

    await act(async () => {
      try {
        await result.current.toggle()
      } catch {
        // Expected
      }
    })

    // Should roll back to false
    await waitFor(() => expect(result.current.isFavorited).toBe(false))
  })

  it('toggle is a no-op when isLoading is true (race condition guard)', async () => {
    mockApiRequest.mockResolvedValueOnce({ is_favorited: false })

    const { result } = renderHook(
      () => useFavoriteVenueToggle(42, true),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isFavorited).toBe(false))

    // Make first mutation hang (never resolve)
    mockApiRequest.mockReturnValueOnce(new Promise(() => {}))

    // Start first toggle
    act(() => {
      result.current.toggle()
    })

    await waitFor(() => expect(result.current.isLoading).toBe(true))

    // Record call count before second toggle attempt
    const callsBefore = mockApiRequest.mock.calls.length

    // Attempt second toggle — should be a no-op
    await act(async () => {
      await result.current.toggle()
    })

    // No additional API calls should have been made
    expect(mockApiRequest.mock.calls.length).toBe(callsBefore)
  })

  it('exposes error from failed mutation', async () => {
    mockApiRequest.mockResolvedValueOnce({ is_favorited: false })

    const { result } = renderHook(
      () => useFavoriteVenueToggle(42, true),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isFavorited).toBe(false))

    mockApiRequest.mockRejectedValueOnce(new Error('Server error'))

    await act(async () => {
      try {
        await result.current.toggle()
      } catch {
        // Expected
      }
    })

    await waitFor(() => expect(result.current.error).toBeTruthy())
    expect(result.current.error?.message).toBe('Server error')
  })

  it('isLoading is true during pending mutation', async () => {
    mockApiRequest.mockResolvedValueOnce({ is_favorited: false })

    const { result } = renderHook(
      () => useFavoriteVenueToggle(42, true),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isFavorited).toBe(false))
    expect(result.current.isLoading).toBe(false)

    let resolveMutation: (v: unknown) => void
    mockApiRequest.mockReturnValueOnce(
      new Promise(resolve => {
        resolveMutation = resolve
      })
    )

    act(() => {
      result.current.toggle()
    })

    await waitFor(() => expect(result.current.isLoading).toBe(true))

    await act(async () => {
      resolveMutation!({ success: true })
    })

    await waitFor(() => expect(result.current.isLoading).toBe(false))
  })
})
