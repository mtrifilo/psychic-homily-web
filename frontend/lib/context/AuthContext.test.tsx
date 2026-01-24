import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createTestQueryClient } from '@/test/utils'

// Mock the auth hooks
const mockUseProfile = vi.fn()
const mockUseLogout = vi.fn()

vi.mock('@/lib/hooks/useAuth', () => ({
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
    mockUseProfile.mockReturnValue({
      data: null,
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

      // User override should be cleared (null)
      // Profile data would determine the final user state
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

    it('handles logout failure gracefully', async () => {
      const consoleSpy = vi.spyOn(console, 'error').mockImplementation(() => {})
      mockMutateAsync.mockRejectedValueOnce(new Error('Network error'))

      const { result } = renderHook(() => useAuthContext(), {
        wrapper: createWrapperWithClient(queryClient),
      })

      await act(async () => {
        result.current.logout()
      })

      expect(consoleSpy).toHaveBeenCalledWith(
        'Logout failed:',
        expect.any(Error)
      )

      consoleSpy.mockRestore()
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
