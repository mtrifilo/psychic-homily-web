import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { createWrapper } from '@/test/utils'

const { mockApiRequest } = vi.hoisted(() => ({ mockApiRequest: vi.fn() }))

vi.mock('@/lib/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api')
  return {
    ...actual,
    apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  }
})

let mockAuthStatus: 'pending' | 'authenticated' | 'anonymous' = 'authenticated'

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({
    authStatus: mockAuthStatus,
    isAuthenticated: mockAuthStatus === 'authenticated',
    user: mockAuthStatus === 'authenticated' ? { id: 42 } : undefined,
  }),
}))

import { useUserFollowStatus } from './useUserFollow'

describe('useUserFollowStatus', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockAuthStatus = 'authenticated'
  })

  // Invariant: the viewer-less key `[follows, user, username, null]` holds only
  // anonymous data.
  //
  // `viewerId` is undefined whenever `isAuthenticated` is false, which includes
  // the window before a signed-in viewer's profile lands. A fetch issued then
  // carries their cookie, so the response is THEIR follow state written under
  // the shared viewer-less key, which the follow/unfollow mutations also write.
  // Session expiry clears no cache, so it would outlive the session.
  it('does not fetch while auth is unsettled', () => {
    mockAuthStatus = 'pending'

    const { result } = renderHook(() => useUserFollowStatus('alice'), {
      wrapper: createWrapper(),
    })

    expect(result.current.fetchStatus).toBe('idle')
    expect(mockApiRequest).not.toHaveBeenCalled()
  })

  it('fetches for a signed-in viewer', async () => {
    mockApiRequest.mockResolvedValueOnce({
      username: 'alice',
      follower_count: 3,
      is_following: true,
    })

    const { result } = renderHook(() => useUserFollowStatus('alice'), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      expect.stringContaining('/users/alice/followers'),
      { method: 'GET' }
    )
  })

  it('honors a caller opting out', () => {
    const { result } = renderHook(() => useUserFollowStatus('alice', false), {
      wrapper: createWrapper(),
    })

    expect(result.current.fetchStatus).toBe('idle')
    expect(mockApiRequest).not.toHaveBeenCalled()
  })

  it('does not fetch without a username', () => {
    const { result } = renderHook(() => useUserFollowStatus(''), {
      wrapper: createWrapper(),
    })

    expect(result.current.fetchStatus).toBe('idle')
    expect(mockApiRequest).not.toHaveBeenCalled()
  })
})
