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
    MAKE_PRIVATE: (showId: string | number) => `/shows/${showId}/make-private`,
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
import { useShowMakePrivate } from './useShowMakePrivate'

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

describe('useShowMakePrivate', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockInvalidateShows.mockReset()
    mockInvalidateSavedShows.mockReset()
  })

  it('makes a show private with the correct endpoint and method', async () => {
    mockApiRequest.mockResolvedValueOnce(showResponse('private'))

    const { result } = renderHook(() => useShowMakePrivate(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(42)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith('/shows/42/make-private', {
      method: 'POST',
    })
    expect(result.current.data?.status).toBe('private')
  })

  it('reflects loading state while the request is in flight', async () => {
    let resolve: ((value: ShowResponse) => void) | undefined
    mockApiRequest.mockReturnValueOnce(
      new Promise<ShowResponse>((res) => {
        resolve = res
      })
    )

    const { result } = renderHook(() => useShowMakePrivate(), {
      wrapper: createWrapper(),
    })

    act(() => {
      result.current.mutate(42)
    })

    await waitFor(() => expect(result.current.isPending).toBe(true))

    await act(async () => {
      resolve?.(showResponse('private'))
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
  })

  it('invalidates shows and savedShows on success', async () => {
    mockApiRequest.mockResolvedValueOnce(showResponse('private'))

    const { result } = renderHook(() => useShowMakePrivate(), {
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

    const { result } = renderHook(() => useShowMakePrivate(), {
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

    const { result } = renderHook(() => useShowMakePrivate(), {
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
