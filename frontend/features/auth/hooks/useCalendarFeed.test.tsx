import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createWrapper, createTestQueryClient } from '@/test/utils'

// Create mocks
const mockApiRequest = vi.fn()
const mockInvalidateCalendar = vi.fn()

// Mock the api module
vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    CALENDAR: {
      TOKEN: '/calendar/token',
    },
  },
  API_BASE_URL: 'http://localhost:8080',
}))

// Mock queryClient module
vi.mock('@/lib/queryClient', () => ({
  queryKeys: {
    calendar: {
      all: ['calendar'],
      tokenStatus: ['calendar', 'tokenStatus'],
    },
  },
  createInvalidateQueries: () => ({
    calendar: mockInvalidateCalendar,
  }),
}))

// Import hooks after mocks are set up
import {
  useCalendarTokenStatus,
  useCreateCalendarToken,
  useDeleteCalendarToken,
} from './useCalendarFeed'

describe('useCalendarTokenStatus', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches token status when enabled', async () => {
    const mockResponse = {
      has_token: true,
      created_at: '2025-03-01T12:00:00Z',
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useCalendarTokenStatus(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith('/calendar/token', {
      method: 'GET',
    })
    expect(result.current.data?.has_token).toBe(true)
    expect(result.current.data?.created_at).toBe('2025-03-01T12:00:00Z')
  })

  it('fetches when enabled is true explicitly', async () => {
    mockApiRequest.mockResolvedValueOnce({ has_token: false })

    const { result } = renderHook(() => useCalendarTokenStatus(true), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith('/calendar/token', {
      method: 'GET',
    })
    expect(result.current.data?.has_token).toBe(false)
  })

  it('does not fetch when enabled is false', () => {
    const { result } = renderHook(() => useCalendarTokenStatus(false), {
      wrapper: createWrapper(),
    })

    expect(mockApiRequest).not.toHaveBeenCalled()
    expect(result.current.fetchStatus).toBe('idle')
  })

  it('handles API errors', async () => {
    const error = new Error('Unauthorized')
    Object.assign(error, { status: 401 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useCalendarTokenStatus(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect((result.current.error as Error).message).toBe('Unauthorized')
  })

  it('returns no created_at when user has no token', async () => {
    mockApiRequest.mockResolvedValueOnce({ has_token: false })

    const { result } = renderHook(() => useCalendarTokenStatus(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(result.current.data?.has_token).toBe(false)
    expect(result.current.data?.created_at).toBeUndefined()
  })
})

describe('useCreateCalendarToken', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockInvalidateCalendar.mockReset()
  })

  it('creates a calendar token and invalidates queries', async () => {
    const mockResponse = {
      token: 'abc123token',
      feed_url: 'https://api.psychichomily.com/calendar/feed/abc123token',
      created_at: '2025-03-15T10:00:00Z',
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useCreateCalendarToken(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      const data = await result.current.mutateAsync()
      expect(data.token).toBe('abc123token')
      expect(data.feed_url).toContain('abc123token')
      expect(data.created_at).toBe('2025-03-15T10:00:00Z')
    })

    expect(mockApiRequest).toHaveBeenCalledWith('/calendar/token', {
      method: 'POST',
    })
    expect(mockInvalidateCalendar).toHaveBeenCalled()
  })

  it('handles creation errors', async () => {
    const error = new Error('Server error')
    Object.assign(error, { status: 500 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useCreateCalendarToken(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      try {
        await result.current.mutateAsync()
      } catch (e) {
        expect((e as Error).message).toBe('Server error')
      }
    })

    expect(mockInvalidateCalendar).not.toHaveBeenCalled()
  })
})

describe('useDeleteCalendarToken', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockInvalidateCalendar.mockReset()
  })

  it('deletes the calendar token and invalidates queries', async () => {
    const mockResponse = {
      success: true,
      message: 'Calendar feed token deleted',
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useDeleteCalendarToken(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      const data = await result.current.mutateAsync()
      expect(data.success).toBe(true)
      expect(data.message).toBe('Calendar feed token deleted')
    })

    expect(mockApiRequest).toHaveBeenCalledWith('/calendar/token', {
      method: 'DELETE',
    })
    expect(mockInvalidateCalendar).toHaveBeenCalled()
  })

  it('handles deletion errors', async () => {
    const error = new Error('Not found')
    Object.assign(error, { status: 404 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useDeleteCalendarToken(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      try {
        await result.current.mutateAsync()
      } catch (e) {
        expect((e as Error).message).toBe('Not found')
      }
    })

    expect(mockInvalidateCalendar).not.toHaveBeenCalled()
  })
})
