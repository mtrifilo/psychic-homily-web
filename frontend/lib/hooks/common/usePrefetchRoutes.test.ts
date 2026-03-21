import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook } from '@testing-library/react'
import { usePrefetchRoutes } from './usePrefetchRoutes'

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
    shows: {
      list: (params: unknown) => ['shows', 'list', params],
      cities: (tz: string) => ['shows', 'cities', tz],
    },
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

    const { unmount } = renderHook(() => usePrefetchRoutes('America/Phoenix'))

    expect(mockRIC).toHaveBeenCalledWith(expect.any(Function))

    unmount()
    expect(mockCIC).toHaveBeenCalledWith(42)
  })

  it('falls back to setTimeout when requestIdleCallback is not available', () => {
    // Remove requestIdleCallback from window
    // @ts-expect-error - intentionally removing for test
    delete window.requestIdleCallback

    vi.useFakeTimers()

    const { unmount } = renderHook(() => usePrefetchRoutes('America/Phoenix'))

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

    const { unmount } = renderHook(() => usePrefetchRoutes('America/Phoenix'))
    unmount()

    expect(cancelSpy).toHaveBeenCalledWith(99)
  })
})
