'use client'

/**
 * Authentication Hooks
 *
 * TanStack Query hooks for authentication operations with HTTP-only cookies.
 * Uses proper caching, error handling, structured logging, and typed errors.
 */

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '../api'
import { queryKeys, createInvalidateQueries } from '../queryClient'
import { authLogger } from '../utils/authLogger'
import { AuthError, AuthErrorCode, type AuthErrorCodeType } from '../errors'

// Types
interface LoginCredentials {
  email: string
  password: string
}

interface RegisterCredentials {
  email: string
  password: string
  first_name?: string
  last_name?: string
}

interface AuthResponse {
  success: boolean
  message: string
  error_code?: AuthErrorCodeType
  request_id?: string
  user?: {
    id: string
    email: string
    name?: string
    first_name?: string
    last_name?: string
    is_admin?: boolean
  }
}

interface UserProfile {
  success: boolean
  message: string
  error_code?: AuthErrorCodeType
  request_id?: string
  user?: {
    id: string
    email: string
    name?: string
    first_name?: string
    last_name?: string
    is_admin?: boolean
    email_verified?: boolean
    created_at: string
    updated_at: string
  }
}

interface RefreshTokenResponse {
  success: boolean
  token?: string
  message: string
  error_code?: AuthErrorCodeType
  request_id?: string
}

// Login mutation
export const useLogin = () => {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (
      credentials: LoginCredentials
    ): Promise<AuthResponse> => {
      authLogger.loginAttempt(credentials.email)

      const response = await apiRequest<AuthResponse>(
        API_ENDPOINTS.AUTH.LOGIN,
        {
          method: 'POST',
          body: JSON.stringify(credentials),
          credentials: 'include',
        }
      )

      // Throw a typed error if login was unsuccessful
      if (!response.success) {
        authLogger.loginFailed(
          response.error_code || AuthErrorCode.UNKNOWN,
          response.message,
          response.request_id
        )

        throw new AuthError(
          response.message || 'Login failed',
          response.error_code || AuthErrorCode.INVALID_CREDENTIALS,
          {
            requestId: response.request_id,
            status: 401,
          }
        )
      }

      return response
    },
    onSuccess: async data => {
      if (data.success && data.user) {
        authLogger.loginSuccess(data.user.id, data.request_id)

        // Refetch profile to ensure we have complete user data including is_admin
        // This is more reliable than caching the login response since the profile
        // endpoint returns the full user object from the database
        await queryClient.refetchQueries({ queryKey: queryKeys.auth.profile })
      }
    },
  })
}

// Register mutation
export const useRegister = () => {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (
      credentials: RegisterCredentials
    ): Promise<AuthResponse> => {
      authLogger.debug('Registration attempt', {
        email: credentials.email.slice(0, 2) + '***',
      })

      const response = await apiRequest<AuthResponse>(
        API_ENDPOINTS.AUTH.REGISTER,
        {
          method: 'POST',
          body: JSON.stringify(credentials),
          credentials: 'include',
        }
      )

      // Throw a typed error if registration was unsuccessful
      if (!response.success) {
        authLogger.warn(
          'Registration failed',
          {
            errorCode: response.error_code,
            message: response.message,
          },
          response.request_id
        )

        throw new AuthError(
          response.message || 'Registration failed',
          response.error_code || AuthErrorCode.UNKNOWN,
          {
            requestId: response.request_id,
            status: 400,
          }
        )
      }

      return response
    },
    onSuccess: async data => {
      if (data.success && data.user) {
        authLogger.info(
          'Registration successful',
          { userId: data.user.id },
          data.request_id
        )

        // Refetch profile to ensure we have complete user data
        await queryClient.refetchQueries({ queryKey: queryKeys.auth.profile })
      }
    },
  })
}

// Logout mutation
export const useLogout = () => {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (): Promise<{ success: boolean; message: string }> => {
      authLogger.debug('Logout attempt')

      return apiRequest(API_ENDPOINTS.AUTH.LOGOUT, {
        method: 'POST',
        credentials: 'include',
      })
    },
    onSuccess: () => {
      authLogger.logout()
      // Clear all cached data (HTTP-only cookie is cleared by server)
      queryClient.clear()
    },
    onError: error => {
      authLogger.error('Logout failed', error)
      // Clear cached data even on error (in case of network issues)
      queryClient.clear()
    },
  })
}

// Get user profile query
export const useProfile = () => {
  return useQuery({
    queryKey: queryKeys.auth.profile,
    queryFn: async (): Promise<UserProfile> => {
      authLogger.debug('Fetching profile')

      const response = await apiRequest<UserProfile>(
        API_ENDPOINTS.AUTH.PROFILE,
        {
          method: 'GET',
          credentials: 'include',
        }
      )

      if (response.success && response.user) {
        authLogger.profileFetch(true, response.user.id, response.request_id)
      } else {
        authLogger.profileFetch(false, undefined, response.request_id)
      }

      return response
    },
    staleTime: 5 * 60 * 1000, // 5 minutes
    retry: (failureCount, error) => {
      // Check if it's an AuthError or has status property
      const authError =
        error instanceof AuthError ? error : AuthError.fromUnknown(error)

      // Don't retry on authentication errors
      if (authError.shouldRedirectToLogin || authError.status === 403) {
        authLogger.debug('Profile fetch auth error, not retrying', {
          code: authError.code,
        })
        return false
      }

      return failureCount < 2
    },
  })
}

// Refresh token mutation
export const useRefreshToken = () => {
  const queryClient = useQueryClient()
  const invalidateQueries = createInvalidateQueries(queryClient)

  return useMutation({
    mutationFn: async (): Promise<RefreshTokenResponse> => {
      authLogger.debug('Token refresh attempt')

      return apiRequest<RefreshTokenResponse>(API_ENDPOINTS.AUTH.REFRESH, {
        method: 'POST',
        credentials: 'include',
      })
    },
    onSuccess: data => {
      authLogger.tokenRefresh(data.success, data.request_id)

      if (data.success) {
        // Invalidate auth queries to refetch with new token
        invalidateQueries.auth()
      }
    },
    onError: error => {
      authLogger.error('Token refresh failed', error)
      // Clear all cached data on refresh failure
      queryClient.clear()
    },
  })
}

// Check if user is authenticated
export const useIsAuthenticated = () => {
  const { data: profile, isLoading, error } = useProfile()

  if (error) {
    authLogger.error('Error checking authentication', error)
  }

  return {
    isAuthenticated:
      Boolean(profile?.success) && Boolean(profile?.user) && !error,
    isLoading,
    user: profile?.user,
    error,
  }
}

// Email verification types
interface VerificationResponse {
  success: boolean
  message: string
  error_code?: string
  request_id?: string
}

// Password change types
interface ChangePasswordCredentials {
  current_password: string
  new_password: string
}

interface ChangePasswordResponse {
  success: boolean
  message: string
  error_code?: AuthErrorCodeType
  request_id?: string
}

// Send verification email mutation
export const useSendVerificationEmail = () => {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (): Promise<VerificationResponse> => {
      authLogger.debug('Sending verification email')

      const response = await apiRequest<VerificationResponse>(
        API_ENDPOINTS.AUTH.VERIFY_EMAIL_SEND,
        {
          method: 'POST',
          credentials: 'include',
        }
      )

      if (!response.success) {
        throw new AuthError(
          response.message || 'Failed to send verification email',
          (response.error_code as AuthErrorCodeType) || AuthErrorCode.UNKNOWN,
          {
            requestId: response.request_id,
            status: 400,
          }
        )
      }

      return response
    },
    onSuccess: data => {
      authLogger.info(
        'Verification email sent',
        { message: data.message },
        data.request_id
      )
    },
  })
}

// Confirm email verification mutation
export const useConfirmVerification = () => {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (token: string): Promise<VerificationResponse> => {
      authLogger.debug('Confirming email verification')

      const response = await apiRequest<VerificationResponse>(
        API_ENDPOINTS.AUTH.VERIFY_EMAIL_CONFIRM,
        {
          method: 'POST',
          body: JSON.stringify({ token }),
          credentials: 'include',
        }
      )

      if (!response.success) {
        throw new AuthError(
          response.message || 'Failed to verify email',
          (response.error_code as AuthErrorCodeType) || AuthErrorCode.UNKNOWN,
          {
            requestId: response.request_id,
            status: 400,
          }
        )
      }

      return response
    },
    onSuccess: async data => {
      authLogger.info(
        'Email verified successfully',
        { message: data.message },
        data.request_id
      )
      // Refetch profile to update email_verified status
      await queryClient.refetchQueries({ queryKey: queryKeys.auth.profile })
    },
  })
}

// Change password mutation
export const useChangePassword = () => {
  return useMutation({
    mutationFn: async (
      credentials: ChangePasswordCredentials
    ): Promise<ChangePasswordResponse> => {
      authLogger.debug('Password change attempt')

      const response = await apiRequest<ChangePasswordResponse>(
        API_ENDPOINTS.AUTH.CHANGE_PASSWORD,
        {
          method: 'POST',
          body: JSON.stringify(credentials),
          credentials: 'include',
        }
      )

      if (!response.success) {
        authLogger.warn(
          'Password change failed',
          {
            errorCode: response.error_code,
            message: response.message,
          },
          response.request_id
        )

        throw new AuthError(
          response.message || 'Failed to change password',
          response.error_code || AuthErrorCode.UNKNOWN,
          {
            requestId: response.request_id,
            status: 400,
          }
        )
      }

      return response
    },
    onSuccess: data => {
      authLogger.info(
        'Password changed successfully',
        { message: data.message },
        data.request_id
      )
    },
  })
}
