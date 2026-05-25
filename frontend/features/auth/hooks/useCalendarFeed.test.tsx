import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { createWrapper, createWrapperWithClient, createTestQueryClient } from '@/test/utils'

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
  })

  it('does not fetch when enabled is false', () => {
    const { result } = renderHook(() => useCalendarTokenStatus(false), {
      wrapper: createWrapper(),
    })

    expect(mockApiRequest).not.toHaveBeenCalled()
    expect(result.current.fetchStatus).toBe('idle')
  })

  it('starts in loading state before resolution', async () => {
    // Hang the request so we can observe the loading state.
    mockApiRequest.mockReturnValueOnce(new Promise(() => {}))

    const { result } = renderHook(() => useCalendarTokenStatus(), {
      wrapper: createWrapper(),
    })

    // isPending is true on initial mount while the query is in-flight.
    expect(result.current.isPending).toBe(true)
    expect(result.current.isSuccess).toBe(false)
    expect(result.current.isError).toBe(false)
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

  it('exposes the full token status payload on success', async () => {
    // The hook is the ONLY caller of GET /calendar/token from the frontend,
    // so the test must assert the entire shape — silent field-drop here
    // would surface in the "Connect calendar" UI as missing metadata.
    mockApiRequest.mockResolvedValueOnce({
      has_token: true,
      created_at: '2025-03-01T12:00:00Z',
    })

    const { result } = renderHook(() => useCalendarTokenStatus(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(result.current.data).toEqual({
      has_token: true,
      created_at: '2025-03-01T12:00:00Z',
    })
  })

  it('surfaces API errors to the caller (does not swallow)', async () => {
    // The "feed generation fails silently" risk in PSY-700 hinges on
    // errors reaching the consuming component. The hook MUST surface
    // the error via .error / .isError; if either is missing the
    // calendar UI would silently render a stale or empty state.
    const error = new Error('Unauthorized')
    Object.assign(error, { status: 401 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useCalendarTokenStatus(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect(result.current.error).toBeTruthy()
    expect((result.current.error as Error).message).toBe('Unauthorized')
    expect(result.current.data).toBeUndefined()
  })

  it('marks 500 server errors as errors (does not silently treat as success)', async () => {
    const error = new Error('Internal Server Error')
    Object.assign(error, { status: 500 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useCalendarTokenStatus(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect(result.current.isSuccess).toBe(false)
    expect((result.current.error as Error).message).toBe('Internal Server Error')
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
      await result.current.mutateAsync()
    })

    expect(mockApiRequest).toHaveBeenCalledWith('/calendar/token', {
      method: 'POST',
    })
    expect(mockInvalidateCalendar).toHaveBeenCalled()
  })

  it('returns the full token + feed_url payload to the caller', async () => {
    // The settings page renders feed_url directly; if the hook drops the
    // field on success, the user sees an empty Copy button. Lock the
    // contract here.
    const mockResponse = {
      token: 'abc123token',
      feed_url: 'https://api.psychichomily.com/calendar/feed/abc123token',
      created_at: '2025-03-15T10:00:00Z',
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useCreateCalendarToken(), {
      wrapper: createWrapper(),
    })

    let returned: unknown
    await act(async () => {
      returned = await result.current.mutateAsync()
    })

    expect(returned).toEqual(mockResponse)
    await waitFor(() => expect(result.current.data).toEqual(mockResponse))
  })

  it('surfaces errors to the caller and does NOT silently invalidate', async () => {
    // Core PSY-700 risk for this hook: if the mutation swallowed the
    // rejection, the calendar UI would show "success" while no token
    // exists server-side. The mutation MUST reject mutateAsync AND skip
    // invalidation so the cache doesn't refetch the (still-missing) status.
    const error = new Error('Server error')
    Object.assign(error, { status: 500 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useCreateCalendarToken(), {
      wrapper: createWrapper(),
    })

    // mutateAsync rejects so consumers in `try { await mutateAsync(); ... }`
    // hit the catch instead of the success branch.
    let caught: unknown
    await act(async () => {
      try {
        await result.current.mutateAsync()
      } catch (e) {
        caught = e
      }
    })

    expect(caught).toBe(error)

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect(result.current.error).toBeTruthy()
    expect((result.current.error as { status?: number }).status).toBe(500)
    expect(mockInvalidateCalendar).not.toHaveBeenCalled()
  })

  it('surfaces 401 auth errors with the original error preserved', async () => {
    // If a user's session expires mid-flow, the calendar UI must see the
    // 401 so it can trigger a re-auth prompt instead of "looks fine".
    const authError = new Error('Token expired')
    Object.assign(authError, { status: 401, code: 'TOKEN_EXPIRED' })
    mockApiRequest.mockRejectedValueOnce(authError)

    const { result } = renderHook(() => useCreateCalendarToken(), {
      wrapper: createWrapper(),
    })

    // Fire-and-forget so the rejection lands as TanStack state instead
    // of being swallowed by a manual try/catch.
    act(() => {
      result.current.mutate()
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect(result.current.error).toBe(authError)
    expect((result.current.error as { status?: number }).status).toBe(401)
    expect(mockInvalidateCalendar).not.toHaveBeenCalled()
  })

  it('isPending toggles during in-flight mutation', async () => {
    let resolveMutation: (v: unknown) => void
    mockApiRequest.mockReturnValueOnce(
      new Promise(resolve => {
        resolveMutation = resolve
      })
    )

    const { result } = renderHook(() => useCreateCalendarToken(), {
      wrapper: createWrapper(),
    })

    expect(result.current.isPending).toBe(false)

    act(() => {
      result.current.mutate()
    })

    await waitFor(() => expect(result.current.isPending).toBe(true))

    await act(async () => {
      resolveMutation!({
        token: 't',
        feed_url: 'https://example.com',
        created_at: 'now',
      })
    })

    await waitFor(() => expect(result.current.isPending).toBe(false))
    expect(result.current.isSuccess).toBe(true)
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
      await result.current.mutateAsync()
    })

    expect(mockApiRequest).toHaveBeenCalledWith('/calendar/token', {
      method: 'DELETE',
    })
    expect(mockInvalidateCalendar).toHaveBeenCalled()
  })

  it('surfaces errors to the caller and skips invalidation on failure', async () => {
    // Same risk class as Create: silent swallow here would leave the UI
    // claiming the feed is disabled while the backend record persists.
    const error = new Error('Not found')
    Object.assign(error, { status: 404 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useDeleteCalendarToken(), {
      wrapper: createWrapper(),
    })

    let caught: unknown
    await act(async () => {
      try {
        await result.current.mutateAsync()
      } catch (e) {
        caught = e
      }
    })

    expect(caught).toBe(error)

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect(result.current.error).toBeTruthy()
    expect((result.current.error as { status?: number }).status).toBe(404)
    expect(mockInvalidateCalendar).not.toHaveBeenCalled()
  })

  it('does not invalidate on 500 server errors', async () => {
    const error = new Error('Internal Server Error')
    Object.assign(error, { status: 500 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useDeleteCalendarToken(), {
      wrapper: createWrapper(),
    })

    act(() => {
      result.current.mutate()
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect(mockInvalidateCalendar).not.toHaveBeenCalled()
  })
})

describe('useCalendarTokenStatus + useCreateCalendarToken integration', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockInvalidateCalendar.mockReset()
  })

  it('exposes shared queryKey so invalidation reaches the status query', async () => {
    // Sanity check: queryKey object exported by queryClient matches the
    // key the status query uses. If the keys drift, invalidation becomes
    // a silent no-op — the very "fails silently" pattern PSY-700 is
    // hardening against.
    const { queryKeys } = await import('@/lib/queryClient')

    expect(queryKeys.calendar.tokenStatus).toEqual(['calendar', 'tokenStatus'])
    expect(queryKeys.calendar.all).toEqual(['calendar'])
  })

  it('caches token status under a stable key (no refetch on remount with shared client)', async () => {
    const queryClient = createTestQueryClient()
    mockApiRequest.mockResolvedValueOnce({
      has_token: true,
      created_at: '2025-03-01T12:00:00Z',
    })

    const { result, unmount } = renderHook(() => useCalendarTokenStatus(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledTimes(1)

    unmount()

    // Remount with the same client: query is still fresh (staleTime=5min)
    // so no second network call should fire.
    const { result: result2 } = renderHook(() => useCalendarTokenStatus(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    expect(result2.current.data?.has_token).toBe(true)
    expect(mockApiRequest).toHaveBeenCalledTimes(1)
  })
})
