import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { createWrapper } from '@/test/utils'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_BASE_URL: 'http://localhost:8080',
}))

vi.mock('@/features/labels/api', () => ({
  labelEndpoints: {
    SEARCH: '/labels/search',
  },
  labelQueryKeys: {
    search: (query: string) => ['labels', 'search', query.toLowerCase()],
  },
}))

// Spy wrapper around the real use-debounce. By default it passes the value
// straight through (synchronous), which lets us assert URL/encoding/enabled
// behavior without timers, while still recording the (value, delay) args the
// hook hands it — that's how we verify the debounce is actually wired and what
// delay it uses (see the "debounce wiring" describe below).
const debounceSpy = vi.fn((value: string, _delay?: number) => [value] as const)
vi.mock('use-debounce', () => ({
  useDebounce: (value: string, delay?: number) => debounceSpy(value, delay),
}))

import { useLabelSearch } from './useLabelSearch'

describe('useLabelSearch', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    debounceSpy.mockImplementation((value: string) => [value] as const)
  })

  describe('query behavior', () => {
    it('fetches labels matching the search query', async () => {
      mockApiRequest.mockResolvedValueOnce({
        labels: [{ id: 1, slug: 'sub-pop', name: 'Sub Pop' }],
        count: 1,
      })

      const { result } = renderHook(() => useLabelSearch({ query: 'sub' }), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))
      expect(mockApiRequest).toHaveBeenCalledWith('/labels/search?q=sub')
    })

    it('does not fetch when the query is empty', () => {
      const { result } = renderHook(() => useLabelSearch({ query: '' }), {
        wrapper: createWrapper(),
      })

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result.current.fetchStatus).toBe('idle')
    })

    it('URL-encodes special characters in the query', async () => {
      mockApiRequest.mockResolvedValueOnce({ labels: [], count: 0 })

      const { result } = renderHook(
        () => useLabelSearch({ query: 'rough & ready' }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))
      expect(mockApiRequest).toHaveBeenCalledWith(
        '/labels/search?q=rough%20%26%20ready'
      )
    })

    it('returns the loading state while the request is in flight', () => {
      mockApiRequest.mockReturnValue(new Promise(() => {}))

      const { result } = renderHook(() => useLabelSearch({ query: 'loading' }), {
        wrapper: createWrapper(),
      })

      expect(result.current.isLoading).toBe(true)
      expect(result.current.data).toBeUndefined()
    })

    it('surfaces an error when the request rejects', async () => {
      mockApiRequest.mockRejectedValueOnce(new Error('Search failed'))

      const { result } = renderHook(() => useLabelSearch({ query: 'boom' }), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
      expect((result.current.error as Error).message).toBe('Search failed')
    })
  })

  // The factory (lib/hooks/factories.ts createSearchHook) routes the input
  // query through use-debounce BEFORE it reaches the query key + request. These
  // tests confirm that wiring and the current delay value without depending on
  // fragile fake-timer + React-state flush semantics: we assert the (value,
  // delay) pair handed to use-debounce, and that the DEBOUNCED value (not the
  // raw input) is what drives the request.
  describe('debounce wiring', () => {
    it('passes the raw query through use-debounce with the default 50ms delay', () => {
      mockApiRequest.mockResolvedValue({ labels: [], count: 0 })
      renderHook(() => useLabelSearch({ query: 'sub' }), {
        wrapper: createWrapper(),
      })

      expect(debounceSpy).toHaveBeenCalledWith('sub', 50)
    })

    it('passes a caller-supplied debounceMs through to use-debounce', () => {
      mockApiRequest.mockResolvedValue({ labels: [], count: 0 })
      renderHook(() => useLabelSearch({ query: 'sub', debounceMs: 300 }), {
        wrapper: createWrapper(),
      })

      expect(debounceSpy).toHaveBeenCalledWith('sub', 300)
    })

    it('drives the request off the DEBOUNCED value, not the raw input', async () => {
      // Force the debounce to lag: return a stale value regardless of input.
      debounceSpy.mockReturnValue(['stale'] as const)
      mockApiRequest.mockResolvedValueOnce({ labels: [], count: 0 })

      const { result } = renderHook(
        () => useLabelSearch({ query: 'fresh-input' }),
        { wrapper: createWrapper() }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))
      // Proves the hook awaits the debounced value before firing.
      expect(mockApiRequest).toHaveBeenCalledWith('/labels/search?q=stale')
      expect(mockApiRequest).not.toHaveBeenCalledWith('/labels/search?q=fresh-input')
    })

    it('keeps the query disabled while the debounced value is still empty', () => {
      // Raw input is non-empty, but the debounced value has not caught up yet.
      debounceSpy.mockReturnValue([''] as const)

      const { result } = renderHook(
        () => useLabelSearch({ query: 'typing' }),
        { wrapper: createWrapper() }
      )

      expect(mockApiRequest).not.toHaveBeenCalled()
      expect(result.current.fetchStatus).toBe('idle')
    })
  })
})
