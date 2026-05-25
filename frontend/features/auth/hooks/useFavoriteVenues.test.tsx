import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import {
  createWrapper,
  createWrapperWithClient,
  createTestQueryClient,
} from '@/test/utils'

const mockApiRequest = vi.fn()
const mockInvalidateFavoriteVenues = vi.fn()

vi.mock('@/lib/api', () => ({
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

vi.mock('@/lib/queryClient', () => ({
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
  useFavoriteVenues,
  useIsVenueFavorited,
  useFavoriteVenue,
  useUnfavoriteVenue,
  useFavoriteVenueToggle,
  useFavoriteVenueShows,
} from './useFavoriteVenues'


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
    mockInvalidateFavoriteVenues.mockReset()
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
    expect(mockInvalidateFavoriteVenues).toHaveBeenCalled()
  })

  it('invalidates the specific venue check key on success', async () => {
    // The Favorited badge on the venue card uses the check key. If the
    // mutation forgets to invalidate it, the user clicks "favorite" and
    // the badge stays grey until they hard-refresh.
    const queryClient = createTestQueryClient()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')
    mockApiRequest.mockResolvedValueOnce({ success: true })

    const { result } = renderHook(() => useFavoriteVenue(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await act(async () => {
      await result.current.mutateAsync(42)
    })

    // First call: top-level "favoriteVenues" prefix via the factory.
    // Second call: the per-venue check key.
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ['favoriteVenues', 'check', '42'],
    })
  })

  it('surfaces server errors to the caller and does not invalidate', async () => {
    const error = new Error('Venue already favorited')
    Object.assign(error, { status: 409 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useFavoriteVenue(), {
      wrapper: createWrapper(),
    })

    let caught: unknown
    await act(async () => {
      try {
        await result.current.mutateAsync(42)
      } catch (e) {
        caught = e
      }
    })

    expect(caught).toBe(error)
    expect(mockInvalidateFavoriteVenues).not.toHaveBeenCalled()
  })
})

describe('useUnfavoriteVenue', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockInvalidateFavoriteVenues.mockReset()
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
    expect(mockInvalidateFavoriteVenues).toHaveBeenCalled()
  })

  it('surfaces server errors to the caller and does not invalidate', async () => {
    const error = new Error('Forbidden')
    Object.assign(error, { status: 403 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useUnfavoriteVenue(), {
      wrapper: createWrapper(),
    })

    let caught: unknown
    await act(async () => {
      try {
        await result.current.mutateAsync(42)
      } catch (e) {
        caught = e
      }
    })

    expect(caught).toBe(error)
    expect(mockInvalidateFavoriteVenues).not.toHaveBeenCalled()
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

describe('useFavoriteVenues (list)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches favorite venues list with default pagination', async () => {
    const list = {
      venues: [
        { id: 1, name: 'Venue A', favorited_at: '2025-03-01T00:00:00Z' },
        { id: 2, name: 'Venue B', favorited_at: '2025-02-01T00:00:00Z' },
      ],
      total: 2,
      limit: 50,
      offset: 0,
    }
    mockApiRequest.mockResolvedValueOnce(list)

    const { result } = renderHook(() => useFavoriteVenues(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    const calledUrl = mockApiRequest.mock.calls[0][0]
    expect(calledUrl).toContain('/favorite-venues')
    expect(calledUrl).toContain('limit=50')
    expect(calledUrl).toContain('offset=0')
    expect(result.current.data).toEqual(list)
  })

  it('honors custom limit and offset', async () => {
    mockApiRequest.mockResolvedValueOnce({
      venues: [],
      total: 0,
      limit: 10,
      offset: 30,
    })

    const { result } = renderHook(
      () => useFavoriteVenues({ limit: 10, offset: 30 }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    const calledUrl = mockApiRequest.mock.calls[0][0]
    expect(calledUrl).toContain('limit=10')
    expect(calledUrl).toContain('offset=30')
  })

  it('does not fetch when enabled is false', () => {
    const { result } = renderHook(
      () => useFavoriteVenues({ enabled: false }),
      { wrapper: createWrapper() }
    )

    expect(mockApiRequest).not.toHaveBeenCalled()
    expect(result.current.fetchStatus).toBe('idle')
  })

  it('surfaces 401 unauthorized errors to the caller', async () => {
    // If the list silently 401'd to empty, an authenticated user with
    // expired session would see "no favorites" instead of being prompted
    // to re-login.
    const error = new Error('Unauthorized')
    Object.assign(error, { status: 401 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useFavoriteVenues(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect(result.current.data).toBeUndefined()
    expect((result.current.error as Error).message).toBe('Unauthorized')
  })
})

describe('useFavoriteVenueShows', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches favorite venue shows with default timezone', async () => {
    const shows = {
      shows: [],
      total: 0,
      limit: 50,
      offset: 0,
    }
    mockApiRequest.mockResolvedValueOnce(shows)

    const { result } = renderHook(() => useFavoriteVenueShows(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    const calledUrl = mockApiRequest.mock.calls[0][0] as string
    expect(calledUrl).toContain('/favorite-venues/shows')
    // The hook resolves a timezone from Intl; just assert the query
    // parameter is present, not its specific value (jsdom timezone
    // varies per environment).
    expect(calledUrl).toMatch(/timezone=/)
    expect(calledUrl).toContain('limit=50')
    expect(calledUrl).toContain('offset=0')
  })

  it('honors explicit timezone option', async () => {
    mockApiRequest.mockResolvedValueOnce({
      shows: [],
      total: 0,
      limit: 50,
      offset: 0,
    })

    const { result } = renderHook(
      () => useFavoriteVenueShows({ timezone: 'America/Phoenix' }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    const calledUrl = mockApiRequest.mock.calls[0][0] as string
    expect(calledUrl).toContain('timezone=America%2FPhoenix')
  })

  it('does not fetch when enabled is false', () => {
    const { result } = renderHook(
      () => useFavoriteVenueShows({ enabled: false }),
      { wrapper: createWrapper() }
    )

    expect(mockApiRequest).not.toHaveBeenCalled()
    expect(result.current.fetchStatus).toBe('idle')
  })

  it('surfaces errors instead of returning empty shows array', async () => {
    const error = new Error('Server error')
    Object.assign(error, { status: 500 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useFavoriteVenueShows(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect(result.current.data).toBeUndefined()
    expect((result.current.error as Error).message).toBe('Server error')
  })
})
