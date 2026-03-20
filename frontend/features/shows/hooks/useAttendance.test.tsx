import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

const mockApiRequest = vi.fn()
const mockInvalidateAttendance = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    ATTENDANCE: {
      SHOW: (showId: number) => `/shows/${showId}/attendance`,
      BATCH: '/shows/attendance/batch',
      MY_SHOWS: '/attendance/my-shows',
    },
  },
  API_BASE_URL: 'http://localhost:8080',
}))

vi.mock('@/lib/queryClient', () => ({
  queryKeys: {
    attendance: {
      show: (showId: number) => ['attendance', 'show', showId],
      batch: (showIds: number[]) => ['attendance', 'batch', ...showIds],
      myShows: (params?: Record<string, unknown>) => ['attendance', 'my-shows', params],
    },
  },
  createInvalidateQueries: () => ({
    attendance: mockInvalidateAttendance,
  }),
}))

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({ isAuthenticated: true }),
}))

import {
  useShowAttendance,
  useBatchAttendance,
  useSetAttendance,
  useRemoveAttendance,
  useMyShows,
} from './useAttendance'

function createWrapper(queryClient?: QueryClient) {
  const qc =
    queryClient ??
    new QueryClient({
      defaultOptions: {
        queries: { retry: false, gcTime: 0 },
        mutations: { retry: false },
      },
    })
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  }
}

describe('useShowAttendance', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches attendance for a show', async () => {
    mockApiRequest.mockResolvedValueOnce({
      going_count: 10,
      interested_count: 20,
      user_status: '',
    })

    const { result } = renderHook(() => useShowAttendance(1), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith('/shows/1/attendance', { method: 'GET' })
    expect(result.current.data?.going_count).toBe(10)
  })

  it('does not fetch when showId is 0', () => {
    const { result } = renderHook(() => useShowAttendance(0), {
      wrapper: createWrapper(),
    })

    expect(result.current.fetchStatus).toBe('idle')
  })
})

describe('useBatchAttendance', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches batch attendance via POST', async () => {
    mockApiRequest.mockResolvedValueOnce({
      attendance: {
        '1': { going_count: 10, interested_count: 5 },
        '2': { going_count: 3, interested_count: 8 },
      },
    })

    const { result } = renderHook(() => useBatchAttendance([1, 2]), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      '/shows/attendance/batch',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ show_ids: [1, 2] }),
      })
    )
    expect(result.current.data?.['1']?.going_count).toBe(10)
  })

  it('does not fetch when showIds is empty', () => {
    const { result } = renderHook(() => useBatchAttendance([]), {
      wrapper: createWrapper(),
    })

    expect(result.current.fetchStatus).toBe('idle')
  })
})

describe('useSetAttendance', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockInvalidateAttendance.mockReset()
  })

  it('sets attendance status with POST', async () => {
    mockApiRequest.mockResolvedValueOnce({ success: true, message: 'Attendance set' })

    const { result } = renderHook(() => useSetAttendance(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ showId: 1, status: 'going' })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/shows/1/attendance',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ status: 'going' }),
      })
    )
  })

  it('performs optimistic update when setting "going"', async () => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false, gcTime: 0 }, mutations: { retry: false } },
    })

    queryClient.setQueryData(['attendance', 'show', 1], {
      going_count: 5,
      interested_count: 3,
      user_status: '',
    })

    mockApiRequest.mockResolvedValueOnce({ success: true })

    const { result } = renderHook(() => useSetAttendance(), {
      wrapper: createWrapper(queryClient),
    })

    await act(async () => {
      result.current.mutate({ showId: 1, status: 'going' })
    })

    const cached = queryClient.getQueryData<{
      going_count: number
      interested_count: number
      user_status: string
    }>(['attendance', 'show', 1])

    expect(cached?.going_count).toBe(6)
    expect(cached?.interested_count).toBe(3)
    expect(cached?.user_status).toBe('going')
  })

  it('handles attendance mutation errors', async () => {
    const error = new Error('Server error')
    Object.assign(error, { status: 500 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useSetAttendance(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ showId: 1, status: 'interested' })
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect(result.current.error).toBeDefined()
  })
})

describe('useRemoveAttendance', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockInvalidateAttendance.mockReset()
  })

  it('removes attendance with DELETE', async () => {
    mockApiRequest.mockResolvedValueOnce({ success: true })

    const { result } = renderHook(() => useRemoveAttendance(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(1)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/shows/1/attendance',
      expect.objectContaining({ method: 'DELETE' })
    )
  })

  it('optimistically clears user status and decrements count', async () => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false, gcTime: 0 }, mutations: { retry: false } },
    })

    queryClient.setQueryData(['attendance', 'show', 1], {
      going_count: 5,
      interested_count: 3,
      user_status: 'going',
    })

    mockApiRequest.mockResolvedValueOnce({ success: true })

    const { result } = renderHook(() => useRemoveAttendance(), {
      wrapper: createWrapper(queryClient),
    })

    await act(async () => {
      result.current.mutate(1)
    })

    const cached = queryClient.getQueryData<{
      going_count: number
      interested_count: number
      user_status: string
    }>(['attendance', 'show', 1])

    expect(cached?.going_count).toBe(4)
    expect(cached?.user_status).toBe('')
  })
})

describe('useMyShows', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches user attending shows', async () => {
    mockApiRequest.mockResolvedValueOnce({ shows: [], total: 0 })

    const { result } = renderHook(() => useMyShows(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    const url = mockApiRequest.mock.calls[0][0] as string
    expect(url).toContain('/attendance/my-shows')
    expect(url).toContain('limit=20')
    expect(url).toContain('offset=0')
  })

  it('includes status filter when not "all"', async () => {
    mockApiRequest.mockResolvedValueOnce({ shows: [], total: 0 })

    const { result } = renderHook(() => useMyShows({ status: 'going' }), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    const url = mockApiRequest.mock.calls[0][0] as string
    expect(url).toContain('status=going')
  })

  it('does not include status when "all"', async () => {
    mockApiRequest.mockResolvedValueOnce({ shows: [], total: 0 })

    const { result } = renderHook(() => useMyShows({ status: 'all' }), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    const url = mockApiRequest.mock.calls[0][0] as string
    expect(url).not.toContain('status=')
  })
})
