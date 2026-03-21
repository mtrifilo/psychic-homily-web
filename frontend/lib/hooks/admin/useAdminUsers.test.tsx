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

  it('handles API errors', async () => {
    const error = new Error('Forbidden')
    Object.assign(error, { status: 403 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useAdminUsers(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
  })
})
