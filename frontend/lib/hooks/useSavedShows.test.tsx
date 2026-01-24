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
  useIsShowSaved,
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

describe('useIsShowSaved', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('checks if a show is saved', async () => {
    mockApiRequest.mockResolvedValueOnce({ is_saved: true })

    const { result } = renderHook(() => useIsShowSaved(123), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/saved-shows/123/check',
      expect.objectContaining({
        method: 'GET',
      })
    )
    expect(result.current.data?.is_saved).toBe(true)
  })

  it('returns false for unsaved shows', async () => {
    mockApiRequest.mockResolvedValueOnce({ is_saved: false })

    const { result } = renderHook(() => useIsShowSaved(456), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(result.current.data?.is_saved).toBe(false)
  })

  it('does not fetch when showId is null', async () => {
    const { result } = renderHook(() => useIsShowSaved(null), {
      wrapper: createWrapper(),
    })

    expect(result.current.isFetching).toBe(false)
    expect(mockApiRequest).not.toHaveBeenCalled()
  })

  it('accepts string showId', async () => {
    mockApiRequest.mockResolvedValueOnce({ is_saved: true })

    const { result } = renderHook(() => useIsShowSaved('789'), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/saved-shows/789/check',
      expect.objectContaining({
        method: 'GET',
      })
    )
  })

  it('handles check errors', async () => {
    const error = new Error('Show not found')
    Object.assign(error, { status: 404 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useIsShowSaved(999), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect(result.current.error).toBeDefined()
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

  it('returns isSaved false when show is not saved', async () => {
    mockApiRequest.mockResolvedValueOnce({ is_saved: false })

    const { result } = renderHook(() => useSaveShowToggle(700), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSaved).toBe(false))

    expect(result.current.isLoading).toBe(false)
  })

  it('returns isSaved true when show is saved', async () => {
    mockApiRequest.mockResolvedValueOnce({ is_saved: true })

    const { result } = renderHook(() => useSaveShowToggle(800), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSaved).toBe(true))
  })

  it('toggle saves an unsaved show', async () => {
    // First call: check if saved (false)
    mockApiRequest.mockResolvedValueOnce({ is_saved: false })
    // Second call: save the show
    mockApiRequest.mockResolvedValueOnce({ message: 'Saved' })

    const { result } = renderHook(() => useSaveShowToggle(900), {
      wrapper: createWrapper(),
    })

    // Wait for initial check to complete
    await waitFor(() => expect(result.current.isSaved).toBe(false))

    // Toggle (should save)
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
    // First call: check if saved (true)
    mockApiRequest.mockResolvedValueOnce({ is_saved: true })
    // Second call: unsave the show
    mockApiRequest.mockResolvedValueOnce({ message: 'Unsaved' })

    const { result } = renderHook(() => useSaveShowToggle(1000), {
      wrapper: createWrapper(),
    })

    // Wait for initial check to complete
    await waitFor(() => expect(result.current.isSaved).toBe(true))

    // Toggle (should unsave)
    await act(async () => {
      await result.current.toggle()
    })

    // Verify unsave was called
    expect(mockApiRequest).toHaveBeenCalledWith(
      '/saved-shows/1000',
      expect.objectContaining({ method: 'DELETE' })
    )
  })

  it('rolls back optimistic update on error', async () => {
    // First call: check if saved (false)
    mockApiRequest.mockResolvedValueOnce({ is_saved: false })
    // Second call: save fails
    const error = new Error('Network error')
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useSaveShowToggle(1100), {
      wrapper: createWrapper(),
    })

    // Wait for initial check to complete
    await waitFor(() => expect(result.current.isSaved).toBe(false))

    // Toggle (should attempt to save and fail)
    await act(async () => {
      try {
        await result.current.toggle()
      } catch {
        // Expected to throw
      }
    })

    // Should have an error
    expect(result.current.error).toBeDefined()
  })

  it('exposes error from save mutation', async () => {
    mockApiRequest.mockResolvedValueOnce({ is_saved: false })
    const error = new Error('Save failed')
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useSaveShowToggle(1200), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSaved).toBe(false))

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
    mockApiRequest.mockResolvedValueOnce({ is_saved: true })
    const error = new Error('Unsave failed')
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useSaveShowToggle(1300), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSaved).toBe(true))

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
