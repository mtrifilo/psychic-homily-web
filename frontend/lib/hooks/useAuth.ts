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
  terms_accepted: boolean
  terms_version: string
  privacy_version: string
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

      try {
        return await apiRequest(API_ENDPOINTS.AUTH.LOGOUT, {
          method: 'POST',
          credentials: 'include',
        })
      } catch (error) {
        // If the token is already expired/missing, the session is already gone —
        // treat this as a successful logout rather than letting it cascade
        // through error handlers that would clear the query cache repeatedly
        if (error instanceof AuthError && error.shouldRedirectToLogin) {
          return { success: true, message: 'Session already expired' }
        }
        throw error
      }
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

// Magic link types
interface SendMagicLinkRequest {
  email: string
}

interface SendMagicLinkResponse {
  success: boolean
  message: string
  error_code?: string
  request_id?: string
}

interface VerifyMagicLinkResponse {
  success: boolean
  message: string
  error_code?: string
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

// Send magic link mutation
export const useSendMagicLink = () => {
  return useMutation({
    mutationFn: async (
      request: SendMagicLinkRequest
    ): Promise<SendMagicLinkResponse> => {
      authLogger.debug('Magic link request', {
        email: request.email.slice(0, 2) + '***',
      })

      const response = await apiRequest<SendMagicLinkResponse>(
        API_ENDPOINTS.AUTH.MAGIC_LINK_SEND,
        {
          method: 'POST',
          body: JSON.stringify(request),
          credentials: 'include',
        }
      )

      // Note: We don't throw on !success for email enumeration protection
      // The error_code field indicates specific issues like EMAIL_NOT_VERIFIED
      return response
    },
    onSuccess: data => {
      authLogger.info(
        'Magic link response',
        { message: data.message, errorCode: data.error_code },
        data.request_id
      )
    },
  })
}

// Verify magic link mutation
export const useVerifyMagicLink = () => {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (token: string): Promise<VerifyMagicLinkResponse> => {
      authLogger.debug('Verifying magic link')

      const response = await apiRequest<VerifyMagicLinkResponse>(
        API_ENDPOINTS.AUTH.MAGIC_LINK_VERIFY,
        {
          method: 'POST',
          body: JSON.stringify({ token }),
          credentials: 'include',
        }
      )

      if (!response.success) {
        throw new AuthError(
          response.message || 'Invalid or expired magic link',
          (response.error_code as AuthErrorCodeType) || AuthErrorCode.UNKNOWN,
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
        authLogger.info(
          'Magic link login successful',
          { userId: data.user.id },
          data.request_id
        )
        // Refetch profile to get complete user data
        await queryClient.refetchQueries({ queryKey: queryKeys.auth.profile })
      }
    },
  })
}

// Account deletion types
interface DeletionSummaryResponse {
  success: boolean
  message: string
  shows_count: number
  saved_shows_count: number
  passkeys_count: number
  has_password: boolean
  error_code?: string
  request_id?: string
}

interface DeleteAccountRequest {
  password: string
  reason?: string
}

interface DeleteAccountResponse {
  success: boolean
  message: string
  deletion_date?: string
  grace_period_days?: number
  error_code?: AuthErrorCodeType
  request_id?: string
}

// Get deletion summary query
export const useDeletionSummary = () => {
  return useQuery({
    queryKey: ['auth', 'deletion-summary'],
    queryFn: async (): Promise<DeletionSummaryResponse> => {
      authLogger.debug('Fetching deletion summary')

      const response = await apiRequest<DeletionSummaryResponse>(
        API_ENDPOINTS.AUTH.DELETION_SUMMARY,
        {
          method: 'GET',
          credentials: 'include',
        }
      )

      if (!response.success) {
        throw new AuthError(
          response.message || 'Failed to get deletion summary',
          (response.error_code as AuthErrorCodeType) || AuthErrorCode.UNKNOWN,
          {
            requestId: response.request_id,
            status: 400,
          }
        )
      }

      return response
    },
    enabled: false, // Only fetch when explicitly called
  })
}

// Delete account mutation
export const useDeleteAccount = () => {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (
      request: DeleteAccountRequest
    ): Promise<DeleteAccountResponse> => {
      authLogger.debug('Delete account attempt')

      const response = await apiRequest<DeleteAccountResponse>(
        API_ENDPOINTS.AUTH.DELETE_ACCOUNT,
        {
          method: 'POST',
          body: JSON.stringify(request),
          credentials: 'include',
        }
      )

      if (!response.success) {
        authLogger.warn(
          'Delete account failed',
          {
            errorCode: response.error_code,
            message: response.message,
          },
          response.request_id
        )

        throw new AuthError(
          response.message || 'Failed to delete account',
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
        'Account deletion successful',
        {
          message: data.message,
          deletionDate: data.deletion_date,
        },
        data.request_id
      )
      // Clear all cached data (user is now logged out)
      queryClient.clear()
    },
  })
}

// Data export types (GDPR Right to Portability)
interface ExportDataResponse {
  success: boolean
  message: string
  exported_at: string
  export_version: string
  profile: {
    id: number
    email: string
    first_name?: string
    last_name?: string
    is_admin: boolean
    email_verified: boolean
    created_at: string
    updated_at: string
  }
  preferences?: {
    email_notifications: boolean
    marketing_emails: boolean
  }
  oauth_accounts?: Array<{
    provider: string
    provider_id: string
    linked_at: string
  }>
  passkeys?: Array<{
    credential_id: string
    name: string
    created_at: string
    last_used_at?: string
  }>
  saved_shows?: Array<{
    show_id: number
    saved_at: string
  }>
  submitted_shows?: Array<{
    id: number
    status: string
    created_at: string
    updated_at: string
  }>
  error_code?: string
  request_id?: string
}

// Export user data mutation (GDPR Right to Portability)
export const useExportData = () => {
  return useMutation({
    mutationFn: async (): Promise<ExportDataResponse> => {
      authLogger.debug('Exporting user data')

      const response = await apiRequest<ExportDataResponse>(
        API_ENDPOINTS.AUTH.EXPORT_DATA,
        {
          method: 'GET',
          credentials: 'include',
        }
      )

      if (!response.success) {
        throw new AuthError(
          response.message || 'Failed to export data',
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
        'Data export successful',
        { exportedAt: data.exported_at },
        data.request_id
      )
    },
  })
}

// OAuth account types
interface OAuthAccount {
  provider: string
  email?: string
  name?: string
  avatar_url?: string
  connected_at: string
}

interface GetOAuthAccountsResponse {
  success: boolean
  accounts: OAuthAccount[]
  error_code?: string
  request_id?: string
}

interface UnlinkOAuthAccountResponse {
  success: boolean
  message: string
  error_code?: string
  request_id?: string
}

// Get OAuth accounts query
export const useOAuthAccounts = () => {
  return useQuery({
    queryKey: ['auth', 'oauth-accounts'],
    queryFn: async (): Promise<GetOAuthAccountsResponse> => {
      authLogger.debug('Fetching OAuth accounts')

      const response = await apiRequest<GetOAuthAccountsResponse>(
        API_ENDPOINTS.AUTH.OAUTH_ACCOUNTS,
        {
          method: 'GET',
          credentials: 'include',
        }
      )

      return response
    },
  })
}

// Unlink OAuth account mutation
export const useUnlinkOAuthAccount = () => {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (provider: string): Promise<UnlinkOAuthAccountResponse> => {
      authLogger.debug('Unlinking OAuth account', { provider })

      const response = await apiRequest<UnlinkOAuthAccountResponse>(
        API_ENDPOINTS.AUTH.OAUTH_UNLINK(provider),
        {
          method: 'DELETE',
          credentials: 'include',
        }
      )

      if (!response.success) {
        throw new AuthError(
          response.message || 'Failed to unlink OAuth account',
          (response.error_code as AuthErrorCodeType) || AuthErrorCode.UNKNOWN,
          {
            requestId: response.request_id,
            status: 400,
          }
        )
      }

      return response
    },
    onSuccess: (data) => {
      authLogger.info(
        'OAuth account unlinked successfully',
        { message: data.message },
        data.request_id
      )
      // Refetch OAuth accounts list
      queryClient.invalidateQueries({ queryKey: ['auth', 'oauth-accounts'] })
    },
  })
}

// Account recovery types
interface RecoverAccountRequest {
  email: string
  password: string
}

interface RecoverAccountResponse {
  success: boolean
  message: string
  user?: {
    id: string
    email: string
    name?: string
    first_name?: string
    last_name?: string
    is_admin?: boolean
  }
  error_code?: AuthErrorCodeType
  request_id?: string
}

interface RequestAccountRecoveryRequest {
  email: string
}

interface RequestAccountRecoveryResponse {
  success: boolean
  message: string
  has_password?: boolean
  days_remaining?: number
  error_code?: string
  request_id?: string
}

interface ConfirmAccountRecoveryResponse {
  success: boolean
  message: string
  user?: {
    id: string
    email: string
    name?: string
    first_name?: string
    last_name?: string
    is_admin?: boolean
  }
  error_code?: AuthErrorCodeType
  request_id?: string
}

// Recover account with password mutation
export const useRecoverAccount = () => {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (
      request: RecoverAccountRequest
    ): Promise<RecoverAccountResponse> => {
      authLogger.debug('Account recovery attempt', {
        email: request.email.slice(0, 2) + '***',
      })

      const response = await apiRequest<RecoverAccountResponse>(
        API_ENDPOINTS.AUTH.RECOVER_ACCOUNT,
        {
          method: 'POST',
          body: JSON.stringify(request),
          credentials: 'include',
        }
      )

      if (!response.success) {
        authLogger.warn(
          'Account recovery failed',
          {
            errorCode: response.error_code,
            message: response.message,
          },
          response.request_id
        )

        throw new AuthError(
          response.message || 'Account recovery failed',
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
          'Account recovered successfully',
          { userId: data.user.id },
          data.request_id
        )
        // Refetch profile to get complete user data
        await queryClient.refetchQueries({ queryKey: queryKeys.auth.profile })
      }
    },
  })
}

// Request account recovery (magic link) mutation
export const useRequestAccountRecovery = () => {
  return useMutation({
    mutationFn: async (
      request: RequestAccountRecoveryRequest
    ): Promise<RequestAccountRecoveryResponse> => {
      authLogger.debug('Account recovery request', {
        email: request.email.slice(0, 2) + '***',
      })

      const response = await apiRequest<RequestAccountRecoveryResponse>(
        API_ENDPOINTS.AUTH.RECOVER_ACCOUNT_REQUEST,
        {
          method: 'POST',
          body: JSON.stringify(request),
          credentials: 'include',
        }
      )

      // Note: We return the response even if !success to provide
      // has_password and days_remaining information
      return response
    },
    onSuccess: data => {
      authLogger.info(
        'Account recovery request processed',
        {
          message: data.message,
          hasPassword: data.has_password,
          daysRemaining: data.days_remaining,
        },
        data.request_id
      )
    },
  })
}

// Confirm account recovery (magic link) mutation
export const useConfirmAccountRecovery = () => {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (token: string): Promise<ConfirmAccountRecoveryResponse> => {
      authLogger.debug('Confirming account recovery')

      const response = await apiRequest<ConfirmAccountRecoveryResponse>(
        API_ENDPOINTS.AUTH.RECOVER_ACCOUNT_CONFIRM,
        {
          method: 'POST',
          body: JSON.stringify({ token }),
          credentials: 'include',
        }
      )

      if (!response.success) {
        throw new AuthError(
          response.message || 'Account recovery failed',
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
          'Account recovery confirmed',
          { userId: data.user.id },
          data.request_id
        )
        // Refetch profile to get complete user data
        await queryClient.refetchQueries({ queryKey: queryKeys.auth.profile })
      }
    },
  })
}

// CLI Token response type
interface CLITokenResponse {
  success: boolean
  token?: string
  expires_in?: number
  message: string
  error_code?: string
  request_id?: string
}

// Generate CLI token mutation (admin only)
export const useGenerateCLIToken = () => {
  return useMutation({
    mutationFn: async (): Promise<CLITokenResponse> => {
      authLogger.debug('Generating CLI token')

      const response = await apiRequest<CLITokenResponse>(
        API_ENDPOINTS.AUTH.CLI_TOKEN,
        {
          method: 'POST',
          credentials: 'include',
        }
      )

      if (!response.success) {
        throw new AuthError(
          response.message || 'Failed to generate CLI token',
          (response.error_code as AuthErrorCodeType) || AuthErrorCode.UNKNOWN,
          {
            requestId: response.request_id,
            status: 400,
          }
        )
      }

      return response
    },
  })
}

// ============================================================================
// API Token Management (Admin only - for discovery app)
// ============================================================================

// API Token types
export interface APIToken {
  id: number
  description: string | null
  scope: string
  created_at: string
  expires_at: string
  last_used_at: string | null
  is_expired: boolean
}

interface APITokenListResponse {
  tokens: APIToken[]
}

interface APITokenCreateRequest {
  description?: string
  expiration_days?: number
}

interface APITokenCreateResponse {
  id: number
  token: string // Plaintext token - only shown once
  description: string | null
  scope: string
  created_at: string
  expires_at: string
}

interface APITokenRevokeResponse {
  message: string
}

// List API tokens query (admin only)
export const useAPITokens = () => {
  return useQuery({
    queryKey: ['admin', 'api-tokens'],
    queryFn: async (): Promise<APITokenListResponse> => {
      authLogger.debug('Fetching API tokens')

      const response = await apiRequest<APITokenListResponse>(
        API_ENDPOINTS.ADMIN.TOKENS.LIST,
        {
          method: 'GET',
          credentials: 'include',
        }
      )

      return response
    },
    staleTime: 30 * 1000, // 30 seconds - tokens change infrequently
  })
}

// Create API token mutation (admin only)
export const useCreateAPIToken = () => {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (
      request: APITokenCreateRequest
    ): Promise<APITokenCreateResponse> => {
      authLogger.debug('Creating API token')

      const response = await apiRequest<APITokenCreateResponse>(
        API_ENDPOINTS.ADMIN.TOKENS.CREATE,
        {
          method: 'POST',
          body: JSON.stringify(request),
          credentials: 'include',
        }
      )

      return response
    },
    onSuccess: () => {
      authLogger.info('API token created successfully')
      // Invalidate tokens list to refetch
      queryClient.invalidateQueries({ queryKey: ['admin', 'api-tokens'] })
    },
  })
}

// Revoke API token mutation (admin only)
export const useRevokeAPIToken = () => {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (tokenId: number): Promise<APITokenRevokeResponse> => {
      authLogger.debug('Revoking API token', { tokenId })

      const response = await apiRequest<APITokenRevokeResponse>(
        API_ENDPOINTS.ADMIN.TOKENS.REVOKE(tokenId),
        {
          method: 'DELETE',
          credentials: 'include',
        }
      )

      return response
    },
    onSuccess: (data) => {
      authLogger.info('API token revoked successfully', { message: data.message })
      // Invalidate tokens list to refetch
      queryClient.invalidateQueries({ queryKey: ['admin', 'api-tokens'] })
    },
  })
}
