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
export type FinishOAuthSignupRequest =
  components['schemas']['FinishOAuthSignupRequestBody']

const SIGNUP_FAILED = 'Your account could not be created. Please try again.'

/**
 * The read of this browser's pending Google sign-up, and of the under-age
 * refusal marker the same read reports. The backend names both by HttpOnly
 * cookies, so nothing about them travels in the URL. Read once per visit: never
 * refetched while the page shows, since a stale read is corrected by the
 * sign-up's own answer, and dropped as soon as the page is left, since the
 * cookies may have changed by the next visit.
 */
const pendingOAuthSignupQuery = {
  queryKey: queryKeys.auth.pendingOAuthSignup,
  queryFn: ({ signal }: { signal: AbortSignal }) =>
    apiRequest<PendingOAuthSignup>(API_ENDPOINTS.AUTH.OAUTH_PENDING_SIGNUP, {
      method: 'GET',
      signal,
    }),
  staleTime: Infinity,
  gcTime: 0,
  retry: false,
  refetchOnWindowFocus: false,
  refetchOnReconnect: false,
} as const

/** Reads this browser's pending Google sign-up, for the step that finishes it. */
export function usePendingOAuthSignup() {
  return useQuery(pendingOAuthSignupQuery)
}

/**
 * Whether this browser carries the under-age refusal marker, which refuses
 * every sign-up from it for a day: the pending-signup read's `age_refused`
 * status. Read while `enabled`; false while unread or when the read fails,
 * since the backend refuses a marked browser's sign-up either way.
 */
export function useSignupAgeRefused(enabled: boolean): boolean {
  const { data } = useQuery({
    ...pendingOAuthSignupQuery,
    select: signup => signup.status === 'age_refused',
    enabled,
  })
  return data === true
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
