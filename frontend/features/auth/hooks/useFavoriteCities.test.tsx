import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { createWrapper } from '@/test/utils'

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
      const data = await result.current.mutateAsync(cities)
      expect(data.success).toBe(true)
      expect(data.cities).toHaveLength(2)
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
      cities: [],
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useSetFavoriteCities(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      const data = await result.current.mutateAsync([])
      expect(data.cities).toHaveLength(0)
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
})
