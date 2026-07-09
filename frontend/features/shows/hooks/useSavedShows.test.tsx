import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createWrapper } from '@/test/utils'

// Create mocks
const mockApiRequest = vi.fn()
const mockInvalidateSavedShows = vi.fn()

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
      list: () => ['savedShows', 'list'],
      count: (showId: number, isAuthenticated: boolean) => [
        'savedShows',
        'count',
        isAuthenticated,
        showId,
      ],
      all: ['savedShows'],
      countBatchPrefix: ['savedShows', 'countBatch'],
      countBatch: (showIds: number[], isAuthenticated: boolean) => [
        'savedShows',
        'countBatch',
        isAuthenticated,
        showIds,
      ],
    },
  },
  createInvalidateQueries: () => ({
    savedShows: mockInvalidateSavedShows,
  }),
}))

// Import hooks after mocks are set up (queryKeys resolves to the mock above)
import { queryKeys } from '@/lib/queryClient'
import {
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
    const countKey = ['savedShows', 'count', true, 1400]
    queryClient.setQueryData(countKey, {
      show_id: 1400,
      save_count: 4,
      is_saved: false,
    })

    // Hold the mutation open so we can observe the pre-resolution cache state.
    let resolveSave: (v: unknown) => void = () => {}
    mockApiRequest.mockImplementationOnce(
      () => new Promise((res) => { resolveSave = res })
    )

    const { result } = renderHook(() => useSaveShowToggle(1400, false), {
      wrapper: ({ children }: { children: React.ReactNode }) => (
        <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
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
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    })
    const countKey = ['savedShows', 'count', true, 1500]
    queryClient.setQueryData(countKey, {
      show_id: 1500,
      save_count: 4,
      is_saved: false,
    })

    mockApiRequest.mockRejectedValueOnce(new Error('Network error'))

    const { result } = renderHook(() => useSaveShowToggle(1500, false), {
      wrapper: ({ children }: { children: React.ReactNode }) => (
        <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
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
    const countKey = ['savedShows', 'count', true, 1600]
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
        <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
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
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    })
    const countKey = ['savedShows', 'count', true, 1700]
    queryClient.setQueryData(countKey, {
      show_id: 1700,
      save_count: 0,
      is_saved: true,
    })
    mockApiRequest.mockRejectedValueOnce(new Error('Network error'))

    const { result } = renderHook(() => useSaveShowToggle(1700, true), {
      wrapper: ({ children }: { children: React.ReactNode }) => (
        <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
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
      () => new Promise((res) => { resolveSave = res })
    )

    const { result } = renderHook(() => useSaveShowToggle(1800, false), {
      wrapper: ({ children }: { children: React.ReactNode }) => (
        <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
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
