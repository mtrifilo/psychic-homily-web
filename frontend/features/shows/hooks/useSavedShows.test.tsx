import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createWrapper } from '@/test/utils'

// Create mocks
const mockApiRequest = vi.fn()
const mockInvalidateSavedShows = vi.fn()
const mockInvalidatePersonalCharts = vi.fn()

// Mock the api module
vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    SAVED_SHOWS: {
      LIST: '/saved-shows',
      SAVE: (id: number) => `/saved-shows/${id}`,
      UNSAVE: (id: number) => `/saved-shows/${id}`,
    },
    SAVE_COUNTS: {
      SHOW: (id: number) => `/shows/${id}/saves`,
      BATCH: '/shows/saves/batch',
    },
  },
}))

// Mock queryClient module
vi.mock('@/lib/queryClient', () => ({
  queryKeys: {
    savedShows: {
      listPrefix: (userId?: string | number) => [
        'savedShows',
        'list',
        userId ?? null,
      ],
      list: (
        userId?: string,
        limit: number = 50,
        offset: number = 0,
        timeFilter?: 'upcoming' | 'past'
      ) => [
        'savedShows',
        'list',
        userId ?? null,
        { limit, offset, timeFilter },
      ],
      infiniteList: (
        userId: number | undefined,
        timeFilter: 'upcoming' | 'past'
      ) => ['savedShows', 'infiniteList', userId ?? null, timeFilter],
      infiniteListPrefix: (userId: number | undefined) => [
        'savedShows',
        'infiniteList',
        userId ?? null,
      ],
      count: (
        showId: number,
        isAuthenticated: boolean,
        userId?: string | number
      ) => ['savedShows', 'count', isAuthenticated, userId ?? null, showId],
      all: ['savedShows'],
      countBatchPrefix: (userId?: string | number) => [
        'savedShows',
        'countBatch',
        true,
        userId ?? null,
      ],
      countBatch: (
        showIds: number[],
        isAuthenticated: boolean,
        userId?: string | number
      ) => [
        'savedShows',
        'countBatch',
        isAuthenticated,
        userId ?? null,
        showIds,
      ],
    },
  },
  createInvalidateQueries: () => ({
    savedShows: mockInvalidateSavedShows,
    personalCharts: mockInvalidatePersonalCharts,
  }),
}))

// Import hooks after mocks are set up (queryKeys resolves to the mock above)
import { queryKeys } from '@/lib/queryClient'
import {
  useInfiniteSavedShows,
  useSavedShows,
  useSaveShow,
  useUnsaveShow,
  useSaveShowToggle,
} from './useSavedShows'

describe('useSavedShows', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('loads a four-row first page and advances through the remaining partition', async () => {
    mockApiRequest
      .mockResolvedValueOnce({
        shows: [{ id: 1 }, { id: 2 }, { id: 3 }, { id: 4 }],
        total: 6,
        limit: 4,
        offset: 0,
      })
      .mockResolvedValueOnce({
        shows: [{ id: 5 }, { id: 6 }],
        total: 6,
        limit: 100,
        offset: 4,
      })

    const { result } = renderHook(() => useInfiniteSavedShows('upcoming', 1), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenNthCalledWith(
      1,
      '/saved-shows?limit=4&offset=0&time_filter=upcoming',
      { method: 'GET' }
    )

    let fetchResult: Awaited<ReturnType<typeof result.current.fetchNextPage>>
    await act(async () => {
      fetchResult = await result.current.fetchNextPage()
    })

    expect(mockApiRequest).toHaveBeenNthCalledWith(
      2,
      '/saved-shows?limit=100&offset=4&time_filter=upcoming',
      { method: 'GET' }
    )
    expect(fetchResult!.data?.pages.flatMap(page => page.shows)).toHaveLength(6)
    expect(fetchResult!.hasNextPage).toBe(false)
  })

  it('isolates infinite saved-show caches by user identity', async () => {
    mockApiRequest
      .mockResolvedValueOnce({
        shows: [{ id: 1, title: 'Alice show' }],
        total: 1,
        limit: 4,
        offset: 0,
      })
      .mockResolvedValueOnce({
        shows: [{ id: 2, title: 'Bob show' }],
        total: 1,
        limit: 4,
        offset: 0,
      })
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    )

    const alice = renderHook(() => useInfiniteSavedShows('upcoming', 1), {
      wrapper,
    })
    await waitFor(() => expect(alice.result.current.isSuccess).toBe(true))
    expect(alice.result.current.data?.pages[0].shows[0].title).toBe(
      'Alice show'
    )
    alice.unmount()

    const bob = renderHook(() => useInfiniteSavedShows('upcoming', 2), {
      wrapper,
    })
    await waitFor(() => expect(bob.result.current.isSuccess).toBe(true))
    expect(bob.result.current.data?.pages[0].shows[0].title).toBe('Bob show')
    expect(mockApiRequest).toHaveBeenCalledTimes(2)
  })

  it('patches infinite Library pages and resumes at the shifted offset', async () => {
    mockApiRequest
      .mockResolvedValueOnce({ success: true })
      .mockResolvedValueOnce({
        shows: [{ id: 105 }],
        total: 104,
        limit: 100,
        offset: 103,
      })
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    const queryKey = queryKeys.savedShows.infiniteList(1, 'upcoming')
    queryClient.setQueryData(queryKey, {
      pages: [
        {
          shows: [{ id: 1 }, { id: 2 }, { id: 3 }, { id: 4 }],
          total: 105,
          limit: 4,
          offset: 0,
        },
        {
          shows: Array.from({ length: 100 }, (_, index) => ({ id: index + 5 })),
          total: 105,
          limit: 100,
          offset: 4,
        },
      ],
      pageParams: [
        { limit: 4, offset: 0 },
        { limit: 100, offset: 4 },
      ],
    })
    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    )

    const { result } = renderHook(
      () => useUnsaveShow({ syncMode: 'patch-infinite', userId: 1 }),
      { wrapper }
    )
    await act(async () => {
      await result.current.mutateAsync(1)
    })

    const cached = queryClient.getQueryData<{
      pages: Array<{ shows: Array<{ id: number }>; total: number }>
    }>(queryKey)
    expect(cached?.pages[0].shows).toEqual([{ id: 2 }, { id: 3 }, { id: 4 }])
    expect(cached?.pages[0].total).toBe(104)
    expect(cached?.pages[1].total).toBe(104)
    expect(mockInvalidateSavedShows).not.toHaveBeenCalled()

    const infinite = renderHook(() => useInfiniteSavedShows('upcoming', 1), {
      wrapper,
    })
    await act(async () => {
      await infinite.result.current.fetchNextPage()
    })
    expect(mockApiRequest).toHaveBeenNthCalledWith(
      2,
      '/saved-shows?limit=100&offset=103&time_filter=upcoming',
      { method: 'GET' }
    )
  })

  it('fetches saved shows list', async () => {
    const mockResponse = {
      shows: [
        { id: 1, title: 'Show 1', saved_at: '2025-01-15T00:00:00Z' },
        { id: 2, title: 'Show 2', saved_at: '2025-01-14T00:00:00Z' },
      ],
      total: 2,
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useSavedShows(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/saved-shows?limit=50&offset=0',
      expect.objectContaining({
        method: 'GET',
      })
    )
  })

  it('supports pagination with limit and offset', async () => {
    mockApiRequest.mockResolvedValueOnce({ shows: [], total: 0 })

    const { result } = renderHook(
      () => useSavedShows({ limit: 20, offset: 40 }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/saved-shows?limit=20&offset=40',
      expect.objectContaining({
        method: 'GET',
      })
    )
  })

  it.each(['upcoming', 'past'] as const)(
    'includes the %s time filter in the request and query key',
    async timeFilter => {
      mockApiRequest.mockResolvedValueOnce({
        shows: [],
        total: 0,
        limit: 100,
        offset: 0,
      })

      const queryClient = new QueryClient({
        defaultOptions: { queries: { retry: false } },
      })
      const wrapper = ({ children }: { children: React.ReactNode }) => (
        <QueryClientProvider client={queryClient}>
          {children}
        </QueryClientProvider>
      )

      const { result } = renderHook(
        () => useSavedShows({ limit: 100, timeFilter }),
        { wrapper }
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        `/saved-shows?limit=100&offset=0&time_filter=${timeFilter}`,
        expect.objectContaining({ method: 'GET' })
      )
      expect(
        queryClient.getQueryData(
          queryKeys.savedShows.list(undefined, 100, 0, timeFilter)
        )
      ).toEqual(expect.objectContaining({ total: 0 }))
    }
  )

  it('handles empty saved shows list', async () => {
    mockApiRequest.mockResolvedValueOnce({ shows: [], total: 0 })

    const { result } = renderHook(() => useSavedShows(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
  })
})

describe('useSaveShow', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockInvalidateSavedShows.mockReset()
    mockInvalidatePersonalCharts.mockReset()
  })

  it('saves a show with correct endpoint', async () => {
    mockApiRequest.mockResolvedValueOnce({
      message: 'Show saved',
      saved_at: '2025-01-15T12:00:00Z',
    })

    const { result } = renderHook(() => useSaveShow(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(100)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/saved-shows/100',
      expect.objectContaining({
        method: 'POST',
      })
    )
  })

  it('invalidates saved shows on success', async () => {
    mockApiRequest.mockResolvedValueOnce({ message: 'Saved' })

    const { result } = renderHook(() => useSaveShow(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(200)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockInvalidateSavedShows).toHaveBeenCalled()
    expect(mockInvalidatePersonalCharts).toHaveBeenCalled()
  })

  it('handles save errors', async () => {
    const error = new Error('Already saved')
    Object.assign(error, { status: 409 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useSaveShow(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(300)
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect(result.current.error).toBeDefined()
  })
})

describe('useUnsaveShow', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockInvalidateSavedShows.mockReset()
    mockInvalidatePersonalCharts.mockReset()
  })

  it('unsaves a show with correct endpoint', async () => {
    mockApiRequest.mockResolvedValueOnce({
      message: 'Show removed from saved list',
    })

    const { result } = renderHook(() => useUnsaveShow(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(400)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/saved-shows/400',
      expect.objectContaining({
        method: 'DELETE',
      })
    )
  })

  it('invalidates saved shows on success', async () => {
    mockApiRequest.mockResolvedValueOnce({ message: 'Unsaved' })

    const { result } = renderHook(() => useUnsaveShow(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(500)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockInvalidateSavedShows).toHaveBeenCalled()
    expect(mockInvalidatePersonalCharts).toHaveBeenCalled()
  })

  it('handles unsave errors', async () => {
    const error = new Error('Show not in saved list')
    Object.assign(error, { status: 404 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useUnsaveShow(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(600)
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect(result.current.error).toBeDefined()
  })
})

describe('useSaveShowToggle', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockInvalidateSavedShows.mockReset()
  })

  it('toggle saves an unsaved show', async () => {
    mockApiRequest.mockResolvedValueOnce({ message: 'Saved' })

    const { result } = renderHook(() => useSaveShowToggle(900, false), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      await result.current.toggle()
    })

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/saved-shows/900',
      expect.objectContaining({ method: 'POST' })
    )
  })

  it('toggle unsaves a saved show', async () => {
    mockApiRequest.mockResolvedValueOnce({ message: 'Unsaved' })

    const { result } = renderHook(() => useSaveShowToggle(1000, true), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      await result.current.toggle()
    })

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/saved-shows/1000',
      expect.objectContaining({ method: 'DELETE' })
    )
  })

  // Optimistic-update guard: assert the CACHE is patched, not just that a
  // request fired. A render-only stub of `toggle` would pass a request-only
  // assertion even with the optimistic wiring deleted.

  it('optimistically bumps the cached count before the mutation resolves', async () => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    const countKey = ['savedShows', 'count', true, null, 1400]
    queryClient.setQueryData(countKey, {
      show_id: 1400,
      save_count: 4,
      is_saved: false,
    })

    // Hold the mutation open so we can observe the pre-resolution cache state.
    let resolveSave: (v: unknown) => void = () => {}
    mockApiRequest.mockImplementationOnce(
      () =>
        new Promise(res => {
          resolveSave = res
        })
    )

    const { result } = renderHook(() => useSaveShowToggle(1400, false), {
      wrapper: ({ children }: { children: React.ReactNode }) => (
        <QueryClientProvider client={queryClient}>
          {children}
        </QueryClientProvider>
      ),
    })

    let toggling: Promise<void>
    await act(async () => {
      toggling = result.current.toggle()
      await Promise.resolve()
    })

    // Optimistic: count +1 and is_saved flipped BEFORE the server replied.
    expect(queryClient.getQueryData(countKey)).toEqual({
      show_id: 1400,
      save_count: 5,
      is_saved: true,
    })

    await act(async () => {
      resolveSave({ message: 'Saved' })
      await toggling!
    })
  })

  it('rolls the cached count back when the mutation fails', async () => {
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    })
    const countKey = ['savedShows', 'count', true, null, 1500]
    queryClient.setQueryData(countKey, {
      show_id: 1500,
      save_count: 4,
      is_saved: false,
    })

    mockApiRequest.mockRejectedValueOnce(new Error('Network error'))

    const { result } = renderHook(() => useSaveShowToggle(1500, false), {
      wrapper: ({ children }: { children: React.ReactNode }) => (
        <QueryClientProvider client={queryClient}>
          {children}
        </QueryClientProvider>
      ),
    })

    await act(async () => {
      try {
        await result.current.toggle()
      } catch {
        // expected
      }
    })

    // Rollback restores the pre-toggle count and saved state.
    expect(queryClient.getQueryData(countKey)).toEqual({
      show_id: 1500,
      save_count: 4,
      is_saved: false,
    })
  })

  it('never drives the cached count negative', async () => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    const countKey = ['savedShows', 'count', true, null, 1600]
    // Server said 0 saves but our local state thinks we saved it — unsaving
    // must clamp at 0 rather than render "-1".
    queryClient.setQueryData(countKey, {
      show_id: 1600,
      save_count: 0,
      is_saved: true,
    })
    mockApiRequest.mockResolvedValueOnce({ message: 'Unsaved' })

    const { result } = renderHook(() => useSaveShowToggle(1600, true), {
      wrapper: ({ children }: { children: React.ReactNode }) => (
        <QueryClientProvider client={queryClient}>
          {children}
        </QueryClientProvider>
      ),
    })

    await act(async () => {
      await result.current.toggle()
    })

    expect(
      (queryClient.getQueryData(countKey) as { save_count: number }).save_count
    ).toBe(0)
  })

  // Regression: rollback must restore the SNAPSHOT, not re-invert the delta.
  // Re-inverting against an already-clamped 0 resurrects a phantom +1.
  it('rollback after a clamped unsave does not resurrect a phantom count', async () => {
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    })
    const countKey = ['savedShows', 'count', true, null, 1700]
    queryClient.setQueryData(countKey, {
      show_id: 1700,
      save_count: 0,
      is_saved: true,
    })
    mockApiRequest.mockRejectedValueOnce(new Error('Network error'))

    const { result } = renderHook(() => useSaveShowToggle(1700, true), {
      wrapper: ({ children }: { children: React.ReactNode }) => (
        <QueryClientProvider client={queryClient}>
          {children}
        </QueryClientProvider>
      ),
    })

    await act(async () => {
      try {
        await result.current.toggle()
      } catch {
        // expected
      }
    })

    expect(queryClient.getQueryData(countKey)).toEqual({
      show_id: 1700,
      save_count: 0,
      is_saved: true,
    })
  })

  // A cached batch that doesn't contain this show must be left alone entirely —
  // in both the optimistic apply and the rollback (symmetric `entry` guards).
  it('leaves unrelated batch caches untouched', async () => {
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    })
    const otherBatchKey = queryKeys.savedShows.countBatch([2000, 2001], true)
    const untouched = {
      '2000': { save_count: 1, is_saved: false },
      '2001': { save_count: 2, is_saved: true },
    }
    queryClient.setQueryData(otherBatchKey, untouched)
    mockApiRequest.mockRejectedValueOnce(new Error('Network error'))

    // Toggle a show that appears in NO cached batch.
    const { result } = renderHook(() => useSaveShowToggle(9999, false), {
      wrapper: ({ children }: { children: React.ReactNode }) => (
        <QueryClientProvider client={queryClient}>
          {children}
        </QueryClientProvider>
      ),
    })

    await act(async () => {
      try {
        await result.current.toggle()
      } catch {
        // expected
      }
    })

    expect(queryClient.getQueryData(otherBatchKey)).toEqual(untouched)
  })

  // Regression: a batch cache entry is SHARED across every show in the list.
  // Rolling back a failed toggle by restoring the whole snapshot would erase a
  // sibling show's save that succeeded while our request was in flight.
  it('rollback does not clobber a sibling show in the same batch', async () => {
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    })
    const batchKey = queryKeys.savedShows.countBatch([1900, 1901], true)
    queryClient.setQueryData(batchKey, {
      '1900': { save_count: 4, is_saved: false },
      '1901': { save_count: 9, is_saved: false },
    })

    // Show 1900's save hangs, then fails.
    let rejectSave: (e: unknown) => void = () => {}
    mockApiRequest.mockImplementationOnce(
      () =>
        new Promise((_res, rej) => {
          rejectSave = rej
        })
    )

    const { result } = renderHook(() => useSaveShowToggle(1900, false), {
      wrapper: ({ children }: { children: React.ReactNode }) => (
        <QueryClientProvider client={queryClient}>
          {children}
        </QueryClientProvider>
      ),
    })

    let failing: Promise<void>
    await act(async () => {
      failing = result.current.toggle().catch(() => {})
      await Promise.resolve()
    })

    // Meanwhile a sibling show in the SAME batch is saved successfully.
    queryClient.setQueryData(
      batchKey,
      (prev: Record<string, { save_count: number; is_saved: boolean }>) => ({
        ...prev,
        '1901': { save_count: 10, is_saved: true },
      })
    )

    await act(async () => {
      rejectSave(new Error('Network error'))
      await failing!
    })

    const after = queryClient.getQueryData(batchKey) as Record<
      string,
      { save_count: number; is_saved: boolean }
    >
    // 1900 rolls back to its pre-toggle value...
    expect(after['1900']).toEqual({ save_count: 4, is_saved: false })
    // ...and 1901's successful save survives.
    expect(after['1901']).toEqual({ save_count: 10, is_saved: true })
  })

  // The shows list (the highest-traffic surface) reads save data from the BATCH
  // cache, so the batch branch of the optimistic update needs its own guard.
  it('optimistically patches the cached BATCH entry, keyed via the real helper', async () => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    const batchKey = queryKeys.savedShows.countBatch([1800, 1801], true)
    queryClient.setQueryData(batchKey, {
      '1800': { save_count: 4, is_saved: false },
      '1801': { save_count: 9, is_saved: false },
    })

    let resolveSave: (v: unknown) => void = () => {}
    mockApiRequest.mockImplementationOnce(
      () =>
        new Promise(res => {
          resolveSave = res
        })
    )

    const { result } = renderHook(() => useSaveShowToggle(1800, false), {
      wrapper: ({ children }: { children: React.ReactNode }) => (
        <QueryClientProvider client={queryClient}>
          {children}
        </QueryClientProvider>
      ),
    })

    let toggling: Promise<void>
    await act(async () => {
      toggling = result.current.toggle()
      await Promise.resolve()
    })

    // Only the toggled show moves; its sibling in the same batch is untouched.
    expect(queryClient.getQueryData(batchKey)).toEqual({
      '1800': { save_count: 5, is_saved: true },
      '1801': { save_count: 9, is_saved: false },
    })

    await act(async () => {
      resolveSave({ message: 'Saved' })
      await toggling!
    })
  })

  it('exposes error from save mutation', async () => {
    mockApiRequest.mockRejectedValueOnce(new Error('Save failed'))

    const { result } = renderHook(() => useSaveShowToggle(1200, false), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      try {
        await result.current.toggle()
      } catch {
        // Expected
      }
    })

    await waitFor(() => expect(result.current.error).toBeDefined())
  })

  it('exposes error from unsave mutation', async () => {
    mockApiRequest.mockRejectedValueOnce(new Error('Unsave failed'))

    const { result } = renderHook(() => useSaveShowToggle(1300, true), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      try {
        await result.current.toggle()
      } catch {
        // Expected
      }
    })

    await waitFor(() => expect(result.current.error).toBeDefined())
  })
})
