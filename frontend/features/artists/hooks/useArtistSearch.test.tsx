import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { createWrapper } from '@/test/utils'

// Create mocks
const mockApiRequest = vi.fn()

// Mock the api module
vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_BASE_URL: 'http://localhost:8080',
}))

// Mock the feature api module
vi.mock('@/features/artists/api', () => ({
  artistEndpoints: {
    SEARCH: '/artists/search',
  },
  artistQueryKeys: {
    search: (query: string) => ['artists', 'search', query.toLowerCase()],
  },
}))

// Mock use-debounce to make tests synchronous
vi.mock('use-debounce', () => ({
  useDebounce: (value: string, _delay: number) => [value],
}))

// Import hooks after mocks are set up
import { useArtistSearch } from './useArtistSearch'

describe('useArtistSearch', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches artists matching the search query', async () => {
    const mockResponse = {
      artists: [
        {
          id: 1,
          slug: 'test-artist',
          name: 'Test Artist',
          city: 'Phoenix',
          state: 'AZ',
          social: {},
        },
        {
          id: 2,
          slug: 'test-band',
          name: 'Test Band',
          city: 'Tempe',
          state: 'AZ',
          social: {},
        },
      ],
      count: 2,
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(
      () => useArtistSearch({ query: 'test' }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith('/artists/search?q=test')
  })

  it('does not fetch when query is empty', () => {
    const { result } = renderHook(
      () => useArtistSearch({ query: '' }),
      { wrapper: createWrapper() }
    )

    expect(mockApiRequest).not.toHaveBeenCalled()
    expect(result.current.fetchStatus).toBe('idle')
  })

  it('URL-encodes special characters in query', async () => {
    mockApiRequest.mockResolvedValueOnce({ artists: [], count: 0 })

    const { result } = renderHook(
      () => useArtistSearch({ query: 'AT&T Center' }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/artists/search?q=AT%26T%20Center'
    )
  })

  it('returns empty results for no matches', async () => {
    mockApiRequest.mockResolvedValueOnce({ artists: [], count: 0 })

    const { result } = renderHook(
      () => useArtistSearch({ query: 'zzzznonexistent' }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
  })

  it('accepts custom debounce delay', async () => {
    mockApiRequest.mockResolvedValueOnce({ artists: [], count: 0 })

    const { result } = renderHook(
      () => useArtistSearch({ query: 'test', debounceMs: 500 }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    // Since use-debounce is mocked, it resolves immediately regardless of delay
    expect(mockApiRequest).toHaveBeenCalledWith('/artists/search?q=test')
  })

  it('returns loading state while fetching', () => {
    // Make the request hang
    mockApiRequest.mockReturnValue(new Promise(() => {}))

    const { result } = renderHook(
      () => useArtistSearch({ query: 'loading' }),
      { wrapper: createWrapper() }
    )

    expect(result.current.isLoading).toBe(true)
    expect(result.current.data).toBeUndefined()
  })

})
