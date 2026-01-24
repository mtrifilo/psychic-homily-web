import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

// Create mocks
const mockApiRequest = vi.fn()
const mockInvalidateShows = vi.fn()
const mockInvalidateSavedShows = vi.fn()

// Mock the api module
vi.mock('../api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    SHOWS: {
      PUBLISH: (id: number) => `/shows/${id}/publish`,
      MAKE_PRIVATE: (id: number) => `/shows/${id}/make-private`,
      UNPUBLISH: (id: number) => `/shows/${id}/unpublish`,
    },
  },
}))

// Mock the show logger
vi.mock('../utils/showLogger', () => ({
  showLogger: {
    debug: vi.fn(),
    error: vi.fn(),
    unpublishAttempt: vi.fn(),
    unpublishSuccess: vi.fn(),
    unpublishFailed: vi.fn(),
  },
}))

// Mock queryClient module
vi.mock('../queryClient', () => ({
  createInvalidateQueries: () => ({
    shows: mockInvalidateShows,
    savedShows: mockInvalidateSavedShows,
  }),
}))

// Import hooks after mocks are set up
import { useShowPublish } from './useShowPublish'
import { useShowMakePrivate } from './useShowMakePrivate'
import { useShowUnpublish } from './useShowUnpublish'

// Helper to create wrapper with query client
function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
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

describe('useShowPublish', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockInvalidateShows.mockReset()
    mockInvalidateSavedShows.mockReset()
  })

  it('publishes a show with correct endpoint', async () => {
    const mockResponse = {
      id: 123,
      title: 'Test Show',
      status: 'approved',
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useShowPublish(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(123)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/shows/123/publish',
      expect.objectContaining({
        method: 'POST',
      })
    )
  })

  it('returns updated show data on success', async () => {
    const mockResponse = {
      id: 456,
      title: 'Published Show',
      status: 'approved',
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useShowPublish(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(456)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(result.current.data?.status).toBe('approved')
  })

  it('invalidates shows and savedShows queries on success', async () => {
    mockApiRequest.mockResolvedValueOnce({ id: 789, status: 'approved' })

    const { result } = renderHook(() => useShowPublish(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(789)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockInvalidateShows).toHaveBeenCalled()
    expect(mockInvalidateSavedShows).toHaveBeenCalled()
  })

  it('handles errors', async () => {
    const error = new Error('Unauthorized')
    Object.assign(error, { status: 403 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useShowPublish(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(999)
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect(result.current.error).toBeDefined()
  })

  it('returns pending status when venue is unverified', async () => {
    const mockResponse = {
      id: 100,
      title: 'Unverified Venue Show',
      status: 'pending',
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useShowPublish(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(100)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(result.current.data?.status).toBe('pending')
  })
})

describe('useShowMakePrivate', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockInvalidateShows.mockReset()
    mockInvalidateSavedShows.mockReset()
  })

  it('makes a show private with correct endpoint', async () => {
    const mockResponse = {
      id: 123,
      title: 'Private Show',
      status: 'private',
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useShowMakePrivate(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(123)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/shows/123/make-private',
      expect.objectContaining({
        method: 'POST',
      })
    )
  })

  it('returns updated show data with private status', async () => {
    const mockResponse = {
      id: 456,
      title: 'Made Private',
      status: 'private',
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useShowMakePrivate(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(456)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(result.current.data?.status).toBe('private')
  })

  it('invalidates shows and savedShows queries on success', async () => {
    mockApiRequest.mockResolvedValueOnce({ id: 789, status: 'private' })

    const { result } = renderHook(() => useShowMakePrivate(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(789)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockInvalidateShows).toHaveBeenCalled()
    expect(mockInvalidateSavedShows).toHaveBeenCalled()
  })

  it('handles errors', async () => {
    const error = new Error('Show not found')
    Object.assign(error, { status: 404 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useShowMakePrivate(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(999)
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect(result.current.error).toBeDefined()
  })
})

describe('useShowUnpublish', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockInvalidateShows.mockReset()
    mockInvalidateSavedShows.mockReset()
  })

  it('unpublishes a show with correct endpoint', async () => {
    const mockResponse = {
      id: 123,
      title: 'Unpublished Show',
      status: 'pending',
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useShowUnpublish(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(123)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/shows/123/unpublish',
      expect.objectContaining({
        method: 'POST',
      })
    )
  })

  it('returns updated show data with pending status', async () => {
    const mockResponse = {
      id: 456,
      title: 'Unpublished',
      status: 'pending',
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useShowUnpublish(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(456)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(result.current.data?.status).toBe('pending')
  })

  it('invalidates shows and savedShows queries on success', async () => {
    mockApiRequest.mockResolvedValueOnce({ id: 789, status: 'pending' })

    const { result } = renderHook(() => useShowUnpublish(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(789)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockInvalidateShows).toHaveBeenCalled()
    expect(mockInvalidateSavedShows).toHaveBeenCalled()
  })

  it('handles errors', async () => {
    const error = new Error('Cannot unpublish')
    Object.assign(error, { status: 400 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useShowUnpublish(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(999)
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect(result.current.error).toBeDefined()
  })
})
