import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { createWrapper } from '@/test/utils'
import type { ShowResponse, ShowStatus } from '../types'

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
    UNPUBLISH: (showId: string | number) => `/shows/${showId}/unpublish`,
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

// Mock showLogger to suppress console output in tests
vi.mock('@/lib/utils/showLogger', () => ({
  showLogger: {
    unpublishAttempt: vi.fn(),
    unpublishSuccess: vi.fn(),
    unpublishFailed: vi.fn(),
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
      requestId: undefined as string | undefined,
    }),
  },
  ShowErrorCode: {
    UNKNOWN: 'UNKNOWN',
  },
}))

// Import hook after mocks are set up
import { useShowUnpublish } from './useShowUnpublish'

function showResponse(status: ShowStatus): ShowResponse {
  return {
    id: 42,
    slug: 'test-show',
    title: 'Test Show',
    event_date: '2025-06-15T20:00:00Z',
    status,
    venues: [],
    artists: [],
    created_at: '2025-01-01T00:00:00Z',
    updated_at: '2025-01-01T00:00:00Z',
    is_sold_out: false,
    is_cancelled: false,
  }
}

describe('useShowUnpublish', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockInvalidateShows.mockReset()
    mockInvalidateSavedShows.mockReset()
  })

  it('unpublishes a show with the correct endpoint and method', async () => {
    mockApiRequest.mockResolvedValueOnce(showResponse('pending'))

    const { result } = renderHook(() => useShowUnpublish(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(42)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith('/shows/42/unpublish', {
      method: 'POST',
    })
  })

  // Unpublishing an approved show reverts it to a pending (draft) state.
  it('returns the show reverted to a pending status', async () => {
    mockApiRequest.mockResolvedValueOnce(showResponse('pending'))

    const { result } = renderHook(() => useShowUnpublish(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(42)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(result.current.data?.status).toBe('pending')
  })

  it('reflects loading state while the request is in flight', async () => {
    let resolve: ((value: ShowResponse) => void) | undefined
    mockApiRequest.mockReturnValueOnce(
      new Promise<ShowResponse>((res) => {
        resolve = res
      })
    )

    const { result } = renderHook(() => useShowUnpublish(), {
      wrapper: createWrapper(),
    })

    act(() => {
      result.current.mutate(42)
    })

    await waitFor(() => expect(result.current.isPending).toBe(true))

    await act(async () => {
      resolve?.(showResponse('pending'))
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
  })

  it('invalidates shows and savedShows on success', async () => {
    mockApiRequest.mockResolvedValueOnce(showResponse('pending'))

    const { result } = renderHook(() => useShowUnpublish(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(42)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockInvalidateShows).toHaveBeenCalled()
    expect(mockInvalidateSavedShows).toHaveBeenCalled()
  })

  it('handles errors and sets error state', async () => {
    const error = new Error('Forbidden')
    Object.assign(error, { status: 403 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useShowUnpublish(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(42)
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect((result.current.error as Error).message).toBe('Forbidden')
  })

  it('does not invalidate queries on error', async () => {
    mockApiRequest.mockRejectedValueOnce(new Error('Server error'))

    const { result } = renderHook(() => useShowUnpublish(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(42)
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect(mockInvalidateShows).not.toHaveBeenCalled()
    expect(mockInvalidateSavedShows).not.toHaveBeenCalled()
  })
})
