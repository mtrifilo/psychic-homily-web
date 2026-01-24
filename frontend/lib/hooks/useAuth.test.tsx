import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createWrapper, createTestQueryClient } from '@/test/utils'
import { AuthErrorCode } from '../errors'

// Create a mock for apiRequest that we can control
const mockApiRequest = vi.fn()

// Mock the api module
vi.mock('../api', () => ({
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
    },
  },
  API_BASE_URL: 'http://localhost:8080',
}))

// Mock the auth logger
vi.mock('../utils/authLogger', () => ({
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
vi.mock('../queryClient', () => ({
  queryKeys: {
    auth: {
      profile: ['auth', 'profile'],
    },
  },
  createInvalidateQueries: () => ({
    auth: vi.fn(),
  }),
}))

// Import hooks after mocks are set up
import {
  useLogin,
  useRegister,
  useLogout,
  useProfile,
  useRefreshToken,
  useIsAuthenticated,
  useSendVerificationEmail,
  useConfirmVerification,
} from './useAuth'

// Helper to create wrapper with specific query client
function createWrapperWithClient(queryClient: QueryClient) {
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    )
  }
}

describe('useAuth hooks', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
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
      expect((result.current.error as { code: string }).code).toBe(
        AuthErrorCode.INVALID_CREDENTIALS
      )
    })

    it('succeeds and returns user data on successful login', async () => {
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

      expect(result.current.data?.user?.email).toBe('test@example.com')
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
        })
      })

      await waitFor(() => expect(result.current.isError).toBe(true))

      expect((result.current.error as Error).name).toBe('AuthError')
      expect((result.current.error as { code: string }).code).toBe(
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

      expect(result.current.data?.user?.email).toBe('test@example.com')
      expect(result.current.data?.user?.is_admin).toBe(true)
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

    it('succeeds and returns success message on successful verification', async () => {
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

      expect(result.current.data?.message).toBe('Email verified')
    })
  })
})
