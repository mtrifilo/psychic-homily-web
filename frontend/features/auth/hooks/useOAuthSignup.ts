'use client'

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import { AuthError, AuthErrorCode, type AuthErrorCodeType } from '@/lib/errors'
import { queryKeys } from '@/lib/queryClient'
import type { components } from '@/types/api'
import { refreshCachesForNewSession, type AuthResponse } from './useAuth'

/** What the "Create your account" step renders from. */
export type PendingOAuthSignup =
  components['schemas']['PendingOAuthSignupResponseBody']

/**
 * The finish answers registration's envelope plus where the person was headed.
 * `user` is read through AuthResponse, the shape every session entry hands
 * `setUser`.
 */
export interface FinishOAuthSignupResponse extends AuthResponse {
  return_to?: components['schemas']['FinishOAuthSignupResponseBody']['return_to']
}

/** The step's submission. */
export type FinishOAuthSignupRequest = Omit<
  components['schemas']['FinishOAuthSignupRequestBody'],
  '$schema'
>

const SIGNUP_FAILED = 'Your account could not be created. Please try again.'

/**
 * Reads this browser's pending Google sign-up. The backend names it by an
 * HttpOnly cookie, so nothing about it travels in the URL. Read once per visit
 * to the step: never refetched while it shows, since a stale read is corrected
 * by the finish's own answer, and dropped as soon as the step is left, since
 * the cookie may have changed by the next visit.
 */
export function usePendingOAuthSignup() {
  return useQuery({
    queryKey: queryKeys.auth.pendingOAuthSignup,
    queryFn: ({ signal }) =>
      apiRequest<PendingOAuthSignup>(API_ENDPOINTS.AUTH.OAUTH_PENDING_SIGNUP, {
        method: 'GET',
        signal,
      }),
    staleTime: Infinity,
    gcTime: 0,
    retry: false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  })
}

/**
 * Finishes the pending Google sign-up. A refusal the backend answers with
 * success false is thrown as an AuthError carrying its code; on success the
 * new session's caches are refreshed before the caller's onSuccess runs.
 */
export function useFinishOAuthSignup() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (
      request: FinishOAuthSignupRequest
    ): Promise<FinishOAuthSignupResponse> => {
      const response = await apiRequest<FinishOAuthSignupResponse>(
        API_ENDPOINTS.AUTH.OAUTH_FINISH_SIGNUP,
        {
          method: 'POST',
          body: JSON.stringify(request),
        }
      )
      if (!response.success) {
        throw new AuthError(
          response.message || SIGNUP_FAILED,
          (response.error_code as AuthErrorCodeType) || AuthErrorCode.UNKNOWN,
          { requestId: response.request_id, status: 400 }
        )
      }
      return response
    },
    onSuccess: async () => {
      await refreshCachesForNewSession(queryClient)
    },
  })
}
