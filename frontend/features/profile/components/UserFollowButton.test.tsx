import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { screen, waitFor, act } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient } from '@tanstack/react-query'
import { renderWithProviders } from '@/test/utils'
import { queryKeys } from '@/lib/queryClient'
import type { UserFollowStatus } from '@/lib/types/follow'
import type { PublicProfileResponse } from '@/features/auth'

const mockPush = vi.fn()
let mockIsAuthenticated = true
let mockUserId: number | undefined = 42

vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush }),
  usePathname: () => '/users/alice',
}))

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({
    isAuthenticated: mockIsAuthenticated,
    user: mockIsAuthenticated ? { id: mockUserId, username: 'bob' } : null,
  }),
}))

const { mockApiRequest } = vi.hoisted(() => ({
  mockApiRequest: vi.fn(),
}))

vi.mock('@/lib/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api')
  return {
    ...actual,
    apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  }
})

const useMockedHooks = vi.hoisted(() => ({ value: true }))
const mockFollowMutate = vi.fn()
const mockUnfollowMutate = vi.fn()
let mockStatusData: UserFollowStatus | undefined
let mockStatusLoading = false
let mockFollowPending = false
let mockUnfollowPending = false

vi.mock('@/lib/hooks/common/useUserFollow', async () => {
  const actual = await vi.importActual<
    typeof import('@/lib/hooks/common/useUserFollow')
  >('@/lib/hooks/common/useUserFollow')
  return {
    ...actual,
    useUserFollowStatus: (username: string, enabled = true) =>
      useMockedHooks.value
        ? { data: mockStatusData, isLoading: mockStatusLoading }
        : actual.useUserFollowStatus(username, enabled),
    useUserFollow: () =>
      useMockedHooks.value
        ? { mutate: mockFollowMutate, isPending: mockFollowPending }
        : actual.useUserFollow(),
    useUserUnfollow: () =>
      useMockedHooks.value
        ? { mutate: mockUnfollowMutate, isPending: mockUnfollowPending }
        : actual.useUserUnfollow(),
  }
})

import { UserFollowButton } from './UserFollowButton'

describe('UserFollowButton', () => {
  beforeEach(() => {
    window.history.replaceState({}, '', '/users/alice')
    vi.clearAllMocks()
    useMockedHooks.value = true
    mockIsAuthenticated = true
    mockUserId = 42
    mockStatusData = {
      username: 'alice',
      follower_count: 3,
      is_following: false,
    }
    mockStatusLoading = false
    mockFollowPending = false
    mockUnfollowPending = false
  })

  it('renders Follow when not following', () => {
    renderWithProviders(<UserFollowButton username="alice" />)
    expect(
      screen.getByRole('button', { name: /^follow$/i })
    ).toBeInTheDocument()
  })

  it('renders Following when already following', () => {
    mockStatusData = {
      username: 'alice',
      follower_count: 4,
      is_following: true,
    }
    renderWithProviders(<UserFollowButton username="alice" />)
    expect(
      screen.getByRole('button', { name: /^following$/i })
    ).toBeInTheDocument()
  })

  it('calls follow mutate on click when not following', async () => {
    const user = userEvent.setup()
    renderWithProviders(<UserFollowButton username="alice" />)
    await user.click(screen.getByRole('button', { name: /^follow$/i }))
    expect(mockFollowMutate).toHaveBeenCalledWith('alice', expect.any(Object))
    expect(mockUnfollowMutate).not.toHaveBeenCalled()
  })

  it('calls unfollow mutate on click when following', async () => {
    const user = userEvent.setup()
    mockStatusData = {
      username: 'alice',
      follower_count: 4,
      is_following: true,
    }
    renderWithProviders(<UserFollowButton username="alice" />)
    await user.click(screen.getByRole('button', { name: /^following$/i }))
    expect(mockUnfollowMutate).toHaveBeenCalledWith(
      'alice',
      expect.any(Object)
    )
  })

  it('redirects to /auth when logged out', async () => {
    const user = userEvent.setup()
    mockIsAuthenticated = false
    window.history.replaceState({}, '', '/users/alice?tab=bio')

    renderWithProviders(<UserFollowButton username="alice" />)
    await user.click(screen.getByRole('button', { name: /sign in to follow/i }))

    expect(mockPush).toHaveBeenCalledWith(
      '/auth?returnTo=%2Fusers%2Falice%3Ftab%3Dbio'
    )
    expect(mockFollowMutate).not.toHaveBeenCalled()
  })

  it('disables while a mutation is pending', () => {
    mockFollowPending = true
    renderWithProviders(<UserFollowButton username="alice" />)
    expect(screen.getByRole('button', { name: /^follow$/i })).toBeDisabled()
  })

  it('shows inline error when follow mutate fails', async () => {
    const user = userEvent.setup()
    mockFollowMutate.mockImplementation(
      (_username: string, opts?: { onError?: () => void }) => {
        opts?.onError?.()
      }
    )
    renderWithProviders(<UserFollowButton username="alice" />)
    await user.click(screen.getByRole('button', { name: /^follow$/i }))
    expect(screen.getByText('Failed to follow')).toBeInTheDocument()
  })

  describe('optimistic cache (real hooks)', () => {
    let queryClient: QueryClient

    beforeEach(() => {
      useMockedHooks.value = false
      queryClient = new QueryClient({
        defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
      })
      queryClient.setQueryData<UserFollowStatus>(
        queryKeys.follows.user('alice', 42),
        {
          username: 'alice',
          follower_count: 3,
          is_following: false,
        }
      )
      queryClient.setQueryData<PublicProfileResponse>(
        queryKeys.contributor.profile('alice'),
        {
          username: 'alice',
          profile_visibility: 'public',
          user_tier: 'contributor',
          joined_at: '2025-01-01T00:00:00Z',
          stats: {
            shows_submitted: 0,
            venues_submitted: 0,
            venue_edits_submitted: 0,
            releases_created: 0,
            labels_created: 0,
            festivals_created: 0,
            artists_edited: 0,
            revisions_made: 0,
            pending_edits_submitted: 0,
            tag_votes_cast: 0,
            relationship_votes_cast: 0,
            request_votes_cast: 0,
            collection_items_added: 0,
            collection_subscriptions: 0,
            reports_filed: 0,
            reports_resolved: 0,
            followers_count: 3,
            following_count: 0,
            moderation_actions: 0,
            total_contributions: 0,
          },
        }
      )
    })

    afterEach(() => {
      queryClient.clear()
    })

    it('optimistically sets following and bumps followers_count', async () => {
      const user = userEvent.setup()
      let resolveFollow!: (v: { success: boolean; message: string }) => void
      mockApiRequest.mockImplementation((url: string, opts?: { method?: string }) => {
        if (
          typeof url === 'string' &&
          url.includes('/users/alice/follow') &&
          opts?.method === 'POST'
        ) {
          return new Promise(resolve => {
            resolveFollow = resolve
          })
        }
        if (
          typeof url === 'string' &&
          url.includes('/users/alice/followers')
        ) {
          return Promise.resolve({
            username: 'alice',
            follower_count: 3,
            is_following: false,
          })
        }
        return Promise.reject(new Error(`unexpected request: ${url}`))
      })

      renderWithProviders(<UserFollowButton username="alice" />, {
        queryClient,
      })

      await user.click(screen.getByRole('button', { name: /^follow$/i }))

      await waitFor(() => {
        const status = queryClient.getQueryData<UserFollowStatus>(
          queryKeys.follows.user('alice', 42)
        )
        expect(status?.is_following).toBe(true)
        expect(status?.follower_count).toBe(4)
      })

      const profile = queryClient.getQueryData<PublicProfileResponse>(
        queryKeys.contributor.profile('alice')
      )
      expect(profile?.stats?.followers_count).toBe(4)

      await act(async () => {
        resolveFollow({ success: true, message: 'ok' })
      })
    })

    it('rolls back follow status on error', async () => {
      const user = userEvent.setup()
      mockApiRequest.mockImplementation((url: string, opts?: { method?: string }) => {
        if (
          typeof url === 'string' &&
          url.includes('/users/alice/follow') &&
          opts?.method === 'POST'
        ) {
          return Promise.reject(new Error('boom'))
        }
        if (
          typeof url === 'string' &&
          url.includes('/users/alice/followers')
        ) {
          return Promise.resolve({
            username: 'alice',
            follower_count: 3,
            is_following: false,
          })
        }
        return Promise.reject(new Error(`unexpected request: ${url}`))
      })

      renderWithProviders(<UserFollowButton username="alice" />, {
        queryClient,
      })

      await user.click(screen.getByRole('button', { name: /^follow$/i }))

      await waitFor(() => {
        expect(screen.getByText('Failed to follow')).toBeInTheDocument()
      })

      const status = queryClient.getQueryData<UserFollowStatus>(
        queryKeys.follows.user('alice', 42)
      )
      expect(status?.is_following).toBe(false)
      expect(status?.follower_count).toBe(3)
    })
  })
})
