import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createTestQueryClient } from '@/test/utils'
import { AuthError, AuthErrorCode } from '@/lib/errors'

// Mock the auth hooks
const mockUseProfile = vi.fn()
const mockUseLogout = vi.fn()

vi.mock('@/features/auth', () => ({
  useProfile: () => mockUseProfile(),
  useLogout: () => mockUseLogout(),
}))

// Import after mocks are set up
import { AuthProvider, useAuthContext } from './AuthContext'

// Helper to create wrapper with specific query client
function createWrapperWithClient(queryClient: QueryClient) {
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>
        <AuthProvider>{children}</AuthProvider>
      </QueryClientProvider>
    )
  }
}

describe('AuthContext', () => {
  let queryClient: QueryClient
  let mockMutateAsync: ReturnType<typeof vi.fn>

  beforeEach(() => {
    vi.clearAllMocks()
    queryClient = createTestQueryClient()
    mockMutateAsync = vi.fn()

    // Default mock implementations
    // `isPending` is stated explicitly rather than left undefined: it is what
    // `authStatus` derives from, so an absent value would make every test in
    // this file depend on undefined being falsy.
    mockUseProfile.mockReturnValue({
      data: null,
      isPending: false,
      isLoading: false,
      error: null,
    })

    mockUseLogout.mockReturnValue({
      mutateAsync: mockMutateAsync,
      isPending: false,
    })
  })

  describe('AuthProvider', () => {
    it('provides auth context to children', () => {
      const { result } = renderHook(() => useAuthContext(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      expect(result.current).toBeDefined()
      expect(result.current.user).toBeNull()
      expect(result.current.isAuthenticated).toBe(false)
    })

    it('derives user from profile data when successful', () => {
      mockUseProfile.mockReturnValue({
        data: {
          success: true,
          user: {
            id: 'user-123',
            email: 'test@example.com',
            first_name: 'John',
            last_name: 'Doe',
            email_verified: true,
            is_admin: false,
          },
        },
        isLoading: false,
        error: null,
      })

      const { result } = renderHook(() => useAuthContext(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      expect(result.current.user).toEqual({
        id: 'user-123',
        email: 'test@example.com',
        first_name: 'John',
        last_name: 'Doe',
        email_verified: true,
        is_admin: false,
      })
      expect(result.current.isAuthenticated).toBe(true)
    })

    it('maps avatar_url from profile data (PSY-1488)', () => {
      mockUseProfile.mockReturnValue({
        data: {
          success: true,
          user: {
            id: 'user-123',
            email: 'test@example.com',
            email_verified: true,
            avatar_url: 'https://example.com/oauth-avatar.jpg',
          },
        },
        isLoading: false,
        error: null,
      })

      const { result } = renderHook(() => useAuthContext(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      expect(result.current.user?.avatar_url).toBe(
        'https://example.com/oauth-avatar.jpg'
      )
    })

    it('returns null user when profile is not successful', () => {
      mockUseProfile.mockReturnValue({
        data: {
          success: false,
          message: 'Not authenticated',
        },
        isLoading: false,
        error: null,
      })

      const { result } = renderHook(() => useAuthContext(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      expect(result.current.user).toBeNull()
      expect(result.current.isAuthenticated).toBe(false)
    })

    it('handles admin users', () => {
      mockUseProfile.mockReturnValue({
        data: {
          success: true,
          user: {
            id: 'admin-123',
            email: 'admin@example.com',
            email_verified: true,
            is_admin: true,
          },
        },
        isLoading: false,
        error: null,
      })

      const { result } = renderHook(() => useAuthContext(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      expect(result.current.user?.is_admin).toBe(true)
    })

    it('defaults email_verified to false when not provided', () => {
      mockUseProfile.mockReturnValue({
        data: {
          success: true,
          user: {
            id: 'user-123',
            email: 'test@example.com',
          },
        },
        isLoading: false,
        error: null,
      })

      const { result } = renderHook(() => useAuthContext(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      expect(result.current.user?.email_verified).toBe(false)
    })
  })

  describe('isLoading state', () => {
    it('returns isLoading true when profile is loading', () => {
      mockUseProfile.mockReturnValue({
        data: null,
        isLoading: true,
        error: null,
      })

      const { result } = renderHook(() => useAuthContext(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      expect(result.current.isLoading).toBe(true)
    })

    it('returns isLoading true when logout is pending', () => {
      mockUseLogout.mockReturnValue({
        mutateAsync: mockMutateAsync,
        isPending: true,
      })

      const { result } = renderHook(() => useAuthContext(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      expect(result.current.isLoading).toBe(true)
    })

    it('returns isLoading false when neither loading nor pending', () => {
      mockUseProfile.mockReturnValue({
        data: { success: true, user: { id: '1', email: 'test@test.com' } },
        isLoading: false,
        error: null,
      })

      const { result } = renderHook(() => useAuthContext(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      expect(result.current.isLoading).toBe(false)
    })
  })

  describe('error handling', () => {
    it('returns error from profile fetch failure', () => {
      const profileError = new Error('Profile fetch failed')
      mockUseProfile.mockReturnValue({
        data: null,
        isLoading: false,
        error: profileError,
      })

      const { result } = renderHook(() => useAuthContext(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      expect(result.current.error).toBe('Profile fetch failed')
    })

    it('returns default error message when error has no message', () => {
      mockUseProfile.mockReturnValue({
        data: null,
        isLoading: false,
        error: {},
      })

      const { result } = renderHook(() => useAuthContext(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      expect(result.current.error).toBe('Authentication failed')
    })

    it('returns null error when no error', () => {
      const { result } = renderHook(() => useAuthContext(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      expect(result.current.error).toBeNull()
    })
  })

  describe('setUser', () => {
    it('allows manual user override', () => {
      const { result } = renderHook(() => useAuthContext(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      act(() => {
        result.current.setUser({
          id: 'override-123',
          email: 'override@example.com',
          email_verified: true,
        })
      })

      expect(result.current.user?.id).toBe('override-123')
      expect(result.current.isAuthenticated).toBe(true)
    })

    it('user override takes precedence over profile data', () => {
      mockUseProfile.mockReturnValue({
        data: {
          success: true,
          user: {
            id: 'profile-user',
            email: 'profile@example.com',
            email_verified: true,
          },
        },
        isLoading: false,
        error: null,
      })

      const { result } = renderHook(() => useAuthContext(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      act(() => {
        result.current.setUser({
          id: 'override-user',
          email: 'override@example.com',
          email_verified: true,
        })
      })

      expect(result.current.user?.id).toBe('override-user')
    })

    it('backfills nav_mode from the profile when the login override omits it (PSY-1117)', () => {
      mockUseProfile.mockReturnValue({
        data: {
          success: true,
          user: {
            id: 'profile-user',
            email: 'profile@example.com',
            email_verified: true,
            nav_mode: 'side',
          },
        },
        isLoading: false,
        error: null,
      })

      const { result } = renderHook(() => useAuthContext(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      // Login overrides carry identity but not nav_mode (the auth response
      // omits it).
      act(() => {
        result.current.setUser({
          id: 'override-user',
          email: 'override@example.com',
          email_verified: true,
        })
      })

      // Identity comes from the override; nav_mode is backfilled from the
      // profile so the appearance control seeds from the saved preference
      // rather than the default within the SPA session.
      expect(result.current.user?.id).toBe('override-user')
      expect(result.current.user?.nav_mode).toBe('side')
    })

    it('keeps the override nav_mode when the override sets one (override wins)', () => {
      mockUseProfile.mockReturnValue({
        data: {
          success: true,
          user: {
            id: 'profile-user',
            email: 'profile@example.com',
            email_verified: true,
            nav_mode: 'top',
          },
        },
        isLoading: false,
        error: null,
      })

      const { result } = renderHook(() => useAuthContext(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      act(() => {
        result.current.setUser({
          id: 'override-user',
          email: 'override@example.com',
          email_verified: true,
          nav_mode: 'side',
        })
      })

      expect(result.current.user?.nav_mode).toBe('side')
    })
  })

  describe('setError and clearError', () => {
    it('allows manual error override', () => {
      const { result } = renderHook(() => useAuthContext(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      act(() => {
        result.current.setError('Custom error message')
      })

      expect(result.current.error).toBe('Custom error message')
    })

    it('error override takes precedence over profile error', () => {
      mockUseProfile.mockReturnValue({
        data: null,
        isLoading: false,
        error: new Error('Profile error'),
      })

      const { result } = renderHook(() => useAuthContext(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      act(() => {
        result.current.setError('Override error')
      })

      expect(result.current.error).toBe('Override error')
    })

    it('clearError clears the error override', () => {
      const { result } = renderHook(() => useAuthContext(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      act(() => {
        result.current.setError('Some error')
      })

      expect(result.current.error).toBe('Some error')

      act(() => {
        result.current.clearError()
      })

      expect(result.current.error).toBeNull()
    })
  })

  describe('logout', () => {
    it('calls logout mutation', async () => {
      mockMutateAsync.mockResolvedValueOnce({})

      const { result } = renderHook(() => useAuthContext(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      await act(async () => {
        result.current.logout()
      })

      expect(mockMutateAsync).toHaveBeenCalled()
    })

    it('clears user override on logout', async () => {
      mockMutateAsync.mockResolvedValueOnce({})

      const { result } = renderHook(() => useAuthContext(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      // Set a user override
      act(() => {
        result.current.setUser({
          id: 'test',
          email: 'test@test.com',
          email_verified: true,
        })
      })

      expect(result.current.user).not.toBeNull()

      // Logout
      await act(async () => {
        result.current.logout()
      })

      // User override should be cleared — with no profile data, user is null
      expect(result.current.user).toBeNull()
      expect(result.current.isAuthenticated).toBe(false)
    })

    it('clears error override on logout', async () => {
      mockMutateAsync.mockResolvedValueOnce({})

      const { result } = renderHook(() => useAuthContext(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      // Set an error override
      act(() => {
        result.current.setError('Some error')
      })

      // Logout
      await act(async () => {
        result.current.logout()
      })

      expect(result.current.error).toBeNull()
    })

    it('handles logout failure gracefully (does not throw)', async () => {
      mockMutateAsync.mockRejectedValueOnce(new Error('Network error'))

      const { result } = renderHook(() => useAuthContext(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      // Should not throw even when mutation fails
      await act(async () => {
        result.current.logout()
      })

      // Logout still clears state even on failure
      expect(mockMutateAsync).toHaveBeenCalled()
    })
  })

  // ── authStatus: the settled-auth signal (PSY-1867) ──────────────────
  //
  // The point of this tri-state is that consumers can tell "we do not know who
  // this viewer is yet" apart from "this viewer is nobody". `isAuthenticated`
  // and `isLoading` both collapse those two, which is what made the PSY-1686
  // anonymous-skip unsafe. Each case below pins one leg of that separation.
  describe('authStatus', () => {
    it('is "pending" while the profile query has not resolved', () => {
      mockUseProfile.mockReturnValue({
        data: undefined,
        isPending: true,
        // Deliberately false: this is the exact shape that misled PSY-1686 —
        // the query is mounted but has not started fetching, so `isLoading`
        // (isPending && isFetching) reads false while the answer is unknown.
        isLoading: false,
        error: null,
      })

      const { result } = renderHook(() => useAuthContext(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      expect(result.current.authStatus).toBe('pending')
      expect(result.current.isAuthenticated).toBe(false)
    })

    it('is "authenticated" once the profile resolves with a user', () => {
      mockUseProfile.mockReturnValue({
        data: {
          success: true,
          user: {
            id: 'user-123',
            email: 'test@example.com',
            email_verified: true,
          },
        },
        isPending: false,
        isLoading: false,
        error: null,
      })

      const { result } = renderHook(() => useAuthContext(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      expect(result.current.authStatus).toBe('authenticated')
      expect(result.current.isAuthenticated).toBe(true)
    })

    it('is "anonymous" only once the profile resolves without a user', () => {
      mockUseProfile.mockReturnValue({
        data: { success: false, message: 'Authentication required' },
        isPending: false,
        isLoading: false,
        error: null,
      })

      const { result } = renderHook(() => useAuthContext(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      expect(result.current.authStatus).toBe('anonymous')
    })

    // REVERSAL of an earlier pin in this same file, recorded deliberately.
    //
    // The first version of this test asserted `authStatus === 'anonymous'` for
    // ANY profile error, using a 401 as the example. Adversarial review showed
    // that pin encoded a forgeable state: the branch it protected could not
    // tell a 401 from a 503, so a transient backend failure settled a signed-in
    // viewer to 'anonymous', and with `refetchOnWindowFocus` off in
    // production and `AuthProvider` mounted once in the root layout, it stayed
    // that way for the whole SPA session. The ticket's own acceptance criterion
    // says no gate may act while auth is unsettled, and a failure that is not
    // an answer IS unsettled, so the correct reading of a non-definitive error
    // is 'pending'. The two cases are split below.
    it('is "anonymous" when the profile query fails DEFINITIVELY (401)', () => {
      mockUseProfile.mockReturnValue({
        data: undefined,
        isPending: false,
        isLoading: false,
        error: new AuthError(
          'Authentication required',
          AuthErrorCode.TOKEN_MISSING,
          { status: 401 }
        ),
      })

      const { result } = renderHook(() => useAuthContext(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      expect(result.current.authStatus).toBe('anonymous')
    })

    it('is "pending", NOT "anonymous", when the profile query fails indefinitely (5xx)', () => {
      // A backend outage is not the backend saying "nobody". Reading it as
      // anonymous is what let one 5xx hand a signed-in viewer an enabled
      // bracket whose replayed pre-hydration click bounces them to /auth.
      mockUseProfile.mockReturnValue({
        data: undefined,
        isPending: false,
        isLoading: false,
        error: new AuthError(
          'Service unavailable',
          AuthErrorCode.SERVICE_UNAVAILABLE,
          { status: 503 }
        ),
      })

      const { result } = renderHook(() => useAuthContext(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      expect(result.current.authStatus).toBe('pending')
      expect(result.current.isAuthenticated).toBe(false)
    })

    // The trap that classifying by error CODE alone would have walked into.
    // `apiRequest` falls back to a generic UNAUTHORIZED code when a 401 body
    // carries no `error_code`, and `shouldRedirectToLogin` does not cover that
    // code. Reading it as non-definitive would leave a genuinely anonymous
    // viewer at 'pending' forever, with a permanently disabled bracket, on any
    // deployment whose 401 body differs from today's.
    it('is "anonymous" for a 401 that carries no recognizable auth error code', () => {
      mockUseProfile.mockReturnValue({
        data: undefined,
        isPending: false,
        isLoading: false,
        error: new AuthError(
          'Authentication failed',
          AuthErrorCode.UNAUTHORIZED,
          { status: 401 }
        ),
      })

      const { result } = renderHook(() => useAuthContext(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      expect(result.current.authStatus).toBe('anonymous')
    })

    // The two cells where `data` and `error` are present SIMULTANEOUSLY.
    // TanStack retains the last successful `data` when a refetch fails, so this
    // is not a contrived shape: it is what every failed background refetch of a
    // resolved profile looks like. Both directions were unpinned, and both were
    // wrong in the code until the panel found them.
    it('is "anonymous", not "authenticated", when a signed-in profile is followed by a 401', () => {
      // Session expired mid-visit. Trusting the retained payload would report
      // the viewer as signed in after the backend said otherwise, leaving every
      // control enabled and every click 401ing with nothing to explain it.
      mockUseProfile.mockReturnValue({
        data: {
          success: true,
          user: { id: 'user-123', email: 'test@example.com' },
        },
        isPending: false,
        isLoading: false,
        error: new AuthError('Token expired', AuthErrorCode.TOKEN_EXPIRED, {
          status: 401,
        }),
      })

      const { result } = renderHook(() => useAuthContext(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      expect(result.current.authStatus).toBe('anonymous')
      expect(result.current.isAuthenticated).toBe(false)
      expect(result.current.user).toBeNull()
    })

    // The override survives an expiry (nothing clears it but logout), so it has
    // to yield to a definitive failure the same way retained data does.
    // Otherwise anyone who signed in during this SPA session is exempt from the
    // demotion above, which is the more common signed-in population.
    it('is "anonymous" when a login override is followed by a 401', () => {
      mockUseProfile.mockReturnValue({
        data: undefined,
        isPending: false,
        isLoading: false,
        error: null,
      })

      const { result } = renderHook(() => useAuthContext(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      act(() => {
        result.current.setUser({
          id: 'user-123',
          email: 'test@example.com',
          email_verified: true,
        })
      })
      expect(result.current.authStatus).toBe('authenticated')

      // Session ends elsewhere; this tab's next profile read 401s.
      mockUseProfile.mockReturnValue({
        data: undefined,
        isPending: false,
        isLoading: false,
        error: new AuthError('Token expired', AuthErrorCode.TOKEN_EXPIRED, {
          status: 401,
        }),
      })

      const { result: after } = renderHook(() => useAuthContext(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      expect(after.current.authStatus).toBe('anonymous')
      expect(after.current.user).toBeNull()
    })

    it('stays "anonymous" when a settled anonymous profile is followed by a 5xx', () => {
      // The mirror case. A resolved payload is an ANSWER, and a failed
      // background refetch does not un-answer it. Sliding back to 'pending'
      // here would re-enable the very request this ticket skips and disable
      // every Follow control sitewide, at the moment the backend is already
      // failing.
      mockUseProfile.mockReturnValue({
        data: { success: false, message: 'Authentication required' },
        isPending: false,
        isLoading: false,
        error: new AuthError('Bad gateway', AuthErrorCode.UNKNOWN, {
          status: 502,
        }),
      })

      const { result } = renderHook(() => useAuthContext(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      expect(result.current.authStatus).toBe('anonymous')
    })

    it('is "pending" when the profile query fails with a non-auth error', () => {
      // Network failure / DNS / abort: `AuthError.fromUnknown` gives UNKNOWN,
      // which is not definitive, so the conservative reading applies.
      mockUseProfile.mockReturnValue({
        data: undefined,
        isPending: false,
        isLoading: false,
        error: new Error('Failed to fetch'),
      })

      const { result } = renderHook(() => useAuthContext(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      expect(result.current.authStatus).toBe('pending')
    })

    it('stays "authenticated" through a background refetch of a cached profile', () => {
      // A stale-time refetch re-enters fetching with data already in hand.
      // Reporting 'pending' here would re-disable settled controls on every
      // window focus.
      mockUseProfile.mockReturnValue({
        data: { success: true, user: { id: 'user-123', email: 'a@b.c' } },
        isPending: false,
        isLoading: false,
        isFetching: true,
        error: null,
      })

      const { result } = renderHook(() => useAuthContext(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      expect(result.current.authStatus).toBe('authenticated')
    })

    it('is "pending", never "anonymous", while a logout is in flight', () => {
      // Pins the logoutMutation clause IN ISOLATION, with no cached profile to
      // fall back on, which is deliberately not the shape an ordinary logout
      // produces. There, `user` survives on the cached profile until
      // `queryClient.clear()`, after which `isProfilePending` covers the gap by
      // itself; see the numbered note beside `authStatus` in AuthContext.tsx.
      // What this asserts is the invariant that survives either ordering: an
      // in-flight logout never reads as a SETTLED anonymous viewer, so nothing
      // gated on 'anonymous' can act a beat early and flip twice.
      mockUseProfile.mockReturnValue({
        data: undefined,
        isPending: false,
        isLoading: false,
        error: null,
      })
      mockUseLogout.mockReturnValue({
        mutateAsync: mockMutateAsync,
        isPending: true,
      })

      const { result } = renderHook(() => useAuthContext(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      expect(result.current.authStatus).toBe('pending')
    })

    it('is "authenticated" from a login override before the profile refetch lands', () => {
      // Login sets the user override, then refetches the profile. The override
      // is already an answer; re-opening the pending window would disable
      // controls the viewer just earned.
      mockUseProfile.mockReturnValue({
        data: undefined,
        isPending: true,
        isLoading: true,
        error: null,
      })

      const { result } = renderHook(() => useAuthContext(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      expect(result.current.authStatus).toBe('pending')

      act(() => {
        result.current.setUser({
          id: 'user-123',
          email: 'test@example.com',
          email_verified: true,
        })
      })

      expect(result.current.authStatus).toBe('authenticated')
      expect(result.current.isAuthenticated).toBe(true)
    })
  })

  describe('useAuthContext outside provider', () => {
    it('throws error when used outside AuthProvider', () => {
      // Use a wrapper without AuthProvider
      const wrapper = ({ children }: { children: React.ReactNode }) => (
        <QueryClientProvider client={queryClient}>
          {children}
        </QueryClientProvider>
      )

      expect(() => {
        renderHook(() => useAuthContext(), { wrapper })
      }).toThrow('useAuthContext must be used within an AuthProvider')
    })
  })
})
