import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createWrapper, createTestQueryClient } from '@/test/utils'

// Create mocks
const mockApiRequest = vi.fn()

// Mock the api module
vi.mock('../../api', () => ({
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

// Mock queryClient module
vi.mock('../../queryClient', () => ({
  queryKeys: {
    admin: {
      users: (limit: number, offset: number, search: string) => [
        'admin',
        'users',
        { limit, offset, search },
      ],
    },
  },
}))

// Import hooks after mocks are set up
import { useAdminUsers } from './useAdminUsers'

describe('useAdminUsers', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches users with default options', async () => {
    const mockResponse = {
      users: [
        {
          id: 1,
          email: 'user1@example.com',
          username: 'user1',
          first_name: 'Alice',
          last_name: 'Smith',
          avatar_url: null,
          is_active: true,
          is_admin: false,
          email_verified: true,
          auth_methods: ['email'],
          submission_stats: {
            approved: 5,
            pending: 1,
            rejected: 0,
            total: 6,
          },
          created_at: '2026-01-15T10:00:00Z',
        },
        {
          id: 2,
          email: 'admin@example.com',
          username: 'admin',
          first_name: 'Bob',
          last_name: 'Jones',
          avatar_url: 'https://example.com/avatar.jpg',
          is_active: true,
          is_admin: true,
          email_verified: true,
          auth_methods: ['email', 'google'],
          submission_stats: {
            approved: 50,
            pending: 2,
            rejected: 3,
            total: 55,
          },
          created_at: '2025-12-01T08:00:00Z',
        },
      ],
      total: 2,
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useAdminUsers(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/admin/users?limit=50&offset=0',
      { method: 'GET' }
    )
    expect(result.current.data?.users).toHaveLength(2)
    expect(result.current.data?.total).toBe(2)
  })

  it('supports custom limit and offset', async () => {
    mockApiRequest.mockResolvedValueOnce({ users: [], total: 0 })

    const { result } = renderHook(
      () => useAdminUsers({ limit: 25, offset: 50 }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      '/admin/users?limit=25&offset=50',
      { method: 'GET' }
    )
  })

  it('supports search filter', async () => {
    const mockResponse = {
      users: [
        {
          id: 1,
          email: 'alice@example.com',
          username: 'alice',
          first_name: 'Alice',
          last_name: null,
          avatar_url: null,
          is_active: true,
          is_admin: false,
          email_verified: true,
          auth_methods: ['email'],
          submission_stats: {
            approved: 0,
            pending: 0,
            rejected: 0,
            total: 0,
          },
          created_at: '2026-03-01T10:00:00Z',
        },
      ],
      total: 1,
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(
      () => useAdminUsers({ search: 'alice' }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    const calledUrl = mockApiRequest.mock.calls[0][0]
    expect(calledUrl).toContain('search=alice')
    expect(result.current.data?.users).toHaveLength(1)
  })

  it('does not include search param when search is empty', async () => {
    mockApiRequest.mockResolvedValueOnce({ users: [], total: 0 })

    const { result } = renderHook(
      () => useAdminUsers({ search: '' }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    const calledUrl = mockApiRequest.mock.calls[0][0]
    expect(calledUrl).not.toContain('search=')
  })

  it('supports custom limit, offset, and search together', async () => {
    mockApiRequest.mockResolvedValueOnce({ users: [], total: 0 })

    const { result } = renderHook(
      () => useAdminUsers({ limit: 10, offset: 20, search: 'admin' }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    const calledUrl = mockApiRequest.mock.calls[0][0]
    expect(calledUrl).toContain('limit=10')
    expect(calledUrl).toContain('offset=20')
    expect(calledUrl).toContain('search=admin')
  })

  it('handles empty users list', async () => {
    mockApiRequest.mockResolvedValueOnce({ users: [], total: 0 })

    const { result } = renderHook(() => useAdminUsers(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(result.current.data?.users).toHaveLength(0)
    expect(result.current.data?.total).toBe(0)
  })

  it('handles users with nullable fields', async () => {
    const mockResponse = {
      users: [
        {
          id: 3,
          email: null,
          username: null,
          first_name: null,
          last_name: null,
          avatar_url: null,
          is_active: true,
          is_admin: false,
          email_verified: false,
          auth_methods: ['google'],
          submission_stats: {
            approved: 0,
            pending: 0,
            rejected: 0,
            total: 0,
          },
          created_at: '2026-03-19T10:00:00Z',
        },
      ],
      total: 1,
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useAdminUsers(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(result.current.data?.users[0].email).toBeNull()
    expect(result.current.data?.users[0].username).toBeNull()
    expect(result.current.data?.users[0].first_name).toBeNull()
  })

  it('handles deleted users', async () => {
    const mockResponse = {
      users: [
        {
          id: 4,
          email: 'deleted@example.com',
          username: 'deleted_user',
          first_name: 'Deleted',
          last_name: 'User',
          avatar_url: null,
          is_active: false,
          is_admin: false,
          email_verified: true,
          auth_methods: ['email'],
          submission_stats: {
            approved: 10,
            pending: 0,
            rejected: 2,
            total: 12,
          },
          created_at: '2025-06-01T10:00:00Z',
          deleted_at: '2026-03-01T10:00:00Z',
        },
      ],
      total: 1,
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useAdminUsers(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(result.current.data?.users[0].is_active).toBe(false)
    expect(result.current.data?.users[0].deleted_at).toBe(
      '2026-03-01T10:00:00Z'
    )
  })

  it('handles API error', async () => {
    const error = new Error('Server error')
    Object.assign(error, { status: 500 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useAdminUsers(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
  })

  it('handles authentication error', async () => {
    const error = new Error('Forbidden')
    Object.assign(error, { status: 403 })
    mockApiRequest.mockRejectedValueOnce(error)

    const { result } = renderHook(() => useAdminUsers(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect((result.current.error as Error).message).toBe('Forbidden')
  })
})
