import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { createWrapper } from '@/test/utils'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    ADMIN: {
      USERS: {
        LIST: '/admin/users',
      },
    },
  },
  API_BASE_URL: 'http://localhost:8080',
}))

vi.mock('@/lib/queryClient', () => ({
  queryKeys: {
    admin: {
      users: (limit: number, offset: number, search: string) =>
        ['admin', 'users', { limit, offset, search }],
    },
  },
}))

import { useAdminUsers } from './useAdminUsers'


describe('useAdminUsers', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches users with default pagination', async () => {
    const mockResponse = {
      users: [{ id: 1, email: 'user@test.com', username: 'testuser' }],
      total: 1,
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useAdminUsers(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    const url = mockApiRequest.mock.calls[0][0] as string
    expect(url).toContain('/admin/users')
    expect(url).toContain('limit=50')
    expect(url).toContain('offset=0')
    expect(url).not.toContain('search=')
  })

  it('uses custom pagination and search', async () => {
    mockApiRequest.mockResolvedValueOnce({ users: [], total: 0 })

    const { result } = renderHook(
      () => useAdminUsers({ limit: 20, offset: 40, search: 'john' }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    const url = mockApiRequest.mock.calls[0][0] as string
    expect(url).toContain('limit=20')
    expect(url).toContain('offset=40')
    expect(url).toContain('search=john')
  })

  it('URL-encodes spaces in the search filter', async () => {
    // Search-box copies frequently contain spaces (e.g. "alice doe") —
    // make sure URLSearchParams produces a backend-parseable URL rather
    // than a raw space that breaks the query.
    mockApiRequest.mockResolvedValueOnce({ users: [], total: 0 })

    const { result } = renderHook(
      () => useAdminUsers({ search: 'alice doe' }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    const url = mockApiRequest.mock.calls[0][0] as string
    expect(url).toMatch(/search=alice(\+|%20)doe/)
  })

  it('starts in loading state before resolving', async () => {
    let resolveFetch: (value: unknown) => void = () => {}
    const pending = new Promise((resolve) => {
      resolveFetch = resolve
    })
    mockApiRequest.mockReturnValueOnce(pending)

    const { result } = renderHook(() => useAdminUsers(), {
      wrapper: createWrapper(),
    })

    expect(result.current.isLoading).toBe(true)

    resolveFetch({ users: [], total: 0 })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
  })

  it('surfaces fetch errors', async () => {
    mockApiRequest.mockRejectedValueOnce(new Error('Forbidden'))

    const { result } = renderHook(() => useAdminUsers(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect((result.current.error as Error).message).toBe('Forbidden')
  })
})
