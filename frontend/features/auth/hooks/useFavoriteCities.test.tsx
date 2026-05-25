import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { createWrapper, createWrapperWithClient, createTestQueryClient } from '@/test/utils'

// Create mocks
const mockApiRequest = vi.fn()

// Mock the api module
vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    AUTH: {
      FAVORITE_CITIES: '/auth/preferences/favorite-cities',
    },
  },
  API_BASE_URL: 'http://localhost:8080',
}))

// Mock queryClient module
vi.mock('@/lib/queryClient', () => ({
  queryKeys: {
    auth: {
      profile: ['auth', 'profile'],
    },
  },
}))

// Import hooks after mocks are set up
import { useSetFavoriteCities } from './useFavoriteCities'

describe('useSetFavoriteCities', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('saves favorite cities and invalidates profile query', async () => {
    const mockResponse = {
      success: true,
      message: 'Favorite cities updated',
      cities: [
        { city: 'Phoenix', state: 'AZ' },
        { city: 'Tempe', state: 'AZ' },
      ],
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useSetFavoriteCities(), {
      wrapper: createWrapper(),
    })

    const cities = [
      { city: 'Phoenix', state: 'AZ' },
      { city: 'Tempe', state: 'AZ' },
    ]

    await act(async () => {
      await result.current.mutateAsync(cities)
    })

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/auth/preferences/favorite-cities',
      {
        method: 'PUT',
        body: JSON.stringify({ cities }),
      }
    )
  })

  it('sends empty array to clear favorite cities', async () => {
    const mockResponse = {
      success: true,
      message: 'Favorite cities cleared',
      cities: [] as unknown[],
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useSetFavoriteCities(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      await result.current.mutateAsync([])
    })

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/auth/preferences/favorite-cities',
      {
        method: 'PUT',
        body: JSON.stringify({ cities: [] }),
      }
    )
  })

  it('saves a single city', async () => {
    const mockResponse = {
      success: true,
      message: 'Favorite cities updated',
      cities: [{ city: 'Chicago', state: 'IL' }],
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useSetFavoriteCities(), {
      wrapper: createWrapper(),
    })

    const cities = [{ city: 'Chicago', state: 'IL' }]

    await act(async () => {
      const data = await result.current.mutateAsync(cities)
      expect(data.cities).toHaveLength(1)
      expect(data.cities[0].city).toBe('Chicago')
      expect(data.cities[0].state).toBe('IL')
    })
  })

  it('handles validation errors', async () => {
    const error = new Error('Invalid city data')
    Object.assign(error, { status: 422 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useSetFavoriteCities(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      try {
        await result.current.mutateAsync([{ city: '', state: '' }])
      } catch (e) {
        expect((e as Error).message).toBe('Invalid city data')
      }
    })
  })

  it('isLoading reflects mutation state', async () => {
    let resolveMutation: (v: unknown) => void
    mockApiRequest.mockReturnValueOnce(
      new Promise(resolve => {
        resolveMutation = resolve
      })
    )

    const { result } = renderHook(() => useSetFavoriteCities(), {
      wrapper: createWrapper(),
    })

    expect(result.current.isPending).toBe(false)

    act(() => {
      result.current.mutate([{ city: 'Phoenix', state: 'AZ' }])
    })

    await waitFor(() => expect(result.current.isPending).toBe(true))

    await act(async () => {
      resolveMutation!({
        success: true,
        message: 'Updated',
        cities: [{ city: 'Phoenix', state: 'AZ' }],
      })
    })

    await waitFor(() => expect(result.current.isPending).toBe(false))
  })

  it('invalidates the profile query on success so favorites propagate', async () => {
    // The hook's whole reason for invalidation is to make the new favorites
    // visible everywhere profile.preferences.favorite_cities is read. If the
    // invalidation key drifts from ['auth','profile'], the user adds a city,
    // hits Save, and nothing in the header / city pills updates without a
    // page refresh.
    const queryClient = createTestQueryClient()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    mockApiRequest.mockResolvedValueOnce({
      success: true,
      message: 'Updated',
      cities: [{ city: 'Phoenix', state: 'AZ' }],
    })

    const { result } = renderHook(() => useSetFavoriteCities(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await act(async () => {
      await result.current.mutateAsync([{ city: 'Phoenix', state: 'AZ' }])
    })

    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ['auth', 'profile'],
    })
  })

  it('does NOT invalidate profile on mutation failure', async () => {
    // Symmetric to the success case: a failed save must leave the cache
    // untouched so the user keeps seeing whatever was previously persisted.
    const queryClient = createTestQueryClient()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    const error = new Error('Validation failed')
    Object.assign(error, { status: 422 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useSetFavoriteCities(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    let caught: unknown
    await act(async () => {
      try {
        await result.current.mutateAsync([{ city: '', state: '' }])
      } catch (e) {
        caught = e
      }
    })

    expect(caught).toBe(error)
    expect(invalidateSpy).not.toHaveBeenCalled()
  })

  it('surfaces server errors via the mutation error state', async () => {
    // Persistence problem must reach the calling component so the user
    // can see "save failed" rather than a phantom green confirmation.
    const error = new Error('Network unreachable')
    Object.assign(error, { status: 0 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useSetFavoriteCities(), {
      wrapper: createWrapper(),
    })

    act(() => {
      result.current.mutate([{ city: 'Phoenix', state: 'AZ' }])
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect(result.current.error).toBe(error)
    expect((result.current.error as Error).message).toBe('Network unreachable')
  })

  it('preserves the order of cities passed in the payload', async () => {
    // The backend stores order; the hook must not reorder client-side.
    mockApiRequest.mockResolvedValueOnce({
      success: true,
      message: 'Updated',
      cities: [],
    })

    const { result } = renderHook(() => useSetFavoriteCities(), {
      wrapper: createWrapper(),
    })

    const cities = [
      { city: 'Phoenix', state: 'AZ' },
      { city: 'New York', state: 'NY' },
      { city: 'Tokyo', state: 'Tokyo' },
    ]

    await act(async () => {
      await result.current.mutateAsync(cities)
    })

    const callBody = JSON.parse(
      (mockApiRequest.mock.calls[0][1] as { body: string }).body
    )
    expect(callBody.cities).toEqual(cities)
  })

  it('supports back-to-back saves without state leaking across mutations', async () => {
    // The Cities settings page allows the user to add a city, save, then
    // add another, save again. Each call must hit the API with its own
    // payload — earlier reset() logic regressions would re-send the prior
    // batch.
    mockApiRequest.mockResolvedValue({
      success: true,
      message: 'Updated',
      cities: [],
    })

    const { result } = renderHook(() => useSetFavoriteCities(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      await result.current.mutateAsync([{ city: 'A', state: 'AA' }])
    })
    await act(async () => {
      await result.current.mutateAsync([{ city: 'B', state: 'BB' }])
    })

    expect(mockApiRequest).toHaveBeenCalledTimes(2)
    const firstBody = JSON.parse(
      (mockApiRequest.mock.calls[0][1] as { body: string }).body
    )
    const secondBody = JSON.parse(
      (mockApiRequest.mock.calls[1][1] as { body: string }).body
    )
    expect(firstBody.cities).toEqual([{ city: 'A', state: 'AA' }])
    expect(secondBody.cities).toEqual([{ city: 'B', state: 'BB' }])
  })
})
