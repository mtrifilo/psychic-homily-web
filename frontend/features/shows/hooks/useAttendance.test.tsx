import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { AttendanceCounts } from '../types'

// Create mocks
const mockApiRequest = vi.fn()
const mockInvalidateAttendance = vi.fn()
const mockIsAuthenticated = vi.fn(() => true)

// Mock the api module
vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    ATTENDANCE: {
      SHOW: (showId: number) => `/shows/${showId}/attendance`,
      BATCH: '/shows/attendance/batch',
      MY_SHOWS: '/attendance/my-shows',
    },
  },
}))

// Mock queryClient module
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

// Mock AuthContext
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({
    isAuthenticated: mockIsAuthenticated(),
  }),
}))

// Import hooks after mocks are set up
import {
  useShowAttendance,
  useBatchAttendance,
  useSetAttendance,
  useRemoveAttendance,
  useMyShows,
} from './useAttendance'

// Helper to create wrapper with a fresh query client
function createWrapper(queryClient?: QueryClient) {
  const qc = queryClient ?? new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0 },
      mutations: { retry: false },
    },
  })
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return (
      <QueryClientProvider client={qc}>{children}</QueryClientProvider>
    )
  }
}

describe('useShowAttendance', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches attendance counts for a show', async () => {
    const mockData: AttendanceCounts = {
      show_id: 1,
      going_count: 5,
      interested_count: 10,
      user_status: 'going',
    }
    mockApiRequest.mockResolvedValueOnce(mockData)

    const { result } = renderHook(() => useShowAttendance(1), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith('/shows/1/attendance', {
      method: 'GET',
    })
    expect(result.current.data?.going_count).toBe(5)
    expect(result.current.data?.interested_count).toBe(10)
    expect(result.current.data?.user_status).toBe('going')
  })

  it('does not fetch when showId is 0', () => {
    const { result } = renderHook(() => useShowAttendance(0), {
      wrapper: createWrapper(),
    })

    expect(mockApiRequest).not.toHaveBeenCalled()
    expect(result.current.fetchStatus).toBe('idle')
  })

  it('does not fetch when showId is negative', () => {
    const { result } = renderHook(() => useShowAttendance(-1), {
      wrapper: createWrapper(),
    })

    expect(mockApiRequest).not.toHaveBeenCalled()
    expect(result.current.fetchStatus).toBe('idle')
  })

  it('handles API errors', async () => {
    const error = new Error('Server error')
    Object.assign(error, { status: 500 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useShowAttendance(1), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect(result.current.error).toBeDefined()
  })

  it('returns empty user_status for unauthenticated users', async () => {
    const mockData: AttendanceCounts = {
      show_id: 1,
      going_count: 3,
      interested_count: 7,
      user_status: '',
    }
    mockApiRequest.mockResolvedValueOnce(mockData)

    const { result } = renderHook(() => useShowAttendance(1), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(result.current.data?.user_status).toBe('')
  })
})

describe('useBatchAttendance', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches batch attendance for multiple shows', async () => {
    const mockResponse = {
      attendance: {
        '1': { show_id: 1, going_count: 5, interested_count: 10, user_status: '' },
        '2': { show_id: 2, going_count: 3, interested_count: 8, user_status: 'going' },
      },
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useBatchAttendance([1, 2]), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith('/shows/attendance/batch', {
      method: 'POST',
      body: JSON.stringify({ show_ids: [1, 2] }),
    })
    expect(result.current.data?.['1'].going_count).toBe(5)
    expect(result.current.data?.['2'].user_status).toBe('going')
  })

  it('does not fetch when showIds is empty', () => {
    const { result } = renderHook(() => useBatchAttendance([]), {
      wrapper: createWrapper(),
    })

    expect(mockApiRequest).not.toHaveBeenCalled()
    expect(result.current.fetchStatus).toBe('idle')
  })

  it('handles API errors', async () => {
    mockApiRequest.mockRejectedValueOnce(new Error('Network error'))

    const { result } = renderHook(() => useBatchAttendance([1, 2]), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
  })
})

describe('useSetAttendance', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockInvalidateAttendance.mockReset()
  })

  it('sets going status with correct endpoint and method', async () => {
    mockApiRequest.mockResolvedValueOnce({ success: true, message: 'Status set' })

    const { result } = renderHook(() => useSetAttendance(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ showId: 1, status: 'going' })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith('/shows/1/attendance', {
      method: 'POST',
      body: JSON.stringify({ status: 'going' }),
    })
  })

  it('sets interested status with correct endpoint and method', async () => {
    mockApiRequest.mockResolvedValueOnce({ success: true, message: 'Status set' })

    const { result } = renderHook(() => useSetAttendance(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ showId: 5, status: 'interested' })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith('/shows/5/attendance', {
      method: 'POST',
      body: JSON.stringify({ status: 'interested' }),
    })
  })

  it('optimistically updates going count from no status', async () => {
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false, gcTime: 0 },
        mutations: { retry: false },
      },
    })

    // Seed cache with initial attendance data
    const initialData: AttendanceCounts = {
      show_id: 1,
      going_count: 3,
      interested_count: 5,
      user_status: '',
    }
    queryClient.setQueryData(['attendance', 'show', 1], initialData)

    // Delay API response to observe optimistic update
    mockApiRequest.mockImplementation(
      () => new Promise(resolve => setTimeout(() => resolve({ success: true, message: 'ok' }), 100))
    )

    const { result } = renderHook(() => useSetAttendance(), {
      wrapper: createWrapper(queryClient),
    })

    await act(async () => {
      result.current.mutate({ showId: 1, status: 'going' })
    })

    // Check optimistic update was applied
    const optimisticData = queryClient.getQueryData<AttendanceCounts>(['attendance', 'show', 1])
    expect(optimisticData?.going_count).toBe(4)
    expect(optimisticData?.interested_count).toBe(5)
    expect(optimisticData?.user_status).toBe('going')

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
  })

  it('optimistically updates when switching from interested to going', async () => {
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false, gcTime: 0 },
        mutations: { retry: false },
      },
    })

    const initialData: AttendanceCounts = {
      show_id: 2,
      going_count: 3,
      interested_count: 5,
      user_status: 'interested',
    }
    queryClient.setQueryData(['attendance', 'show', 2], initialData)

    mockApiRequest.mockImplementation(
      () => new Promise(resolve => setTimeout(() => resolve({ success: true, message: 'ok' }), 100))
    )

    const { result } = renderHook(() => useSetAttendance(), {
      wrapper: createWrapper(queryClient),
    })

    await act(async () => {
      result.current.mutate({ showId: 2, status: 'going' })
    })

    const optimisticData = queryClient.getQueryData<AttendanceCounts>(['attendance', 'show', 2])
    expect(optimisticData?.going_count).toBe(4)       // incremented
    expect(optimisticData?.interested_count).toBe(4)   // decremented
    expect(optimisticData?.user_status).toBe('going')

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
  })

  it('calls rollback with previous data on error', async () => {
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false, gcTime: Infinity },
        mutations: { retry: false },
      },
    })

    const initialData: AttendanceCounts = {
      show_id: 3,
      going_count: 2,
      interested_count: 4,
      user_status: '',
    }
    queryClient.setQueryData(['attendance', 'show', 3], initialData)

    // Track setQueryData calls to verify rollback happens
    const setQueryDataSpy = vi.spyOn(queryClient, 'setQueryData')

    mockApiRequest.mockRejectedValueOnce(new Error('Server error'))

    const { result } = renderHook(() => useSetAttendance(), {
      wrapper: createWrapper(queryClient),
    })

    await act(async () => {
      result.current.mutate({ showId: 3, status: 'going' })
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    // Verify that setQueryData was called with the original data for rollback
    // First call is optimistic update, second call is rollback
    const rollbackCall = setQueryDataSpy.mock.calls.find(
      call => {
        const data = call[1] as AttendanceCounts | undefined
        return data?.going_count === 2 && data?.user_status === ''
      }
    )
    expect(rollbackCall).toBeDefined()

    setQueryDataSpy.mockRestore()
  })

  it('invalidates attendance queries on settled', async () => {
    mockApiRequest.mockResolvedValueOnce({ success: true, message: 'ok' })

    const { result } = renderHook(() => useSetAttendance(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ showId: 1, status: 'going' })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockInvalidateAttendance).toHaveBeenCalled()
  })

  it('handles API errors', async () => {
    const error = new Error('Unauthorized')
    Object.assign(error, { status: 401 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useSetAttendance(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ showId: 1, status: 'going' })
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect((result.current.error as Error).message).toBe('Unauthorized')
  })
})

describe('useRemoveAttendance', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockInvalidateAttendance.mockReset()
  })

  it('removes attendance with correct endpoint and method', async () => {
    mockApiRequest.mockResolvedValueOnce({ success: true, message: 'Removed' })

    const { result } = renderHook(() => useRemoveAttendance(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(1)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith('/shows/1/attendance', {
      method: 'DELETE',
    })
  })

  it('optimistically clears going status', async () => {
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false, gcTime: 0 },
        mutations: { retry: false },
      },
    })

    const initialData: AttendanceCounts = {
      show_id: 1,
      going_count: 5,
      interested_count: 3,
      user_status: 'going',
    }
    queryClient.setQueryData(['attendance', 'show', 1], initialData)

    mockApiRequest.mockImplementation(
      () => new Promise(resolve => setTimeout(() => resolve({ success: true, message: 'ok' }), 100))
    )

    const { result } = renderHook(() => useRemoveAttendance(), {
      wrapper: createWrapper(queryClient),
    })

    await act(async () => {
      result.current.mutate(1)
    })

    const optimisticData = queryClient.getQueryData<AttendanceCounts>(['attendance', 'show', 1])
    expect(optimisticData?.going_count).toBe(4)       // decremented
    expect(optimisticData?.interested_count).toBe(3)   // unchanged
    expect(optimisticData?.user_status).toBe('')

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
  })

  it('optimistically clears interested status', async () => {
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false, gcTime: 0 },
        mutations: { retry: false },
      },
    })

    const initialData: AttendanceCounts = {
      show_id: 2,
      going_count: 5,
      interested_count: 3,
      user_status: 'interested',
    }
    queryClient.setQueryData(['attendance', 'show', 2], initialData)

    mockApiRequest.mockImplementation(
      () => new Promise(resolve => setTimeout(() => resolve({ success: true, message: 'ok' }), 100))
    )

    const { result } = renderHook(() => useRemoveAttendance(), {
      wrapper: createWrapper(queryClient),
    })

    await act(async () => {
      result.current.mutate(2)
    })

    const optimisticData = queryClient.getQueryData<AttendanceCounts>(['attendance', 'show', 2])
    expect(optimisticData?.going_count).toBe(5)        // unchanged
    expect(optimisticData?.interested_count).toBe(2)    // decremented
    expect(optimisticData?.user_status).toBe('')

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
  })

  it('does not decrement below zero', async () => {
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false, gcTime: 0 },
        mutations: { retry: false },
      },
    })

    const initialData: AttendanceCounts = {
      show_id: 3,
      going_count: 0,
      interested_count: 0,
      user_status: 'going',
    }
    queryClient.setQueryData(['attendance', 'show', 3], initialData)

    mockApiRequest.mockImplementation(
      () => new Promise(resolve => setTimeout(() => resolve({ success: true, message: 'ok' }), 100))
    )

    const { result } = renderHook(() => useRemoveAttendance(), {
      wrapper: createWrapper(queryClient),
    })

    await act(async () => {
      result.current.mutate(3)
    })

    const optimisticData = queryClient.getQueryData<AttendanceCounts>(['attendance', 'show', 3])
    expect(optimisticData?.going_count).toBe(0)   // clamped at 0

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
  })

  it('calls rollback with previous data on error', async () => {
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false, gcTime: Infinity },
        mutations: { retry: false },
      },
    })

    const initialData: AttendanceCounts = {
      show_id: 4,
      going_count: 10,
      interested_count: 5,
      user_status: 'going',
    }
    queryClient.setQueryData(['attendance', 'show', 4], initialData)

    // Track setQueryData calls to verify rollback happens
    const setQueryDataSpy = vi.spyOn(queryClient, 'setQueryData')

    mockApiRequest.mockRejectedValueOnce(new Error('Failed'))

    const { result } = renderHook(() => useRemoveAttendance(), {
      wrapper: createWrapper(queryClient),
    })

    await act(async () => {
      result.current.mutate(4)
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    // Verify that setQueryData was called with the original data for rollback
    // First call is optimistic update, second call is rollback
    const rollbackCall = setQueryDataSpy.mock.calls.find(
      call => {
        const data = call[1] as AttendanceCounts | undefined
        return data?.going_count === 10 && data?.user_status === 'going'
      }
    )
    expect(rollbackCall).toBeDefined()

    setQueryDataSpy.mockRestore()
  })

  it('invalidates attendance queries on settled', async () => {
    mockApiRequest.mockResolvedValueOnce({ success: true, message: 'ok' })

    const { result } = renderHook(() => useRemoveAttendance(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(1)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockInvalidateAttendance).toHaveBeenCalled()
  })

  it('handles network errors', async () => {
    mockApiRequest.mockRejectedValueOnce(new TypeError('Failed to fetch'))

    const { result } = renderHook(() => useRemoveAttendance(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate(1)
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect(result.current.error).toBeInstanceOf(TypeError)
  })
})

describe('useMyShows', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockIsAuthenticated.mockReturnValue(true)
  })

  it('fetches my shows when authenticated', async () => {
    const mockResponse = {
      shows: [
        {
          show_id: 1,
          title: 'Show 1',
          slug: 'show-1',
          event_date: '2025-06-15T20:00:00Z',
          status: 'going',
          venue_name: 'The Venue',
          venue_slug: 'the-venue',
          city: 'Phoenix',
          state: 'AZ',
        },
      ],
      total: 1,
      limit: 20,
      offset: 0,
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useMyShows(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/attendance/my-shows?limit=20&offset=0',
      { method: 'GET' }
    )
    expect(result.current.data?.shows).toHaveLength(1)
    expect(result.current.data?.total).toBe(1)
  })

  it('does not fetch when not authenticated', () => {
    mockIsAuthenticated.mockReturnValue(false)

    const { result } = renderHook(() => useMyShows(), {
      wrapper: createWrapper(),
    })

    expect(mockApiRequest).not.toHaveBeenCalled()
    expect(result.current.fetchStatus).toBe('idle')
  })

  it('includes status filter when not "all"', async () => {
    mockApiRequest.mockResolvedValueOnce({
      shows: [],
      total: 0,
      limit: 20,
      offset: 0,
    })

    const { result } = renderHook(() => useMyShows({ status: 'going' }), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/attendance/my-shows?status=going&limit=20&offset=0',
      { method: 'GET' }
    )
  })

  it('does not include status filter when "all"', async () => {
    mockApiRequest.mockResolvedValueOnce({
      shows: [],
      total: 0,
      limit: 20,
      offset: 0,
    })

    const { result } = renderHook(() => useMyShows({ status: 'all' }), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    const calledUrl = mockApiRequest.mock.calls[0][0]
    expect(calledUrl).not.toContain('status=')
  })

  it('supports custom limit and offset', async () => {
    mockApiRequest.mockResolvedValueOnce({
      shows: [],
      total: 0,
      limit: 10,
      offset: 20,
    })

    const { result } = renderHook(
      () => useMyShows({ limit: 10, offset: 20 }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/attendance/my-shows?limit=10&offset=20',
      { method: 'GET' }
    )
  })

  it('handles API errors', async () => {
    mockApiRequest.mockRejectedValueOnce(new Error('Unauthorized'))

    const { result } = renderHook(() => useMyShows(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect(result.current.error).toBeDefined()
  })
})
