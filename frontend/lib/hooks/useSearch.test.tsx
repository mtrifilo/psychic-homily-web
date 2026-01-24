import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

// Create mocks
const mockApiRequest = vi.fn()

// Mock the api module
vi.mock('../api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    ARTISTS: {
      SEARCH: '/artists/search',
    },
    VENUES: {
      SEARCH: '/venues/search',
    },
  },
}))

// Mock queryClient module
vi.mock('../queryClient', () => ({
  queryKeys: {
    artists: {
      search: (query: string) => ['artists', 'search', query.toLowerCase()],
    },
    venues: {
      search: (query: string) => ['venues', 'search', query.toLowerCase()],
    },
  },
}))

// Mock use-debounce
vi.mock('use-debounce', () => ({
  useDebounce: (value: string) => [value], // No debounce in tests
}))

// Import hooks after mocks are set up
import { useArtistSearch } from './useArtistSearch'
import { useVenueSearch } from './useVenueSearch'

// Helper to create wrapper with query client
function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        gcTime: 0,
      },
    },
  })
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    )
  }
}

describe('useArtistSearch', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('does not fetch when query is empty', async () => {
    const { result } = renderHook(() => useArtistSearch({ query: '' }), {
      wrapper: createWrapper(),
    })

    // Should not be fetching
    expect(result.current.isFetching).toBe(false)
    expect(mockApiRequest).not.toHaveBeenCalled()
  })

  it('fetches artists when query has content', async () => {
    const mockResponse = {
      artists: [
        { id: 1, name: 'Test Artist', city: 'Phoenix', state: 'AZ' },
        { id: 2, name: 'Test Band', city: 'Tempe', state: 'AZ' },
      ],
      count: 2,
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(
      () => useArtistSearch({ query: 'Test' }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/artists/search?q=Test'
    )
    expect(result.current.data?.artists).toHaveLength(2)
    expect(result.current.data?.count).toBe(2)
  })

  it('URL encodes the search query', async () => {
    mockApiRequest.mockResolvedValueOnce({ artists: [], count: 0 })

    const { result } = renderHook(
      () => useArtistSearch({ query: 'Test & Band' }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/artists/search?q=Test%20%26%20Band'
    )
  })

  it('handles API errors', async () => {
    const error = new Error('Network error')
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(
      () => useArtistSearch({ query: 'Test' }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect(result.current.error).toBeDefined()
  })

  it('returns empty results when no artists match', async () => {
    mockApiRequest.mockResolvedValueOnce({ artists: [], count: 0 })

    const { result } = renderHook(
      () => useArtistSearch({ query: 'xyz123' }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(result.current.data?.artists).toEqual([])
    expect(result.current.data?.count).toBe(0)
  })
})

describe('useVenueSearch', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('does not fetch when query is empty', async () => {
    const { result } = renderHook(() => useVenueSearch({ query: '' }), {
      wrapper: createWrapper(),
    })

    expect(result.current.isFetching).toBe(false)
    expect(mockApiRequest).not.toHaveBeenCalled()
  })

  it('fetches venues when query has content', async () => {
    const mockResponse = {
      venues: [
        { id: 1, name: 'The Rebel Lounge', city: 'Phoenix', state: 'AZ', verified: true },
        { id: 2, name: 'Rebel Basement', city: 'Phoenix', state: 'AZ', verified: false },
      ],
      count: 2,
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(
      () => useVenueSearch({ query: 'Rebel' }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/venues/search?q=Rebel'
    )
    expect(result.current.data?.venues).toHaveLength(2)
    expect(result.current.data?.count).toBe(2)
  })

  it('URL encodes the search query', async () => {
    mockApiRequest.mockResolvedValueOnce({ venues: [], count: 0 })

    const { result } = renderHook(
      () => useVenueSearch({ query: 'Bar & Grill' }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/venues/search?q=Bar%20%26%20Grill'
    )
  })

  it('handles API errors', async () => {
    const error = new Error('Network error')
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(
      () => useVenueSearch({ query: 'Test' }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect(result.current.error).toBeDefined()
  })

  it('returns empty results when no venues match', async () => {
    mockApiRequest.mockResolvedValueOnce({ venues: [], count: 0 })

    const { result } = renderHook(
      () => useVenueSearch({ query: 'NonExistentVenue' }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(result.current.data?.venues).toEqual([])
    expect(result.current.data?.count).toBe(0)
  })
})
