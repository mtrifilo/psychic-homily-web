import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { createWrapper, createWrapperWithClient, createTestQueryClient } from '@/test/utils'
import { AuthErrorCode, AuthError } from '@/lib/errors'

// Create a mock for apiRequest that we can control
const mockApiRequest = vi.fn()
const mockInvalidateAuth = vi.fn()

// Mock the api module
vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    AUTH: {
      LOGIN: '/auth/login',
      LOGOUT: '/auth/logout',
      REGISTER: '/auth/register',
      PROFILE: '/auth/profile',
      REFRESH: '/auth/refresh',
      VERIFY_EMAIL_SEND: '/auth/verify-email/send',
      VERIFY_EMAIL_CONFIRM: '/auth/verify-email/confirm',
      CHANGE_PASSWORD: '/auth/change-password',
      MAGIC_LINK_SEND: '/auth/magic-link/send',
      MAGIC_LINK_VERIFY: '/auth/magic-link/verify',
      DELETION_SUMMARY: '/auth/account/deletion-summary',
      DELETE_ACCOUNT: '/auth/account/delete',
      EXPORT_DATA: '/auth/account/export',
      OAUTH_ACCOUNTS: '/auth/oauth/accounts',
      OAUTH_UNLINK: (provider: string) =>
        `/auth/oauth/accounts/${provider}`,
      RECOVER_ACCOUNT: '/auth/recover-account',
      RECOVER_ACCOUNT_REQUEST: '/auth/recover-account/request',
      RECOVER_ACCOUNT_CONFIRM: '/auth/recover-account/confirm',
      CLI_TOKEN: '/auth/cli-token',
    },
    ADMIN: {
      TOKENS: {
        LIST: '/admin/tokens',
        CREATE: '/admin/tokens',
        REVOKE: (id: number) => `/admin/tokens/${id}`,
      },
    },
  },
  API_BASE_URL: 'http://localhost:8080',
}))

// Mock the auth logger
vi.mock('@/lib/utils/authLogger', () => ({
  authLogger: {
    debug: vi.fn(),
    info: vi.fn(),
    warn: vi.fn(),
    error: vi.fn(),
    loginAttempt: vi.fn(),
    loginSuccess: vi.fn(),
    loginFailed: vi.fn(),
    logout: vi.fn(),
    profileFetch: vi.fn(),
    tokenRefresh: vi.fn(),
  },
}))

// Mock queryClient module
vi.mock('@/lib/queryClient', () => ({
  queryKeys: {
    auth: {
      profile: ['auth', 'profile'],
    },
  },
  createInvalidateQueries: () => ({
    auth: mockInvalidateAuth,
  }),
}))

// Import hooks after mocks are set up
import {
  useLogin,
  useRegister,
  useLogout,
  useProfile,
  useUpdateProfile,
  useRefreshToken,
  useIsAuthenticated,
  useSendVerificationEmail,
  useConfirmVerification,
  useChangePassword,
  useSendMagicLink,
  useVerifyMagicLink,
  useDeleteAccount,
  useExportData,
  useOAuthAccounts,
  useUnlinkOAuthAccount,
  useRecoverAccount,
  useRequestAccountRecovery,
  useConfirmAccountRecovery,
} from './useAuth'


describe('useAuth hooks', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
    mockInvalidateAuth.mockReset()
  })

  describe('useLogin', () => {
    it('calls login endpoint with credentials', async () => {
      mockApiRequest.mockResolvedValueOnce({
        success: true,
        message: 'Login successful',
        user: { id: '1', email: 'test@example.com' },
      })

      const { result } = renderHook(() => useLogin(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({
          email: 'test@example.com',
          password: 'password123',
        })
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/auth/login',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({
            email: 'test@example.com',
            password: 'password123',
          }),
          credentials: 'include',
        })
      )
    })

    it('throws AuthError when login response is not successful', async () => {
      mockApiRequest.mockResolvedValueOnce({
        success: false,
        message: 'Invalid credentials',
        error_code: 'INVALID_CREDENTIALS',
        request_id: 'req-123',
      })

      const { result } = renderHook(() => useLogin(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({
          email: 'test@example.com',
          password: 'wrong',
        })
      })

      await waitFor(() => expect(result.current.isError).toBe(true))

      expect((result.current.error as Error).name).toBe('AuthError')
      expect((result.current.error as unknown as { code: string }).code).toBe(
        AuthErrorCode.INVALID_CREDENTIALS
      )
    })

    it('succeeds on successful login', async () => {
      const queryClient = createTestQueryClient()

      mockApiRequest.mockResolvedValueOnce({
        success: true,
        message: 'Login successful',
        user: { id: '1', email: 'test@example.com' },
      })

      const { result } = renderHook(() => useLogin(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      await act(async () => {
        result.current.mutate({
          email: 'test@example.com',
          password: 'password123',
        })
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))
    })
  })

  describe('useRegister', () => {
    it('calls register endpoint with credentials', async () => {
      mockApiRequest.mockResolvedValueOnce({
        success: true,
        message: 'Registration successful',
        user: { id: '1', email: 'new@example.com' },
      })

      const { result } = renderHook(() => useRegister(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({
          email: 'new@example.com',
          password: 'password123',
          first_name: 'John',
          last_name: 'Doe',
          terms_accepted: true,
          terms_version: '2026-01-31',
          privacy_version: '2026-02-15',
        })
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/auth/register',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({
            email: 'new@example.com',
            password: 'password123',
            first_name: 'John',
            last_name: 'Doe',
            terms_accepted: true,
            terms_version: '2026-01-31',
            privacy_version: '2026-02-15',
          }),
        })
      )
    })

    it('throws AuthError when registration fails', async () => {
      mockApiRequest.mockResolvedValueOnce({
        success: false,
        message: 'Email already exists',
        error_code: 'USER_EXISTS',
      })

      const { result } = renderHook(() => useRegister(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({
          email: 'existing@example.com',
          password: 'password123',
          terms_accepted: true,
          terms_version: '2026-01-31',
          privacy_version: '2026-02-15',
        })
      })

      await waitFor(() => expect(result.current.isError).toBe(true))

      expect((result.current.error as Error).name).toBe('AuthError')
      expect((result.current.error as unknown as { code: string }).code).toBe(
        AuthErrorCode.USER_EXISTS
      )
    })
  })

  describe('useLogout', () => {
    it('calls logout endpoint', async () => {
      mockApiRequest.mockResolvedValueOnce({
        success: true,
        message: 'Logged out',
      })

      const { result } = renderHook(() => useLogout(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate()
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/auth/logout',
        expect.objectContaining({
          method: 'POST',
          credentials: 'include',
        })
      )
    })

    it('clears query cache on success', async () => {
      const queryClient = createTestQueryClient()
      const clearSpy = vi.spyOn(queryClient, 'clear')

      mockApiRequest.mockResolvedValueOnce({
        success: true,
        message: 'Logged out',
      })

      const { result } = renderHook(() => useLogout(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      await act(async () => {
        result.current.mutate()
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(clearSpy).toHaveBeenCalled()
    })

    it('clears query cache even on error', async () => {
      const queryClient = createTestQueryClient()
      const clearSpy = vi.spyOn(queryClient, 'clear')

      mockApiRequest.mockRejectedValueOnce(new Error('Network error'))

      const { result } = renderHook(() => useLogout(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      await act(async () => {
        result.current.mutate()
      })

      await waitFor(() => expect(result.current.isError).toBe(true))

      expect(clearSpy).toHaveBeenCalled()
    })
  })

  describe('useProfile', () => {
    it('fetches user profile', async () => {
      mockApiRequest.mockResolvedValueOnce({
        success: true,
        user: {
          id: '1',
          email: 'test@example.com',
          is_admin: true,
          email_verified: true,
        },
      })

      const { result } = renderHook(() => useProfile(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))
    })

    it('exposes the full user object (preserves is_admin, email_verified, user_tier)', async () => {
      // The header reads `is_admin` to render the admin link, the
      // verification banner reads `email_verified`, and trust-tier badges
      // read `user_tier`. Locking the shape here surfaces any future
      // contract drift in one place.
      const payload = {
        success: true,
        message: 'ok',
        user: {
          id: '1',
          email: 'test@example.com',
          username: 'testuser',
          is_admin: true,
          email_verified: true,
          user_tier: 'trusted_contributor',
          created_at: '2024-01-01T00:00:00Z',
          updated_at: '2025-01-01T00:00:00Z',
        },
      }
      mockApiRequest.mockResolvedValueOnce(payload)

      const { result } = renderHook(() => useProfile(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))
      expect(result.current.data).toEqual(payload)
    })

    it('does NOT retry on token-expired errors (short-circuits to login)', async () => {
      // Retrying on TOKEN_EXPIRED would burn extra refresh attempts and
      // delay the redirect-to-login. The hook explicitly bails on
      // shouldRedirectToLogin.
      const tokenError = new AuthError(
        'Token expired',
        AuthErrorCode.TOKEN_EXPIRED,
        { status: 401 }
      )
      mockApiRequest.mockRejectedValue(tokenError)

      const { result } = renderHook(() => useProfile(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
      expect(mockApiRequest).toHaveBeenCalledTimes(1)
    })

    it('does NOT retry on 403 forbidden', async () => {
      const forbiddenError = new AuthError(
        'Forbidden',
        AuthErrorCode.UNAUTHORIZED,
        { status: 403 }
      )
      mockApiRequest.mockRejectedValue(forbiddenError)

      const { result } = renderHook(() => useProfile(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
      expect(mockApiRequest).toHaveBeenCalledTimes(1)
    })

    it('caches profile across remount (uses 5-minute staleTime)', async () => {
      // The profile is read from multiple places (header, gated buttons,
      // settings) — they should share a single cache entry, not each
      // trigger an independent fetch.
      const queryClient = createTestQueryClient()
      mockApiRequest.mockResolvedValueOnce({
        success: true,
        user: { id: '1', email: 'a@b.c' },
      })

      const { result, unmount } = renderHook(() => useProfile(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))
      expect(mockApiRequest).toHaveBeenCalledTimes(1)

      unmount()

      const { result: result2 } = renderHook(() => useProfile(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      // Cached — still 1 call.
      expect(result2.current.data).toEqual({
        success: true,
        user: { id: '1', email: 'a@b.c' },
      })
      expect(mockApiRequest).toHaveBeenCalledTimes(1)
    })
  })

  describe('useRefreshToken', () => {
    it('calls refresh endpoint', async () => {
      mockApiRequest.mockResolvedValueOnce({
        success: true,
        message: 'Token refreshed',
      })

      const { result } = renderHook(() => useRefreshToken(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate()
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/auth/refresh',
        expect.objectContaining({
          method: 'POST',
          credentials: 'include',
        })
      )
    })

    it('invalidates auth queries on successful refresh (so profile re-fetches)', async () => {
      // PSY-700 named "token refresh on expiry" as a critical scenario.
      // When refresh succeeds, downstream auth queries must re-fetch so
      // the just-renewed session info appears in the UI without a
      // hard refresh.
      mockApiRequest.mockResolvedValueOnce({
        success: true,
        message: 'Token refreshed',
      })

      const { result } = renderHook(() => useRefreshToken(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate()
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))
      expect(mockInvalidateAuth).toHaveBeenCalled()
    })

    it('does NOT invalidate auth on refresh response with success=false', async () => {
      // Backend can return 200 with success=false (e.g. expired refresh
      // token). Invalidating in that case would trigger a stale-data
      // refetch on the soon-to-redirect page.
      mockApiRequest.mockResolvedValueOnce({
        success: false,
        message: 'Refresh token expired',
        error_code: 'TOKEN_EXPIRED',
      })

      const { result } = renderHook(() => useRefreshToken(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate()
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))
      expect(mockInvalidateAuth).not.toHaveBeenCalled()
    })

    it('clears cache on refresh failure', async () => {
      const queryClient = createTestQueryClient()
      const clearSpy = vi.spyOn(queryClient, 'clear')

      mockApiRequest.mockRejectedValueOnce(new Error('Refresh failed'))

      const { result } = renderHook(() => useRefreshToken(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      await act(async () => {
        result.current.mutate()
      })

      await waitFor(() => expect(result.current.isError).toBe(true))

      expect(clearSpy).toHaveBeenCalled()
    })

    it('surfaces refresh error to the caller', async () => {
      const error = new Error('Network error')
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => useRefreshToken(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate()
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
      expect(result.current.error).toBe(error)
    })
  })

  describe('useIsAuthenticated', () => {
    it('returns authenticated true when profile has user', async () => {
      mockApiRequest.mockResolvedValueOnce({
        success: true,
        user: { id: '1', email: 'test@example.com' },
      })

      const { result } = renderHook(() => useIsAuthenticated(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isLoading).toBe(false))

      expect(result.current.isAuthenticated).toBe(true)
      expect(result.current.user?.email).toBe('test@example.com')
    })

    it('returns authenticated false when no user', async () => {
      mockApiRequest.mockResolvedValueOnce({
        success: false,
        message: 'Not authenticated',
      })

      const { result } = renderHook(() => useIsAuthenticated(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isLoading).toBe(false))

      expect(result.current.isAuthenticated).toBe(false)
      expect(result.current.user).toBeUndefined()
    })
  })

  describe('useSendVerificationEmail', () => {
    it('calls send verification endpoint', async () => {
      mockApiRequest.mockResolvedValueOnce({
        success: true,
        message: 'Verification email sent',
      })

      const { result } = renderHook(() => useSendVerificationEmail(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate()
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/auth/verify-email/send',
        expect.objectContaining({
          method: 'POST',
          credentials: 'include',
        })
      )
    })

    it('throws AuthError when sending fails', async () => {
      mockApiRequest.mockResolvedValueOnce({
        success: false,
        message: 'Too many requests',
        error_code: 'VALIDATION_FAILED',
      })

      const { result } = renderHook(() => useSendVerificationEmail(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate()
      })

      await waitFor(() => expect(result.current.isError).toBe(true))

      expect((result.current.error as Error).name).toBe('AuthError')
    })
  })

  describe('useConfirmVerification', () => {
    it('calls confirm verification endpoint with token', async () => {
      mockApiRequest.mockResolvedValueOnce({
        success: true,
        message: 'Email verified',
      })

      const { result } = renderHook(() => useConfirmVerification(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate('verification-token-123')
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/auth/verify-email/confirm',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({ token: 'verification-token-123' }),
          credentials: 'include',
        })
      )
    })

    it('throws AuthError when verification fails', async () => {
      mockApiRequest.mockResolvedValueOnce({
        success: false,
        message: 'Invalid token',
        error_code: 'TOKEN_INVALID',
      })

      const { result } = renderHook(() => useConfirmVerification(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate('bad-token')
      })

      await waitFor(() => expect(result.current.isError).toBe(true))

      expect((result.current.error as Error).name).toBe('AuthError')
    })

    it('succeeds on successful verification', async () => {
      const queryClient = createTestQueryClient()

      mockApiRequest.mockResolvedValueOnce({
        success: true,
        message: 'Email verified',
      })

      const { result } = renderHook(() => useConfirmVerification(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      await act(async () => {
        result.current.mutate('valid-token')
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))
    })
  })

  describe('useUpdateProfile', () => {
    it('calls PATCH /auth/profile with the update body', async () => {
      mockApiRequest.mockResolvedValueOnce({
        success: true,
        message: 'Profile updated',
        user: { id: '1', email: 'a@b.c', username: 'newname' },
      })

      const { result } = renderHook(() => useUpdateProfile(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        await result.current.mutateAsync({ username: 'newname', bio: 'hi' })
      })

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/auth/profile',
        expect.objectContaining({
          method: 'PATCH',
          body: JSON.stringify({ username: 'newname', bio: 'hi' }),
        })
      )
    })

    it('throws AuthError when update fails', async () => {
      mockApiRequest.mockResolvedValueOnce({
        success: false,
        message: 'Username taken',
        error_code: 'VALIDATION_FAILED',
      })

      const { result } = renderHook(() => useUpdateProfile(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({ username: 'taken' })
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
      expect((result.current.error as Error).name).toBe('AuthError')
    })
  })

  describe('useChangePassword', () => {
    it('sends current + new password to the change-password endpoint', async () => {
      mockApiRequest.mockResolvedValueOnce({
        success: true,
        message: 'Password changed',
      })

      const { result } = renderHook(() => useChangePassword(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        await result.current.mutateAsync({
          current_password: 'old123',
          new_password: 'new456',
        })
      })

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/auth/change-password',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({
            current_password: 'old123',
            new_password: 'new456',
          }),
        })
      )
    })

    it('throws AuthError when current password is wrong', async () => {
      mockApiRequest.mockResolvedValueOnce({
        success: false,
        message: 'Current password incorrect',
        error_code: 'INVALID_CREDENTIALS',
      })

      const { result } = renderHook(() => useChangePassword(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({
          current_password: 'wrong',
          new_password: 'new456',
        })
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
      expect((result.current.error as unknown as { code: string }).code).toBe(
        AuthErrorCode.INVALID_CREDENTIALS
      )
    })
  })

  describe('useSendMagicLink', () => {
    it('calls the magic-link send endpoint with email', async () => {
      mockApiRequest.mockResolvedValueOnce({
        success: true,
        message: 'If eligible, sent',
      })

      const { result } = renderHook(() => useSendMagicLink(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        await result.current.mutateAsync({ email: 'test@example.com' })
      })

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/auth/magic-link/send',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({ email: 'test@example.com' }),
        })
      )
    })

    it('does NOT throw on success=false (enumeration-safe path)', async () => {
      // The backend returns 200 with success=false on cases like
      // EMAIL_NOT_VERIFIED to avoid leaking which addresses exist. The
      // hook must propagate this as a normal mutation result, not as
      // an error — UI inspects `data.error_code`.
      mockApiRequest.mockResolvedValueOnce({
        success: false,
        message: 'Email not verified',
        error_code: 'EMAIL_NOT_VERIFIED',
      })

      const { result } = renderHook(() => useSendMagicLink(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        await result.current.mutateAsync({ email: 'test@example.com' })
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))
      expect(result.current.data?.success).toBe(false)
      expect(result.current.data?.error_code).toBe('EMAIL_NOT_VERIFIED')
      expect(result.current.isError).toBe(false)
    })
  })

  describe('useVerifyMagicLink', () => {
    it('calls verify endpoint with token', async () => {
      mockApiRequest.mockResolvedValueOnce({
        success: true,
        message: 'Logged in',
        user: { id: '1', email: 'a@b.c' },
      })

      const { result } = renderHook(() => useVerifyMagicLink(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        await result.current.mutateAsync('magic-token-123')
      })

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/auth/magic-link/verify',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({ token: 'magic-token-123' }),
        })
      )
    })

    it('throws AuthError on invalid/expired token', async () => {
      mockApiRequest.mockResolvedValueOnce({
        success: false,
        message: 'Invalid or expired magic link',
        error_code: 'TOKEN_EXPIRED',
      })

      const { result } = renderHook(() => useVerifyMagicLink(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate('bad-token')
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
      expect((result.current.error as Error).name).toBe('AuthError')
    })
  })

  describe('useDeleteAccount', () => {
    it('sends password (and optional reason) to delete endpoint', async () => {
      mockApiRequest.mockResolvedValueOnce({
        success: true,
        message: 'Account scheduled for deletion',
        deletion_date: '2026-06-01',
        grace_period_days: 7,
      })

      const { result } = renderHook(() => useDeleteAccount(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        await result.current.mutateAsync({
          password: 'pw',
          reason: 'no longer needed',
        })
      })

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/auth/account/delete',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({ password: 'pw', reason: 'no longer needed' }),
        })
      )
    })

    it('clears query cache on successful deletion', async () => {
      const queryClient = createTestQueryClient()
      const clearSpy = vi.spyOn(queryClient, 'clear')

      mockApiRequest.mockResolvedValueOnce({
        success: true,
        message: 'Account scheduled for deletion',
      })

      const { result } = renderHook(() => useDeleteAccount(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      await act(async () => {
        await result.current.mutateAsync({ password: 'pw' })
      })

      expect(clearSpy).toHaveBeenCalled()
    })

    it('throws AuthError on wrong password', async () => {
      mockApiRequest.mockResolvedValueOnce({
        success: false,
        message: 'Incorrect password',
        error_code: 'INVALID_CREDENTIALS',
      })

      const { result } = renderHook(() => useDeleteAccount(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({ password: 'wrong' })
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
      expect((result.current.error as unknown as { code: string }).code).toBe(
        AuthErrorCode.INVALID_CREDENTIALS
      )
    })
  })

  describe('useExportData', () => {
    it('calls export-data and returns the export payload', async () => {
      const exportPayload = {
        success: true,
        message: 'Export ready',
        exported_at: '2025-03-15T00:00:00Z',
        export_version: '1.0',
        profile: {
          id: 1,
          email: 'a@b.c',
          is_admin: false,
          email_verified: true,
          created_at: '2024-01-01T00:00:00Z',
          updated_at: '2025-01-01T00:00:00Z',
        },
      }
      mockApiRequest.mockResolvedValueOnce(exportPayload)

      const { result } = renderHook(() => useExportData(), {
        wrapper: createWrapper(),
      })

      let returned: unknown
      await act(async () => {
        returned = await result.current.mutateAsync()
      })

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/auth/account/export',
        expect.objectContaining({ method: 'GET' })
      )
      expect(returned).toEqual(exportPayload)
    })

    it('throws AuthError when export fails', async () => {
      mockApiRequest.mockResolvedValueOnce({
        success: false,
        message: 'Export disabled',
      })

      const { result } = renderHook(() => useExportData(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate()
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
      expect((result.current.error as Error).name).toBe('AuthError')
    })
  })

  describe('useOAuthAccounts (SSO state on page load)', () => {
    it('fetches connected OAuth accounts', async () => {
      // PSY-700 specifically called out "SSO state on page load" as a
      // critical behavior to cover. useOAuthAccounts is the GET that
      // populates the "Linked accounts" panel on settings — silent
      // failures here mean the user can't tell whether Google/GitHub
      // is actually linked.
      const response = {
        success: true,
        accounts: [
          {
            provider: 'google',
            email: 'test@gmail.com',
            connected_at: '2024-01-01T00:00:00Z',
          },
        ],
      }
      mockApiRequest.mockResolvedValueOnce(response)

      const { result } = renderHook(() => useOAuthAccounts(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))
      expect(mockApiRequest).toHaveBeenCalledWith(
        '/auth/oauth/accounts',
        expect.objectContaining({ method: 'GET', credentials: 'include' })
      )
      expect(result.current.data).toEqual(response)
    })

    it('exposes the full list of connected providers', async () => {
      const response = {
        success: true,
        accounts: [
          { provider: 'google', connected_at: '2024-01-01T00:00:00Z' },
          { provider: 'github', connected_at: '2024-02-01T00:00:00Z' },
        ],
      }
      mockApiRequest.mockResolvedValueOnce(response)

      const { result } = renderHook(() => useOAuthAccounts(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))
      expect(result.current.data?.accounts).toHaveLength(2)
      expect(result.current.data?.accounts[0].provider).toBe('google')
      expect(result.current.data?.accounts[1].provider).toBe('github')
    })

    it('returns empty accounts array when no SSO is linked', async () => {
      mockApiRequest.mockResolvedValueOnce({ success: true, accounts: [] })

      const { result } = renderHook(() => useOAuthAccounts(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))
      expect(result.current.data?.accounts).toEqual([])
    })

    it('surfaces 401 errors (so the settings page can prompt re-login)', async () => {
      const error = new Error('Unauthorized')
      Object.assign(error, { status: 401 })
      mockApiRequest.mockRejectedValueOnce(error)

      const { result } = renderHook(() => useOAuthAccounts(), {
        wrapper: createWrapper(),
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
      expect(result.current.data).toBeUndefined()
    })
  })

  describe('useUnlinkOAuthAccount', () => {
    it('calls DELETE on the provider-specific unlink endpoint', async () => {
      mockApiRequest.mockResolvedValueOnce({
        success: true,
        message: 'Unlinked',
      })

      const { result } = renderHook(() => useUnlinkOAuthAccount(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        await result.current.mutateAsync('google')
      })

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/auth/oauth/accounts/google',
        expect.objectContaining({ method: 'DELETE' })
      )
    })

    it('invalidates the oauth-accounts query on success', async () => {
      const queryClient = createTestQueryClient()
      const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

      mockApiRequest.mockResolvedValueOnce({
        success: true,
        message: 'Unlinked',
      })

      const { result } = renderHook(() => useUnlinkOAuthAccount(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      await act(async () => {
        await result.current.mutateAsync('github')
      })

      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: ['auth', 'oauth-accounts'],
      })
    })

    it('throws AuthError if unlink fails', async () => {
      mockApiRequest.mockResolvedValueOnce({
        success: false,
        message: 'Cannot unlink — no other auth method',
      })

      const { result } = renderHook(() => useUnlinkOAuthAccount(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate('google')
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
      expect((result.current.error as Error).name).toBe('AuthError')
    })
  })

  describe('useRecoverAccount', () => {
    it('calls recover endpoint with email + password', async () => {
      mockApiRequest.mockResolvedValueOnce({
        success: true,
        message: 'Account recovered',
        user: { id: '1', email: 'a@b.c' },
      })

      const { result } = renderHook(() => useRecoverAccount(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        await result.current.mutateAsync({
          email: 'a@b.c',
          password: 'pw',
        })
      })

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/auth/recover-account',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({ email: 'a@b.c', password: 'pw' }),
        })
      )
    })

    it('throws AuthError when recovery fails', async () => {
      mockApiRequest.mockResolvedValueOnce({
        success: false,
        message: 'Account not eligible for recovery',
      })

      const { result } = renderHook(() => useRecoverAccount(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate({ email: 'a@b.c', password: 'pw' })
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
    })
  })

  describe('useRequestAccountRecovery (enumeration-safe)', () => {
    it('does NOT throw on success=false (enumeration-safe acknowledgement)', async () => {
      // Backend always returns a generic "if eligible, sent" body — the
      // hook must propagate it as success so the UI shows the same
      // neutral message regardless of account state.
      mockApiRequest.mockResolvedValueOnce({
        success: false,
        message: 'If your account is eligible, an email was sent.',
      })

      const { result } = renderHook(() => useRequestAccountRecovery(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        await result.current.mutateAsync({ email: 'a@b.c' })
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))
      expect(result.current.isError).toBe(false)
      expect(result.current.data?.message).toBe(
        'If your account is eligible, an email was sent.'
      )
    })

    it('returns the generic acknowledgement on success=true', async () => {
      mockApiRequest.mockResolvedValueOnce({
        success: true,
        message: 'If your account is eligible, an email was sent.',
      })

      const { result } = renderHook(() => useRequestAccountRecovery(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        await result.current.mutateAsync({ email: 'a@b.c' })
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))
      expect(mockApiRequest).toHaveBeenCalledWith(
        '/auth/recover-account/request',
        expect.objectContaining({ method: 'POST' })
      )
    })
  })

  describe('useConfirmAccountRecovery', () => {
    it('calls confirm endpoint with token', async () => {
      mockApiRequest.mockResolvedValueOnce({
        success: true,
        message: 'Recovered',
        user: { id: '1', email: 'a@b.c' },
      })

      const { result } = renderHook(() => useConfirmAccountRecovery(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        await result.current.mutateAsync('recovery-token-xyz')
      })

      expect(mockApiRequest).toHaveBeenCalledWith(
        '/auth/recover-account/confirm',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({ token: 'recovery-token-xyz' }),
        })
      )
    })

    it('throws AuthError on invalid token', async () => {
      mockApiRequest.mockResolvedValueOnce({
        success: false,
        message: 'Invalid token',
      })

      const { result } = renderHook(() => useConfirmAccountRecovery(), {
        wrapper: createWrapper(),
      })

      await act(async () => {
        result.current.mutate('bad-token')
      })

      await waitFor(() => expect(result.current.isError).toBe(true))
      expect((result.current.error as Error).name).toBe('AuthError')
    })
  })

  // PSY-700 lists "multi-tab sync via BroadcastChannel" as a scenario to
  // cover. As of this PR there is NO BroadcastChannel implementation in
  // features/auth, frontend/lib, or anywhere else in the app code (only
  // node_modules). The auth flow today is driven by HTTP-only cookies +
  // TanStack Query — sibling tabs see updates on their next refetch
  // (window-focus refetch in dev; staleTime expiry otherwise). There is
  // nothing testable to assert here without first building the sync
  // mechanism, which is out of scope for a test-strengthening ticket.
  // Filing this as a documented gap; if a BroadcastChannel-based sync
  // ships later, the new code should land with its own tests.
})
