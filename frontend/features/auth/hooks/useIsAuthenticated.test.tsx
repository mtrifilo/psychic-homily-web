import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createTestQueryClient } from '@/test/utils'
import { AuthError, AuthErrorCode } from '@/lib/errors'

// Mock the profile query, not this hook's own module: the point of the hook is
// that it re-derives NOTHING, so the assertions have to flow through the real
// AuthProvider to mean anything.
const mockUseProfile = vi.fn()
const mockUseLogout = vi.fn()

vi.mock('@/features/auth/hooks/useAuth', () => ({
  useProfile: () => mockUseProfile(),
  useLogout: () => mockUseLogout(),
}))

import { AuthProvider, useAuthContext } from '@/lib/context/AuthContext'
import { useIsAuthenticated } from './useIsAuthenticated'

function wrapperFor(queryClient: QueryClient) {
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>
        <AuthProvider>{children}</AuthProvider>
      </QueryClientProvider>
    )
  }
}

const SIGNED_IN_PAYLOAD = {
  success: true,
  user: {
    id: 'user-123',
    email: 'test@example.com',
    email_verified: true,
    is_admin: true,
    user_tier: 'trusted_contributor',
  },
}

const ANON_SENTINEL = { success: false, message: 'Not authenticated' }

describe('useIsAuthenticated', () => {
  let queryClient: QueryClient

  beforeEach(() => {
    vi.clearAllMocks()
    queryClient = createTestQueryClient()
    mockUseLogout.mockReturnValue({ mutateAsync: vi.fn(), isPending: false })
  })

  const render = () =>
    renderHook(() => useIsAuthenticated(), { wrapper: wrapperFor(queryClient) })

  // (data present) x (latest response) x (settledness). The two rows this hook
  // used to get wrong are marked; both came from deriving auth locally with
  // `!error` / `isLoading` instead of reading the settled signal.

  it('reads pending before the profile query resolves, even while isLoading is false', () => {
    // The window `isLoading` cannot see: the observer has mounted, the fetch
    // has not started, so `isLoading` (isPending && isFetching) is false while
    // the viewer is still unidentified. THE regression cell: the old predicate
    // reported a settled anonymous viewer here.
    mockUseProfile.mockReturnValue({
      data: undefined,
      isPending: true,
      isLoading: false,
      error: null,
    })

    const { result } = render()

    expect(result.current.authStatus).toBe('pending')
    expect(result.current.isAuthenticated).toBe(false)
    expect(result.current.user).toBeNull()
  })

  it('reads authenticated when the profile resolves with a user', () => {
    mockUseProfile.mockReturnValue({
      data: SIGNED_IN_PAYLOAD,
      isPending: false,
      isLoading: false,
      error: null,
    })

    const { result } = render()

    expect(result.current.authStatus).toBe('authenticated')
    expect(result.current.isAuthenticated).toBe(true)
    expect(result.current.user?.email).toBe('test@example.com')
    // Consumers gate direct-edit brackets on these two fields.
    expect(result.current.user?.is_admin).toBe(true)
    expect(result.current.user?.user_tier).toBe('trusted_contributor')
  })

  it('reads anonymous when the profile resolves with no user', () => {
    mockUseProfile.mockReturnValue({
      data: ANON_SENTINEL,
      isPending: false,
      isLoading: false,
      error: null,
    })

    const { result } = render()

    expect(result.current.authStatus).toBe('anonymous')
    expect(result.current.isAuthenticated).toBe(false)
    expect(result.current.user).toBeNull()
  })

  it('reads anonymous when the profile fails with a 401', () => {
    mockUseProfile.mockReturnValue({
      data: undefined,
      isPending: false,
      isLoading: false,
      error: new AuthError('no session', AuthErrorCode.TOKEN_MISSING, {
        status: 401,
      }),
    })

    const { result } = render()

    expect(result.current.authStatus).toBe('anonymous')
    expect(result.current.isAuthenticated).toBe(false)
  })

  it('stays authenticated when a background refetch fails with a 5xx', () => {
    // A payload already received is an answer; a failed refetch does not revoke
    // it. THE other regression cell: the old predicate ANDed `!error`, so one
    // transient 5xx logged the viewer out of every consumer of this hook while
    // the brackets beside them stayed signed in.
    mockUseProfile.mockReturnValue({
      data: SIGNED_IN_PAYLOAD,
      isPending: false,
      isLoading: false,
      error: new AuthError('boom', AuthErrorCode.UNKNOWN, { status: 500 }),
    })

    const { result } = render()

    expect(result.current.authStatus).toBe('authenticated')
    expect(result.current.isAuthenticated).toBe(true)
  })

  it('reads pending when the profile fails with a 5xx and no payload ever arrived', () => {
    mockUseProfile.mockReturnValue({
      data: undefined,
      isPending: false,
      isLoading: false,
      error: new AuthError('boom', AuthErrorCode.UNKNOWN, { status: 500 }),
    })

    const { result } = render()

    expect(result.current.authStatus).toBe('pending')
    expect(result.current.isAuthenticated).toBe(false)
  })

  it('reads pending when the profile fails with a 403', () => {
    // Forbidden is not "unidentified".
    mockUseProfile.mockReturnValue({
      data: undefined,
      isPending: false,
      isLoading: false,
      error: new AuthError('forbidden', AuthErrorCode.UNKNOWN, { status: 403 }),
    })

    const { result } = render()

    expect(result.current.authStatus).toBe('pending')
    expect(result.current.isAuthenticated).toBe(false)
  })

  // The class defect this hook existed to create: a page could paint anonymous
  // chrome from this predicate next to a bracket disabled by the context's.
  it.each([
    [
      'pending',
      { data: undefined, isPending: true, isLoading: false, error: null },
    ],
    [
      'authenticated',
      {
        data: SIGNED_IN_PAYLOAD,
        isPending: false,
        isLoading: false,
        error: new AuthError('boom', AuthErrorCode.UNKNOWN, { status: 500 }),
      },
    ],
    [
      'anonymous',
      { data: ANON_SENTINEL, isPending: false, isLoading: false, error: null },
    ],
  ])('agrees with useAuthContext in the %s cell', (_label, profileState) => {
    mockUseProfile.mockReturnValue(profileState)

    const { result } = renderHook(
      () => ({ hook: useIsAuthenticated(), context: useAuthContext() }),
      { wrapper: wrapperFor(queryClient) }
    )

    expect(result.current.hook.authStatus).toBe(result.current.context.authStatus)
    expect(result.current.hook.isAuthenticated).toBe(
      result.current.context.isAuthenticated
    )
    expect(result.current.hook.user).toBe(result.current.context.user)
  })
})
