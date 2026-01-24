import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createWrapper, createTestQueryClient } from '@/test/utils'

// Create mocks
const mockApiRequest = vi.fn()

// Mock the api module
vi.mock('../api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    SHOWS: {
      MY_SUBMISSIONS: '/shows/my-submissions',
    },
  },
  API_BASE_URL: 'http://localhost:8080',
}))

// Mock queryClient module
vi.mock('../queryClient', () => ({
  queryKeys: {
    mySubmissions: {
      list: () => ['mySubmissions', 'list'],
    },
  },
}))

// Import hooks after mocks are set up
import { useMySubmissions } from './useMySubmissions'

// Helper to create wrapper with specific query client
function createWrapperWithClient(queryClient: QueryClient) {
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    )
  }
}

describe('useMySubmissions', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches user submissions with default options', async () => {
    const mockResponse = {
      shows: [
        { id: 1, title: 'My Show 1', status: 'approved' },
        { id: 2, title: 'My Show 2', status: 'pending' },
      ],
      total: 2,
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useMySubmissions(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    // Default limit is 50, offset is 0
    expect(mockApiRequest).toHaveBeenCalledWith(
      '/shows/my-submissions?limit=50&offset=0',
      { method: 'GET' }
    )
    expect(result.current.data?.shows).toHaveLength(2)
  })

  it('supports custom limit', async () => {
    mockApiRequest.mockResolvedValueOnce({ shows: [], total: 0 })

    const { result } = renderHook(() => useMySubmissions({ limit: 25 }), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest.mock.calls[0][0]).toContain('limit=25')
  })

  it('supports custom offset for pagination', async () => {
    mockApiRequest.mockResolvedValueOnce({ shows: [], total: 0 })

    const { result } = renderHook(() => useMySubmissions({ offset: 50 }), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest.mock.calls[0][0]).toContain('offset=50')
  })

  it('combines limit and offset', async () => {
    mockApiRequest.mockResolvedValueOnce({ shows: [], total: 0 })

    const { result } = renderHook(
      () => useMySubmissions({ limit: 10, offset: 20 }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    const calledUrl = mockApiRequest.mock.calls[0][0]
    expect(calledUrl).toContain('limit=10')
    expect(calledUrl).toContain('offset=20')
  })

  it('handles authentication error', async () => {
    const error = new Error('Unauthorized')
    Object.assign(error, { status: 401 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useMySubmissions(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect((result.current.error as Error).message).toBe('Unauthorized')
  })

  it('handles API errors', async () => {
    const error = new Error('Server error')
    Object.assign(error, { status: 500 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useMySubmissions(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect(result.current.error).toBeDefined()
  })

  it('returns empty list when user has no submissions', async () => {
    mockApiRequest.mockResolvedValueOnce({ shows: [], total: 0 })

    const { result } = renderHook(() => useMySubmissions(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(result.current.data?.shows).toHaveLength(0)
    expect(result.current.data?.total).toBe(0)
  })
})
