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
    PUBLISH: (showId: string | number) => `/shows/${showId}/publish`,
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
import { useShowPublish } from './useShowPublish'

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

describe('useShowPublish', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockInvalidateShows.mockReset()
    mockInvalidateSavedShows.mockReset()
  })

  it('publishes a show with the correct endpoint and method', async () => {
    mockApiRequest.mockResolvedValueOnce(showResponse('approved'))

    const { result } = renderHook(() => useShowPublish(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(42)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith('/shows/42/publish', {
      method: 'POST',
    })
    expect(result.current.data?.status).toBe('approved')
  })

  it('reflects loading state while the request is in flight', async () => {
    let resolve: ((value: ShowResponse) => void) | undefined
    mockApiRequest.mockReturnValueOnce(
      new Promise<ShowResponse>((res) => {
        resolve = res
      })
    )

    const { result } = renderHook(() => useShowPublish(), {
      wrapper: createWrapper(),
    })

    act(() => {
      result.current.mutate(42)
    })

    await waitFor(() => expect(result.current.isPending).toBe(true))

    await act(async () => {
      resolve?.(showResponse('approved'))
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
  })

  it('invalidates shows and savedShows on success', async () => {
    mockApiRequest.mockResolvedValueOnce(showResponse('approved'))

    const { result } = renderHook(() => useShowPublish(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(42)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockInvalidateShows).toHaveBeenCalled()
    expect(mockInvalidateSavedShows).toHaveBeenCalled()
  })

  // The backend rejects publishing a show that is missing required fields; the
  // hook surfaces that 422 as an error rather than swallowing it.
  it('surfaces a validation error when required fields are missing', async () => {
    const error = new Error('Show is missing required fields')
    Object.assign(error, { status: 422 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useShowPublish(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(42)
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect((result.current.error as Error).message).toBe(
      'Show is missing required fields'
    )
  })

  it('does not invalidate queries on error', async () => {
    mockApiRequest.mockRejectedValueOnce(new Error('Server error'))

    const { result } = renderHook(() => useShowPublish(), {
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
