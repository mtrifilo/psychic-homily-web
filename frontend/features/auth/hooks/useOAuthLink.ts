'use client'

import { useMutation, useQuery } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import { AuthError, AuthErrorCode, type AuthErrorCodeType } from '@/lib/errors'
import { queryKeys } from '@/lib/queryClient'
import type { components } from '@/types/api'

/** What the Sign in tab's collision state renders from. */
export type PendingOAuthLink =
  components['schemas']['PendingOAuthLinkResponseBody']

type OAuthLinkByEmailResponse =
  components['schemas']['OAuthLinkByEmailResponseBody']

/** The Sign in tab's own copy for a magic link that could not be sent. */
export const SEND_LINK_FAILED_MESSAGE = 'Failed to send magic link. Please try again.'

/**
 * Reads this browser's pending Google link. The backend names it by an
 * HttpOnly cookie, so the address never travels in the URL and only this read
 * can tell whether one is live. Read once per visit to the auth page and never
 * refetched while it shows: the proofs' own answers correct a stale read.
 * `enabled` is false where no sign-in is offered, which never reads it.
 */
export function usePendingOAuthLink(enabled: boolean) {
  return useQuery({
    queryKey: queryKeys.auth.pendingOAuthLink,
    queryFn: ({ signal }) =>
      apiRequest<PendingOAuthLink>(API_ENDPOINTS.AUTH.OAUTH_PENDING_LINK, {
        method: 'GET',
        signal,
      }),
    enabled,
    staleTime: Infinity,
    gcTime: 0,
    retry: false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  })
}

/**
 * Emails the sign-in link that proves the account's mailbox for the pending
 * link. A refusal the backend answers with success false is thrown as an
 * AuthError carrying its code.
 */
export function useSendOAuthLinkEmail() {
  return useMutation({
    mutationFn: async (): Promise<OAuthLinkByEmailResponse> => {
      const response = await apiRequest<OAuthLinkByEmailResponse>(
        API_ENDPOINTS.AUTH.OAUTH_LINK_BY_EMAIL,
        { method: 'POST' }
      )
      if (!response.success) {
        throw new AuthError(
          response.message || SEND_LINK_FAILED_MESSAGE,
          (response.error_code as AuthErrorCodeType) || AuthErrorCode.UNKNOWN,
          { requestId: response.request_id, status: 400 }
        )
      }
      return response
    },
  })
}
