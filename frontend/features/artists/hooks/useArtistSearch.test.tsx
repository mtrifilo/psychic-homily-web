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
    expect(result.current.data?.artists).toHaveLength(2)
    expect(result.current.data?.count).toBe(2)
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

  it('handles API errors', async () => {
    const error = new Error('Server error')
    Object.assign(error, { status: 500 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(
      () => useArtistSearch({ query: 'test' }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect(result.current.error).toBeDefined()
  })

  it('returns empty results for no matches', async () => {
    mockApiRequest.mockResolvedValueOnce({ artists: [], count: 0 })

    const { result } = renderHook(
      () => useArtistSearch({ query: 'zzzznonexistent' }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(result.current.data?.artists).toHaveLength(0)
    expect(result.current.data?.count).toBe(0)
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

  it('returns artist details in response', async () => {
    const mockResponse = {
      artists: [
        {
          id: 5,
          slug: 'sonic-youth',
          name: 'Sonic Youth',
          city: 'New York',
          state: 'NY',
          bandcamp_embed_url: null,
          social: {
            bandcamp: 'https://sonicyouth.bandcamp.com',
            spotify: null,
            instagram: null,
          },
          created_at: '2024-01-01T00:00:00Z',
          updated_at: '2024-06-01T00:00:00Z',
        },
      ],
      count: 1,
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(
      () => useArtistSearch({ query: 'sonic' }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    const artist = result.current.data?.artists[0]
    expect(artist?.name).toBe('Sonic Youth')
    expect(artist?.slug).toBe('sonic-youth')
    expect(artist?.social.bandcamp).toBe('https://sonicyouth.bandcamp.com')
  })
})
