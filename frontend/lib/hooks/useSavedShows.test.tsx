import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

// Create mocks
const mockApiRequest = vi.fn()
const mockInvalidateSavedShows = vi.fn()

// Mock the api module
vi.mock('../api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    SAVED_SHOWS: {
      LIST: '/saved-shows',
      CHECK: (id: number | string) => `/saved-shows/${id}/check`,
      SAVE: (id: number) => `/saved-shows/${id}`,
      UNSAVE: (id: number) => `/saved-shows/${id}`,
    },
  },
}))

// Mock queryClient module
vi.mock('../queryClient', () => ({
  queryKeys: {
    savedShows: {
      list: () => ['savedShows', 'list'],
      check: (showId: string) => ['savedShows', 'check', showId],
    },
  },
  createInvalidateQueries: () => ({
    savedShows: mockInvalidateSavedShows,
  }),
}))

// Import hooks after mocks are set up
import {
  useSavedShows,
  useSavedShowIds,
  useSaveShow,
  useUnsaveShow,
  useSaveShowToggle,
} from './useSavedShows'

// Helper to create wrapper with query client
function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        gcTime: 0,
      },
      mutations: {
        retry: false,
      },
    },
  })
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    )
  }
}

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
    expect(result.current.data?.shows).toHaveLength(2)
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

    expect(result.current.data?.shows).toEqual([])
    expect(result.current.data?.total).toBe(0)
  })

  it('handles fetch errors', async () => {
    const error = new Error('Unauthorized')
    Object.assign(error, { status: 401 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useSavedShows(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect(result.current.error).toBeDefined()
  })
})

describe('useSavedShowIds', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('returns a Set of saved show IDs', async () => {
    mockApiRequest.mockResolvedValueOnce({
      shows: [
        { id: 1, title: 'Show 1' },
        { id: 5, title: 'Show 5' },
        { id: 10, title: 'Show 10' },
      ],
      total: 3,
    })

    const { result } = renderHook(() => useSavedShowIds(true), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isLoading).toBe(false))

    expect(result.current.savedIds.has(1)).toBe(true)
    expect(result.current.savedIds.has(5)).toBe(true)
    expect(result.current.savedIds.has(10)).toBe(true)
    expect(result.current.savedIds.has(99)).toBe(false)
  })

  it('returns empty set when not authenticated', () => {
    const { result } = renderHook(() => useSavedShowIds(false), {
      wrapper: createWrapper(),
    })

    expect(result.current.savedIds.size).toBe(0)
  })

  it('returns empty set when no shows saved', async () => {
    mockApiRequest.mockResolvedValueOnce({ shows: [], total: 0 })

    const { result } = renderHook(() => useSavedShowIds(true), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isLoading).toBe(false))

    expect(result.current.savedIds.size).toBe(0)
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

  it('returns isSaved false when show is not in saved list', async () => {
    // Saved shows list doesn't include show 700
    mockApiRequest.mockResolvedValueOnce({
      shows: [{ id: 1 }, { id: 2 }],
      total: 2,
    })

    const { result } = renderHook(() => useSaveShowToggle(700, true), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSaved).toBe(false))

    expect(result.current.isLoading).toBe(false)
  })

  it('returns isSaved true when show is in saved list', async () => {
    // Saved shows list includes show 800
    mockApiRequest.mockResolvedValueOnce({
      shows: [{ id: 800 }, { id: 2 }],
      total: 2,
    })

    const { result } = renderHook(() => useSaveShowToggle(800, true), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSaved).toBe(true))
  })

  it('toggle saves an unsaved show', async () => {
    // First call: saved shows list (doesn't include 900)
    mockApiRequest.mockResolvedValueOnce({
      shows: [{ id: 1 }],
      total: 1,
    })

    const { result } = renderHook(() => useSaveShowToggle(900, true), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSaved).toBe(false))

    // Second call: save the show
    mockApiRequest.mockResolvedValueOnce({ message: 'Saved' })

    await act(async () => {
      await result.current.toggle()
    })

    // Verify save was called
    expect(mockApiRequest).toHaveBeenCalledWith(
      '/saved-shows/900',
      expect.objectContaining({ method: 'POST' })
    )
  })

  it('toggle unsaves a saved show', async () => {
    // First call: saved shows list (includes 1000)
    mockApiRequest.mockResolvedValueOnce({
      shows: [{ id: 1000 }],
      total: 1,
    })

    const { result } = renderHook(() => useSaveShowToggle(1000, true), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSaved).toBe(true))

    // Second call: unsave the show
    mockApiRequest.mockResolvedValueOnce({ message: 'Unsaved' })

    await act(async () => {
      await result.current.toggle()
    })

    // Verify unsave was called
    expect(mockApiRequest).toHaveBeenCalledWith(
      '/saved-shows/1000',
      expect.objectContaining({ method: 'DELETE' })
    )
  })

  it('exposes error from save mutation', async () => {
    mockApiRequest.mockResolvedValueOnce({ shows: [], total: 0 })

    const { result } = renderHook(() => useSaveShowToggle(1200, true), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSaved).toBe(false))

    const error = new Error('Save failed')
    mockApiRequest.mockRejectedValueOnce(error)

    await act(async () => {
      try {
        await result.current.toggle()
      } catch {
        // Expected
      }
    })

    expect(result.current.error).toBeDefined()
  })

  it('exposes error from unsave mutation', async () => {
    mockApiRequest.mockResolvedValueOnce({
      shows: [{ id: 1300 }],
      total: 1,
    })

    const { result } = renderHook(() => useSaveShowToggle(1300, true), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSaved).toBe(true))

    const error = new Error('Unsave failed')
    mockApiRequest.mockRejectedValueOnce(error)

    await act(async () => {
      try {
        await result.current.toggle()
      } catch {
        // Expected
      }
    })

    expect(result.current.error).toBeDefined()
  })
})
