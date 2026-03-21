import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { createWrapper } from '@/test/utils'

// Create mocks
const mockApiRequest = vi.fn()

// Mock the api module
vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    VENUES: {
      SEARCH: '/venues/search',
    },
  },
  API_BASE_URL: 'http://localhost:8080',
}))

// Mock queryClient module
vi.mock('@/lib/queryClient', () => ({
  queryKeys: {
    venues: {
      search: (query: string) => ['venues', 'search', query.toLowerCase()],
    },
  },
}))

// Mock use-debounce to make tests synchronous
vi.mock('use-debounce', () => ({
  useDebounce: (value: string, _delay: number) => [value],
}))

// Import hooks after mocks are set up
import { useVenueSearch } from './useVenueSearch'

describe('useVenueSearch', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches venues matching the search query', async () => {
    const mockResponse = {
      venues: [
        { id: 1, slug: 'the-rebel-lounge', name: 'The Rebel Lounge', city: 'Phoenix', state: 'AZ' },
        { id: 2, slug: 'rebel-bar', name: 'Rebel Bar', city: 'Tempe', state: 'AZ' },
      ],
      count: 2,
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(
      () => useVenueSearch({ query: 'rebel' }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith('/venues/search?q=rebel')
    expect(result.current.data?.venues).toHaveLength(2)
    expect(result.current.data?.count).toBe(2)
  })

  it('does not fetch when query is empty', () => {
    const { result } = renderHook(
      () => useVenueSearch({ query: '' }),
      { wrapper: createWrapper() }
    )

    expect(mockApiRequest).not.toHaveBeenCalled()
    expect(result.current.fetchStatus).toBe('idle')
  })

  it('URL-encodes special characters in query', async () => {
    mockApiRequest.mockResolvedValueOnce({ venues: [], count: 0 })

    const { result } = renderHook(
      () => useVenueSearch({ query: 'bar & grill' }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/venues/search?q=bar%20%26%20grill'
    )
  })

  it('handles API errors', async () => {
    const error = new Error('Server error')
    Object.assign(error, { status: 500 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(
      () => useVenueSearch({ query: 'test' }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect(result.current.error).toBeDefined()
  })

  it('returns empty results for no matches', async () => {
    mockApiRequest.mockResolvedValueOnce({ venues: [], count: 0 })

    const { result } = renderHook(
      () => useVenueSearch({ query: 'nonexistent' }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(result.current.data?.venues).toHaveLength(0)
    expect(result.current.data?.count).toBe(0)
  })

  it('accepts custom debounce delay', async () => {
    mockApiRequest.mockResolvedValueOnce({ venues: [], count: 0 })

    const { result } = renderHook(
      () => useVenueSearch({ query: 'test', debounceMs: 300 }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    // Since use-debounce is mocked, it resolves immediately regardless of delay
    expect(mockApiRequest).toHaveBeenCalledWith('/venues/search?q=test')
  })

  it('returns loading state while fetching', () => {
    // Make the request hang
    mockApiRequest.mockReturnValue(new Promise(() => {}))

    const { result } = renderHook(
      () => useVenueSearch({ query: 'loading' }),
      { wrapper: createWrapper() }
    )

    expect(result.current.isLoading).toBe(true)
    expect(result.current.data).toBeUndefined()
  })
})
