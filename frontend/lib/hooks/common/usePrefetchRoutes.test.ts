import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook } from '@testing-library/react'
import { usePrefetchRoutes } from './usePrefetchRoutes'
// Deliberately NOT mocked: the show entries must be prefetched under the REAL
// first-screen constants, which is what makes them the entry an arriving
// `/shows` reads rather than a sibling that merely hashes the same today.
import {
  SHOW_CITIES_FIRST_SCREEN_KEY,
  UPCOMING_SHOWS_FIRST_SCREEN_KEY,
} from '@/features/shows/api'

// Mock TanStack Query
const mockPrefetchQuery = vi.fn()
vi.mock('@tanstack/react-query', () => ({
  useQueryClient: () => ({
    prefetchQuery: mockPrefetchQuery,
  }),
}))

// Mock API
vi.mock('../../api', () => ({
  apiRequest: vi.fn(),
  API_ENDPOINTS: {
    SHOWS: { UPCOMING: '/api/shows/upcoming', CITIES: '/api/shows/cities' },
    VENUES: { LIST: '/api/venues', CITIES: '/api/venues/cities' },
  },
}))

vi.mock('../../queryClient', () => ({
  queryKeys: {
    venues: {
      list: (params: unknown) => ['venues', 'list', params],
      cities: ['venues', 'cities'],
    },
  },
}))

describe('usePrefetchRoutes', () => {
  let originalRequestIdleCallback: typeof window.requestIdleCallback
  let originalCancelIdleCallback: typeof window.cancelIdleCallback

  beforeEach(() => {
    vi.clearAllMocks()
    originalRequestIdleCallback = window.requestIdleCallback
    originalCancelIdleCallback = window.cancelIdleCallback
  })

  afterEach(() => {
    window.requestIdleCallback = originalRequestIdleCallback
    window.cancelIdleCallback = originalCancelIdleCallback
  })

  it('uses window.requestIdleCallback when available', () => {
    const mockRIC = vi.fn((cb: IdleRequestCallback) => 42)
    const mockCIC = vi.fn()
    window.requestIdleCallback = mockRIC
    window.cancelIdleCallback = mockCIC

    const { unmount } = renderHook(() => usePrefetchRoutes())

    expect(mockRIC).toHaveBeenCalledWith(expect.any(Function))

    unmount()
    expect(mockCIC).toHaveBeenCalledWith(42)
  })

  it('falls back to setTimeout when requestIdleCallback is not available', () => {
    // Remove requestIdleCallback from window. `requestIdleCallback` is a
    // required field on the Window type, so we cast to `unknown` to
    // satisfy `delete`'s optional-operand constraint.
    delete (window as unknown as { requestIdleCallback?: unknown }).requestIdleCallback

    vi.useFakeTimers()

    const { unmount } = renderHook(() => usePrefetchRoutes())

    // The prefetch should not have been called yet
    expect(mockPrefetchQuery).not.toHaveBeenCalled()

    // Advance past the 1000ms setTimeout
    vi.advanceTimersByTime(1000)
    expect(mockPrefetchQuery).toHaveBeenCalled()

    unmount()
    vi.useRealTimers()
  })

  it('cleans up window.cancelIdleCallback on unmount, not bare cancelIdleCallback', () => {
    // This tests the bug fix: ensure we call window.cancelIdleCallback
    // not bare cancelIdleCallback which could cause ReferenceError
    const cancelSpy = vi.fn()
    window.requestIdleCallback = vi.fn(() => 99)
    window.cancelIdleCallback = cancelSpy

    const { unmount } = renderHook(() => usePrefetchRoutes())
    unmount()

    expect(cancelSpy).toHaveBeenCalledWith(99)
  })

  // The point of the prefetch, and the thing PSY-1678 made possible: the two
  // show entries must be warmed under the SAME keys `/shows` reads on arrival.
  // Keying them on the viewer's timezone (what this hook did before) warmed
  // entries that page never looked at, so the prefetch was a guaranteed miss.
  it('warms the show entries under the first-screen keys, with no timezone', () => {
    window.requestIdleCallback = vi.fn((cb: IdleRequestCallback) => {
      cb({ didTimeout: false, timeRemaining: () => 50 })
      return 1
    })
    window.cancelIdleCallback = vi.fn()

    const { unmount } = renderHook(() => usePrefetchRoutes())

    const keys = mockPrefetchQuery.mock.calls.map(c => c[0].queryKey)
    expect(keys).toContainEqual(UPCOMING_SHOWS_FIRST_SCREEN_KEY)
    expect(keys).toContainEqual(SHOW_CITIES_FIRST_SCREEN_KEY)

    unmount()
  })
})
