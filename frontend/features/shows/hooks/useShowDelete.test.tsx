import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { createWrapper } from '@/test/utils'

// Create mocks
const mockApiRequest = vi.fn()
const mockInvalidateShows = vi.fn()
const mockInvalidateSavedShows = vi.fn()

// Mock the api module
vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}))

vi.mock('@/features/shows/api', () => ({
  showEndpoints: {
    DELETE: (showId: string | number) => `/shows/${showId}`,
  },
}))

// Mock queryClient module
vi.mock('@/lib/queryClient', () => ({
  queryKeys: {},
  createInvalidateQueries: () => ({
    shows: mockInvalidateShows,
    savedShows: mockInvalidateSavedShows,
  }),
}))

// Mock showLogger
vi.mock('@/lib/utils/showLogger', () => ({
  showLogger: {
    deleteAttempt: vi.fn(),
    deleteSuccess: vi.fn(),
    deleteFailed: vi.fn(),
    debug: vi.fn(),
    info: vi.fn(),
    warn: vi.fn(),
    error: vi.fn(),
  },
}))

// Mock errors module
vi.mock('@/lib/errors', () => ({
  ShowError: {
    fromUnknown: (error: unknown) => ({
      code: 'UNKNOWN',
      message: error instanceof Error ? error.message : String(error),
      requestId: undefined,
    }),
  },
  ShowErrorCode: {
    UNKNOWN: 'UNKNOWN',
  },
}))

// Import hooks after mocks are set up
import { useShowDelete } from './useShowDelete'


describe('useShowDelete', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockInvalidateShows.mockReset()
    mockInvalidateSavedShows.mockReset()
  })

  it('deletes a show with correct endpoint and method', async () => {
    mockApiRequest.mockResolvedValueOnce(undefined)

    const { result } = renderHook(() => useShowDelete(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(42)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith('/shows/42', {
      method: 'DELETE',
    })
  })

  it('invalidates shows and savedShows on success', async () => {
    mockApiRequest.mockResolvedValueOnce(undefined)

    const { result } = renderHook(() => useShowDelete(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(1)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockInvalidateShows).toHaveBeenCalled()
    expect(mockInvalidateSavedShows).toHaveBeenCalled()
  })

  it('returns void on success (no response body)', async () => {
    mockApiRequest.mockResolvedValueOnce(undefined)

    const { result } = renderHook(() => useShowDelete(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(1)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(result.current.data).toBeUndefined()
  })

  it('handles 404 not found errors', async () => {
    const error = new Error('Show not found')
    Object.assign(error, { status: 404 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useShowDelete(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(999)
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect((result.current.error as Error).message).toBe('Show not found')
  })

  it('does not invalidate queries on error', async () => {
    mockApiRequest.mockRejectedValueOnce(new Error('Server error'))

    const { result } = renderHook(() => useShowDelete(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(1)
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect(mockInvalidateShows).not.toHaveBeenCalled()
    expect(mockInvalidateSavedShows).not.toHaveBeenCalled()
  })

  it('can be called multiple times sequentially', async () => {
    mockApiRequest.mockResolvedValue(undefined)

    const { result } = renderHook(() => useShowDelete(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(1)
    })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    await act(async () => {
      result.current.mutate(2)
    })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledTimes(2)
    expect(mockApiRequest).toHaveBeenCalledWith('/shows/1', { method: 'DELETE' })
    expect(mockApiRequest).toHaveBeenCalledWith('/shows/2', { method: 'DELETE' })
  })
})
