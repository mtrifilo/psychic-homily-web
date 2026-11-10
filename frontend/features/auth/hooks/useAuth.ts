'use client'

/**
 * Authentication Hooks
 *
 * TanStack Query hooks for authentication operations with HTTP-only cookies.
 * Uses proper caching, error handling, structured logging, and typed errors.
 */

import {
  useMutation,
  useQuery,
  useQueryClient,
  type QueryClient,
  queryOptions,
} from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import {
  queryKeys,
  refreshViewerTierQueries,
  refetchSignedInProfile,
  releaseExpiredSessionLatch,
  resetViewerTierQueries,
} from '@/lib/queryClient'
import { refetchAfterMutation } from '@/lib/refetchAfterMutation'
import { authLogger } from '@/lib/utils/authLogger'
import {
  AuthError,
  AuthErrorCode,
  isDefinitiveUnauthenticated,
  type AuthErrorCodeType,
} from '@/lib/errors'
import type { NavMode } from '@/lib/nav-mode'
import { forgetAccountCreatedThisSession } from '../accountCreatedMarker'
import { refreshCalendarAfterLinkRevocation } from './useCalendarFeed'
import type { AuthApiUser } from '../authUser'
import type { APIToken } from '../types'
import type { components } from '@/types/api'

// Types
interface LoginCredentials {
  email: string
  password: string
  /**
   * The sign-in is the password proof for this browser's pending Google
   * link: when the link is gone, a correct password is refused with
   * OAUTH_LINK_PENDING_EXPIRED rather than signing in without connecting.
   */
  connect_pending_link?: boolean
}

interface RegisterCredentials {
  email: string
  password: string
  first_name?: string
  last_name?: string
  terms_accepted: boolean
  terms_version: string
  privacy_version: string
  /** YYYY-MM-DD; the backend computes the age from it. */
  date_of_birth: string
}

interface RegisterRequest extends RegisterCredentials {
  /**
   * Handed to the request and never sent. A registration that fails while it
   * is aborted fails with REGISTRATION_FAILED, whatever the abort rejected with.
   */
  signal?: AbortSignal
  /**
   * Called once the register request has answered with success, before the
   * profile refetch that `onSuccess` awaits. Never sent. A bound on the request
   * alone ends here.
   */
  onRegistered?: () => void
}

/**
 * What the caller of `useRegister` does with the outcome, handed to the hook
 * rather than to `mutate`. TanStack Query skips `mutate`'s own callbacks once
 * the component that called it has unmounted; these run for every
 * registration, so anything they set must outlive that component.
 */
interface RegisterCallbacks {
  /** Runs after the profile refetch a registration with a user awaits. */
  onSuccess?: (data: AuthResponse, variables: RegisterRequest) => void
  onError?: (error: Error) => void
  onSettled?: () => void
}

const REGISTRATION_FAILED = 'Registration failed'

/**
 * The envelope every auth endpoint answers with.
 *
 * `user` is the full viewer payload, not a login-sized subset: the backend
 * serializes the same user model here as it does on /auth/profile. Exported
 * because the passkey components establish their session with a raw `fetch`
 * rather than a mutation, and read the same envelope.
 */
export interface AuthResponse {
  success: boolean
  message: string
  error_code?: AuthErrorCodeType
  request_id?: string
  user?: AuthApiUser
}

interface FavoriteCity {
  city: string
  state: string
}

interface UserPreferencesData {
  show_reminders?: boolean
  favorite_cities?: FavoriteCity[]
  // PSY-1423: saved /charts window + scene. Absent/null = no saved defaults.
  chart_defaults?: {
    window: 'month' | 'quarter' | 'all_time'
    scene: string | null
  } | null
  // Signed-in home section order + visibility, as the generated contract
  // types it. Absent/null = the shipped default layout.
  home_layout?: components['schemas']['HomeLayout'] | null
  // Set when the one-time first-save hint is first shown (the first time is
  // kept). Absent/null = never shown, the only state in which it may render.
  first_save_hint_dismissed_at?: string | null
  default_reply_permission?: string
  // PSY-350 / PSY-515: weekly digest of new items in collections the user
  // follows. Server default is FALSE (opt-IN); user toggles this from the
  // notification settings page.
  notify_on_collection_digest?: boolean
  notify_on_scene_digest?: boolean
  // PSY-756 / PSY-807: per-category opt-OUT for tier-change and edit-review
  // emails. Server default is TRUE (opt-OUT) for both; the UI treats an
  // undefined value as ON to match that default.
  notify_on_tier_notifications?: boolean
  notify_on_edit_notifications?: boolean
}

export interface UserProfile {
  success: boolean
  message: string
  error_code?: AuthErrorCodeType
  request_id?: string
  // The shared payload plus the fields only the profile endpoint's consumers
  // read. Declaring the shared part once is what lets `toAuthUser` map this
  // payload and a session-entry payload with the same code.
  user?: AuthApiUser & {
    name?: string
    created_at: string
    updated_at: string
    preferences?: UserPreferencesData
    /** The providers the account signs in with. Only `provider` is read. */
    oauth_accounts?: Pick<components['schemas']['OAuthAccount'], 'provider'>[] | null
  }
  /**
   * The account's other sign-in factors. Absent when the backend could not
   * read them, which is not the same as false and 0.
   */
  has_password?: components['schemas']['UserProfileResponseBody']['has_password']
  passkey_count?: components['schemas']['UserProfileResponseBody']['passkey_count']
  /**
   * The session is old enough that the client should POST /auth/refresh once
   * to restart its clock (SessionRenewal). Absent when not due.
   */
  session_renewal_due?: components['schemas']['UserProfileResponseBody']['session_renewal_due']
}

// The wire shape; the web client never receives `token` (a cookie credential
// is renewed in Set-Cookie), so nothing here reads it.
type RefreshTokenResponse = components['schemas']['RefreshTokenResponseBody']

/**
 * Cache work shared by every transition INTO an authenticated session: login,
 * registration, magic-link sign-in, email verification, and account recovery.
 *
 * The profile refetch is awaited because the rest of the app reads identity
 * from that one query: it outranks the in-session claim `setUser` makes for
 * the same viewer, so until it lands the new session's identity rests on that
 * claim alone. The two passkey components call it too, unawaited, since they
 * establish their session with a raw fetch.
 *
 * The viewer-tier refresh is deliberately NOT awaited. Any panel already on
 * screen whose payload depends on privilege (revision history, comments)
 * still holds the previous viewer's answer, and would keep serving it for the
 * rest of the 15-minute `staleTime` because those cache keys carry no viewer
 * dimension. Refreshing them is a background correction, so the sign-in flow
 * must not block on it.
 */
export async function refreshCachesForNewSession(queryClient: QueryClient) {
  // eslint-disable-next-line no-restricted-syntax -- the profile is refetched without a cancel because the cache error handlers in lib/queryClient.ts read its fetchStatus; the SSR seed gives it data whenever the backend answered, so the refetch restarts an in-flight fetch instead of reusing it (an unseeded first fetch still in flight at sign-in can still reuse a pre-login response)
  await queryClient.refetchQueries({ queryKey: queryKeys.auth.profile })
  void refreshViewerTierQueries(queryClient)
}

/**
 * Cache work for the transition OUT of an authenticated session.
 *
 * The reset must precede the clear, and it is not redundant with it. See
 * `resetViewerTierQueries` for why `clear()` alone leaves a still-mounted
 * panel painting the payload the signed-in viewer could see.
 */
function dropSignedInCaches(queryClient: QueryClient) {
  forgetAccountCreatedThisSession()
  void resetViewerTierQueries(queryClient)
  // HTTP-only cookie is cleared by the server; drop everything else the
  // signed-in viewer could see.
  queryClient.clear()
}

// Login mutation
export const useLogin = () => {
  const queryClient = useQueryClient()

  return useMutation({
    mutationKey: sessionChangeKey('login'),
    mutationFn: async (
      credentials: LoginCredentials
    ): Promise<AuthResponse> => {
      await settlePendingSessionRenewal()
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

        // The response already carries the whole user model, which `setUser`
        // claims for the window before this lands. Refetching makes the profile
        // query — the source every consumer reads — agree with it.
        await refreshCachesForNewSession(queryClient)
      }
    },
  })
}

// Register mutation
export const useRegister = (callbacks: RegisterCallbacks = {}) => {
  const queryClient = useQueryClient()

  return useMutation({
    mutationKey: sessionChangeKey('register'),
    mutationFn: async ({
      signal,
      onRegistered,
      ...credentials
    }: RegisterRequest): Promise<AuthResponse> => {
      await settlePendingSessionRenewal()
      authLogger.debug('Registration attempt', {
        email: credentials.email.slice(0, 2) + '***',
      })

      const response = await apiRequest<AuthResponse>(
        API_ENDPOINTS.AUTH.REGISTER,
        {
          method: 'POST',
          body: JSON.stringify(credentials),
          credentials: 'include',
          signal,
        }
      ).catch((error: unknown) => {
        if (signal?.aborted) {
          throw new AuthError(REGISTRATION_FAILED, AuthErrorCode.UNKNOWN, {
            cause: error instanceof Error ? error : undefined,
          })
        }
        throw error
      })

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
          response.message || REGISTRATION_FAILED,
          response.error_code || AuthErrorCode.UNKNOWN,
          {
            requestId: response.request_id,
            status: 400,
          }
        )
      }

      onRegistered?.()
      return response
    },
    onSuccess: async (data, variables) => {
      if (data.success && data.user) {
        authLogger.info(
          'Registration successful',
          { userId: data.user.id },
          data.request_id
        )

        // Refetch profile to ensure we have complete user data
        await refreshCachesForNewSession(queryClient)
      }
      callbacks.onSuccess?.(data, variables)
    },
    onError: callbacks.onError,
    onSettled: callbacks.onSettled,
  })
}

// Logout mutation
export const useLogout = () => {
  const queryClient = useQueryClient()

  return useMutation({
    mutationKey: sessionChangeKey('logout'),
    mutationFn: async (): Promise<{ success: boolean; message: string }> => {
      await settlePendingSessionRenewal()
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
      dropSignedInCaches(queryClient)
    },
    onError: error => {
      authLogger.error('Logout failed', error)
      // Clear cached data even on error (in case of network issues)
      dropSignedInCaches(queryClient)
    },
  })
}

// Get user profile query
// Floor between event-triggered refetches of a FAILING profile query. An
// errored query has `dataUpdatedAt === 0` and so is permanently stale, which
// makes an unthrottled refetch fire on every focus or online event.
const PROFILE_ERROR_REFETCH_FLOOR_MS = 30_000

/** Exported for the options test; see the cells beside `refetchOnWindowFocus`. */
export const refetchFailedProfileOnly = (query: {
  state: { status: string; errorUpdatedAt: number }
}): boolean =>
  query.state.status === 'error' &&
  Date.now() - query.state.errorUpdatedAt > PROFILE_ERROR_REFETCH_FLOOR_MS

/**
 * The profile query's options. Shared by `useProfile` and `SessionRenewal`,
 * which adds a refetch interval on top so a tab kept open sees the renewal
 * flag come back; everything else about the query lives here once.
 */
export function profileQueryOptions(queryClient: QueryClient) {
  return queryOptions({
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
        // A payload naming a viewer means a session is live in this tab, which
        // is what re-arms the expiry re-mask. It hangs off the QUERY rather
        // than the session-entry mutations because not every way into a
        // session runs one here: a viewer who signs in on another tab reaches
        // this state through the focus refetch below, and a latch still held
        // then would let the next expiry keep painting their data.
        releaseExpiredSessionLatch(queryClient)
      } else {
        authLogger.profileFetch(false, undefined, response.request_id)
      }

      return response
    },
    staleTime: 5 * 60 * 1000, // 5 minutes
    // An indeterminate failure reads as 'pending', which gates UI, so this
    // query's error state must be recoverable. Cells:
    //   healthy  -> false, matching production's global default
    //               (`NODE_ENV === 'development'`); repairing an error path is
    //               no reason to add a per-focus request for every viewer.
    //   errored  -> true, but not oftener than the floor. An errored query has
    //               `dataUpdatedAt === 0` and is permanently stale, so an
    //               unthrottled predicate refires on every event, each starting
    //               a fresh retry chain against a rate-limited endpoint.
    // Focus and reconnect share one predicate: the same staleness argument
    // applies to both, and `refetchOnReconnect` defaults to a bare `true`.
    refetchOnWindowFocus: refetchFailedProfileOnly,
    refetchOnReconnect: refetchFailedProfileOnly,
    retry: (failureCount, error) => {
      // Check if it's an AuthError or has status property
      const authError =
        error instanceof AuthError ? error : AuthError.fromUnknown(error)

      // Don't retry when the failure is the backend ANSWERING "no session".
      // Shared with the SSR prefetch and AuthContext so all three agree on
      // which failures are terminal; this used to be a local code-plus-403
      // test, which called a bodyless 401 retryable while AuthContext called
      // the same response definitive.
      // 403 is tested separately from the identity predicate on purpose: it is
      // not an answer about who the viewer is (see isDefinitiveUnauthenticated),
      // but it is still pointless to retry.
      if (
        isDefinitiveUnauthenticated(authError.status, authError.code) ||
        authError.status === 403
      ) {
        authLogger.debug('Profile fetch auth error, not retrying', {
          code: authError.code,
        })
        return false
      }

      return failureCount < 2
    },
  })
}

export const useProfile = () => {
  const queryClient = useQueryClient()
  return useQuery(profileQueryOptions(queryClient))
}

// Update profile identity fields (username, display name, location, bio;
// first/last accepted for legacy callers)
interface UpdateProfileInput {
  username?: string
  display_name?: string
  first_name?: string
  last_name?: string
  location?: string
  bio?: string
  // Nav chrome preference (PSY-1117). Sent on its own from the appearance
  // settings toggle; the backend PATCH validates it against 'top' | 'side'.
  nav_mode?: NavMode
}

interface UpdateProfileResponse {
  success: boolean
  message: string
  error_code?: AuthErrorCodeType
  request_id?: string
  user?: UserProfile['user']
}

export const useUpdateProfile = () => {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (
      input: UpdateProfileInput
    ): Promise<UpdateProfileResponse> => {
      authLogger.debug('Updating profile')

      const response = await apiRequest<UpdateProfileResponse>(
        API_ENDPOINTS.AUTH.PROFILE,
        {
          method: 'PATCH',
          body: JSON.stringify(input),
          credentials: 'include',
        }
      )

      if (!response.success) {
        throw new AuthError(
          response.message || 'Failed to update profile',
          response.error_code || AuthErrorCode.UNKNOWN,
          {
            requestId: response.request_id,
            status: 400,
          }
        )
      }

      return response
    },
    onSuccess: async () => {
      authLogger.info('Profile updated successfully')
      // Refetch profile to get updated user data
      // eslint-disable-next-line no-restricted-syntax -- the profile editor renders only for a signed-in viewer, whose profile holds data, so this restarts an in-flight fetch rather than reusing it (see refetchSignedInProfile)
      await queryClient.refetchQueries({ queryKey: queryKeys.auth.profile })
    },
  })
}

/** The renewal mutation's key in the mutation cache (tests and devtools). */
export const SESSION_RENEWAL_MUTATION_KEY = ['auth', 'session-renewal'] as const

/**
 * Prefix shared by every mutation that changes which session the browser
 * holds. `SessionRenewal` reads it through `useIsMutating` and the renewal
 * itself refuses to start while one is in flight, which is the other half of
 * `settlePendingSessionRenewal`: that call orders a change after a renewal
 * already running; this key keeps a renewal from starting under a change.
 */
export const SESSION_CHANGE_MUTATION_KEY = ['auth', 'session-change'] as const

function sessionChangeKey(change: string) {
  return [...SESSION_CHANGE_MUTATION_KEY, change] as const
}

// The renewal in flight, if any: the request's AbortController and a promise
// that settles with the request. Module state rather than React state because
// the flows that must wait for it run in other components.
let pendingSessionRenewal: { controller: AbortController; settled: Promise<void> } | null = null

/**
 * Aborts a session renewal in flight and waits for it to settle. Every
 * mutation that changes which session the browser holds (sign-in, sign-out,
 * magic link, recovery, reset, set or change password, sign out everywhere
 * else, account deletion) calls this before its own request: the renewal's
 * Set-Cookie would otherwise land after theirs and put the previous session
 * back (a sign-out that does not stick on a shared machine; account A's cookie
 * under account B's magic-link sign-in). Resolves at once when nothing is in
 * flight. Only this tab is covered; another tab's renewal is not.
 */
export async function settlePendingSessionRenewal(): Promise<void> {
  const pending = pendingSessionRenewal
  if (!pending) return
  pending.controller.abort()
  await pending.settled
}

/**
 * Renew the session the browser holds (`SessionRenewal` is the caller). The
 * backend answers a cookie credential with Set-Cookie only (no token in the
 * body), so success leaves nothing to store: it marks the cached profile as no
 * longer due, so `SessionRenewal` stops asking until a later profile read (a
 * page load or the six-hour poll) says due again. A transient failure changes
 * nothing visible: the session the viewer has
 * still works, and the component asks again on the next profile read that
 * says due once SESSION_RENEWAL_RETRY_MS has passed (in a tab left open that
 * read may be the six-hour poll). A 401 refusal
 * (revoked or closed session) is not handled here at all: the global mutation
 * cache handler (`recoverFromSessionExpiry` in lib/queryClient.ts, covered by
 * its "recovers from an expiry that surfaces through a mutation" test) takes
 * the viewer out of the dead session, which is why this hook clears nothing.
 */
export const useRenewSession = () => {
  const queryClient = useQueryClient()

  return useMutation({
    mutationKey: SESSION_RENEWAL_MUTATION_KEY,
    mutationFn: async (): Promise<RefreshTokenResponse> => {
      // A session change already in flight wins: its cookie must land last.
      if (queryClient.isMutating({ mutationKey: SESSION_CHANGE_MUTATION_KEY }) > 0) {
        return { success: false, message: 'Session renewal abandoned for a session change' }
      }
      authLogger.debug('Session renewal attempt')
      // Only one renewal is tracked; an earlier one still hanging is cut off
      // so that settlePendingSessionRenewal cannot miss it.
      pendingSessionRenewal?.controller.abort()
      const controller = new AbortController()
      const request = apiRequest<RefreshTokenResponse>(API_ENDPOINTS.AUTH.REFRESH, {
        method: 'POST',
        credentials: 'include',
        signal: controller.signal,
      })
      const pending = { controller, settled: request.then(() => undefined, () => undefined) }
      pendingSessionRenewal = pending
      try {
        return await request
      } catch (error) {
        // Abandoned for a session change (settlePendingSessionRenewal): not a
        // failure, and nothing to recover from.
        if (controller.signal.aborted) {
          return { success: false, message: 'Session renewal abandoned for a session change' }
        }
        throw error
      } finally {
        if (pendingSessionRenewal === pending) pendingSessionRenewal = null
      }
    },
    onSuccess: data => {
      authLogger.tokenRefresh(data.success, data.request_id)
      if (data.success) {
        queryClient.setQueryData<UserProfile>(queryKeys.auth.profile, previous =>
          previous ? { ...previous, session_renewal_due: false } : previous
        )
      }
    },
    onError: error => {
      authLogger.error('Session renewal failed', error)
    },
  })
}

// `useIsAuthenticated` lives in ./useIsAuthenticated: it reads AuthContext,
// which imports this module.

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
  user?: AuthApiUser
  /**
   * For a link that connected Google, where the sign-in that started it was
   * headed, a path on the frontend.
   */
  return_to?: string
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
    mutationKey: sessionChangeKey('confirm-verification'),
    mutationFn: async (token: string): Promise<VerificationResponse> => {
      await settlePendingSessionRenewal()
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
      // Refetch profile to update email_verified status. Verification is a
      // privilege gain, so the viewer-tier caches go with it.
      await refreshCachesForNewSession(queryClient)
    },
  })
}

// Change password mutation
export const useChangePassword = () => {
  return useMutation({
    mutationKey: sessionChangeKey('change-password'),
    mutationFn: async (
      credentials: ChangePasswordCredentials
    ): Promise<ChangePasswordResponse> => {
      await settlePendingSessionRenewal()
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

type SetPasswordResponse = components['schemas']['SetPasswordResponseBody']

/**
 * Set a first password on an account that has none. A refusal the response
 * carries (a password the validator rejects, one already set, a closed
 * account) rejects with an AuthError carrying its code; a stale session
 * rejects with REAUTH_REQUIRED through apiRequest's 403 handling.
 *
 * On success the profile is refetched, since it carries `has_password`.
 */
export const useSetPassword = () => {
  const queryClient = useQueryClient()

  return useMutation({
    mutationKey: sessionChangeKey('set-password'),
    mutationFn: async (newPassword: string): Promise<SetPasswordResponse> => {
      await settlePendingSessionRenewal()
      authLogger.debug('Set password attempt')

      const response = await apiRequest<SetPasswordResponse>(
        API_ENDPOINTS.AUTH.SET_PASSWORD,
        {
          method: 'POST',
          body: JSON.stringify({ new_password: newPassword }),
          credentials: 'include',
        }
      )

      if (!response.success) {
        authLogger.warn(
          'Set password refused',
          { errorCode: response.error_code, message: response.message },
          response.request_id
        )
        throw new AuthError(
          response.message || 'Failed to set password',
          (response.error_code as AuthErrorCodeType) || AuthErrorCode.UNKNOWN,
          { requestId: response.request_id, status: 400 }
        )
      }

      return response
    },
    onSuccess: data => {
      authLogger.info('Password set', { message: data.message }, data.request_id)
      // Not awaited, so the mutation settles, and its callers' onSuccess runs,
      // without waiting on the refetch.
      void refetchSignedInProfile(queryClient)
    },
  })
}

type SignOutEverywhereElseResponse =
  components['schemas']['SignOutEverywhereElseResponseBody']

/**
 * Sign out every other session the account holds. The response renews this
 * browser's session cookie in the same step, so the signed-in profile is
 * unchanged and is not refetched. The same step deletes the calendar feed
 * link, so the calendar caches are refreshed. Any failure rejects: a 5xx
 * means no session was signed out, and a 401 means this session had itself
 * been signed out by then and its cookie is cleared.
 */
export const useSignOutEverywhereElse = () => {
  const queryClient = useQueryClient()

  return useMutation({
    mutationKey: sessionChangeKey('sign-out-everywhere-else'),
    mutationFn: async (): Promise<SignOutEverywhereElseResponse> => {
      await settlePendingSessionRenewal()
      const response = await apiRequest<SignOutEverywhereElseResponse>(
        API_ENDPOINTS.AUTH.SIGN_OUT_EVERYWHERE_ELSE,
        { method: 'POST', credentials: 'include' }
      )
      if (!response.success) {
        throw new AuthError(
          'Failed to sign out other devices',
          AuthErrorCode.UNKNOWN,
          { requestId: response.request_id }
        )
      }
      return response
    },
    onSuccess: data => {
      authLogger.info('Signed out everywhere else', undefined, data.request_id)
      // Not awaited: the mutation settles without waiting on the refetch.
      void refreshCalendarAfterLinkRevocation(queryClient)
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
    mutationKey: sessionChangeKey('verify-magic-link'),
    mutationFn: async (token: string): Promise<VerifyMagicLinkResponse> => {
      await settlePendingSessionRenewal()
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
        // Bring the profile query, which every consumer reads, up to date
        // with the session this response just established.
        await refreshCachesForNewSession(queryClient)
      }
    },
  })
}

type RequestPasswordResetResponse =
  components['schemas']['RequestPasswordResetResponseBody']
type SendOwnPasswordResetResponse =
  components['schemas']['SendOwnPasswordResetResponseBody']
type CheckPasswordResetResponse =
  components['schemas']['CheckPasswordResetResponseBody']
type CompletePasswordResetResponse = Omit<
  components['schemas']['CompletePasswordResetResponseBody'],
  'user'
> & { user?: AuthApiUser }

/**
 * Ask for the emailed password reset link for an address. The answer is the
 * same whether or not an account uses it; a malformed address (400), a server
 * with no email provider (503) and a throttle (429) reject.
 */
export const useRequestPasswordReset = () => {
  return useMutation({
    mutationFn: async (email: string): Promise<RequestPasswordResetResponse> => {
      authLogger.debug('Password reset request', {
        email: email.slice(0, 2) + '***',
      })
      return apiRequest<RequestPasswordResetResponse>(
        API_ENDPOINTS.AUTH.PASSWORD_RESET_REQUEST,
        {
          method: 'POST',
          body: JSON.stringify({ email }),
          credentials: 'include',
        }
      )
    },
  })
}

/**
 * Send the emailed password reset link to the signed-in account's own
 * address. Resolves once it is queued; any failure, a throttle included,
 * rejects, so nothing is confirmed that was not queued.
 */
export const useSendOwnPasswordReset = () => {
  return useMutation({
    mutationFn: async (): Promise<SendOwnPasswordResetResponse> =>
      apiRequest<SendOwnPasswordResetResponse>(
        API_ENDPOINTS.AUTH.PASSWORD_RESET_SEND_OWN,
        { method: 'POST', credentials: 'include' }
      ),
  })
}

/**
 * Raises a reset route's body-encoded refusal as an AuthError carrying its
 * code. Status 400 marks it as an answer about the link or the password, so
 * the page tells it apart from a 5xx or a transport failure, which carry
 * their own status or none.
 */
function throwPasswordResetRefusal(response: {
  message?: string
  error_code?: string
  request_id?: string
}): never {
  throw new AuthError(
    response.message || 'This password reset link no longer works.',
    (response.error_code as AuthErrorCodeType) || AuthErrorCode.UNKNOWN,
    { requestId: response.request_id, status: 400 }
  )
}

/**
 * Whether an emailed reset link may still set a password, and for which
 * address. The check writes nothing, so it is a query: opening the link spends
 * nothing, and a retry is a refetch. It runs once per token and is not kept
 * after the page leaves, so the link does not outlive the page in the cache.
 */
export const useCheckPasswordReset = (token: string | null) => {
  return useQuery({
    queryKey: queryKeys.auth.passwordResetCheck(token ?? ''),
    queryFn: async (): Promise<CheckPasswordResetResponse> => {
      const response = await apiRequest<CheckPasswordResetResponse>(
        API_ENDPOINTS.AUTH.PASSWORD_RESET_CHECK,
        {
          method: 'POST',
          body: JSON.stringify({ token }),
          credentials: 'include',
        }
      )
      if (!response.success) throwPasswordResetRefusal(response)
      return response
    },
    enabled: !!token,
    retry: false,
    staleTime: Infinity,
    gcTime: 0,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  })
}

/**
 * Set a new password with an emailed reset link. Success signs every other
 * session out and signs this browser in, so the profile is brought up to
 * date with the new session before the mutation settles. It also deletes the
 * calendar feed link, so the calendar caches are refreshed.
 */
export const useCompletePasswordReset = () => {
  const queryClient = useQueryClient()

  return useMutation({
    mutationKey: sessionChangeKey('complete-password-reset'),
    mutationFn: async ({
      token,
      newPassword,
    }: {
      token: string
      newPassword: string
    }): Promise<CompletePasswordResetResponse> => {
      await settlePendingSessionRenewal()
      const response = await apiRequest<CompletePasswordResetResponse>(
        API_ENDPOINTS.AUTH.PASSWORD_RESET_COMPLETE,
        {
          method: 'POST',
          body: JSON.stringify({ token, new_password: newPassword }),
          credentials: 'include',
        }
      )
      if (!response.success) throwPasswordResetRefusal(response)
      return response
    },
    onSuccess: async data => {
      authLogger.info('Password reset', undefined, data.request_id)
      // Not awaited: its reset is synchronous, and the refetch need not hold
      // the mutation open.
      void refreshCalendarAfterLinkRevocation(queryClient)
      await refreshCachesForNewSession(queryClient)
    },
  })
}

// Account deletion types
interface DeletionSummaryResponse {
  success: boolean
  message: string
  shows_count: number
  saved_shows_count: number
  saved_releases_count: number
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
    mutationKey: sessionChangeKey('delete-account'),
    mutationFn: async (
      request: DeleteAccountRequest
    ): Promise<DeleteAccountResponse> => {
      await settlePendingSessionRenewal()
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
}

// The export endpoint's successful answer is the export document itself, with
// no response envelope. When the handler runs without a user set by the auth
// middleware (a server wiring defect; a missing or expired session is a 401
// from the middleware instead), it answers HTTP 200 with this body instead.
interface ExportDataErrorBody {
  success: false
  error: string
  message: string
}

// `null` is a JSON `null` body; `undefined` is what apiRequest returns for 204.
type ExportDataBody = ExportDataResponse | ExportDataErrorBody | null | undefined

// Only the export document carries `exported_at`; the inline error body never
// does.
function isExportDocument(body: ExportDataBody): body is ExportDataResponse {
  return (
    typeof body === 'object' &&
    body !== null &&
    'exported_at' in body &&
    typeof body.exported_at === 'string'
  )
}

function isExportErrorBody(body: ExportDataBody): body is ExportDataErrorBody {
  return (
    typeof body === 'object' &&
    body !== null &&
    'success' in body &&
    body.success === false
  )
}

// Export user data mutation (GDPR Right to Portability)
export const useExportData = () => {
  return useMutation({
    mutationFn: async (): Promise<ExportDataResponse> => {
      authLogger.debug('Exporting user data')

      const body = await apiRequest<ExportDataBody>(
        API_ENDPOINTS.AUTH.EXPORT_DATA,
        {
          method: 'GET',
          credentials: 'include',
        }
      )

      if (isExportDocument(body)) {
        return body
      }

      // Only the handler's own error body contributes its message; any other
      // non-document body is an unrecognized response.
      const handlerMessage = isExportErrorBody(body) ? body.message : undefined
      throw new AuthError(
        handlerMessage || 'Failed to export data',
        AuthErrorCode.UNKNOWN,
        { status: 400 }
      )
    },
    onSuccess: data => {
      authLogger.info('Data export successful', { exportedAt: data.exported_at })
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

/**
 * Mints the one-time token the connect flow puts on its start URL.
 *
 * Separate from the navigation itself because it has to be a same-origin
 * authenticated request: that is the only thing a page on another site cannot
 * make on the user's behalf and read back, and it is what stands between a
 * cookie-authenticated GET and a cross-site connect.
 */
export const useStartOAuthLink = () => {
  return useMutation({
    mutationFn: async (): Promise<{ token: string }> => {
      const response = await apiRequest<{
        success: boolean
        token: string
      }>(API_ENDPOINTS.AUTH.OAUTH_LINK_TOKEN, {
        method: 'POST',
        credentials: 'include',
      })

      if (!response.success || !response.token) {
        throw new AuthError(
          'Could not start the connection',
          AuthErrorCode.UNKNOWN
        )
      }

      return { token: response.token }
    },
  })
}

// Unlink OAuth account mutation
export const useUnlinkOAuthAccount = () => {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (
      provider: string
    ): Promise<UnlinkOAuthAccountResponse> => {
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
    onSuccess: data => {
      authLogger.info(
        'OAuth account unlinked successfully',
        { message: data.message },
        data.request_id
      )
      // Refetch OAuth accounts list
      void refetchAfterMutation(queryClient, ['auth', 'oauth-accounts'])
    },
  })
}

// Passkey credential management types (settings → security)
export interface PasskeyCredential {
  id: number
  display_name: string
  created_at: string
  last_used_at: string | null
  backup_eligible: boolean
  backup_state: boolean
}

interface PasskeyCredentialsResponse {
  success: boolean
  credentials?: PasskeyCredential[]
  message?: string
  error_code?: string
  request_id?: string
}

interface DeletePasskeyResponse {
  success: boolean
  message?: string
  error_code?: string
  request_id?: string
}

// List the current user's registered passkeys (PSY-1102). Replaces a raw
// fetch-in-effect in PasskeyManagement so the list joins the shared query
// cache and is invalidated after register / delete. `enabled` is caller-gated
// (browserSupportsWebAuthn) so unsupported browsers never issue the request.
export const usePasskeyCredentials = (enabled = true) => {
  return useQuery({
    queryKey: queryKeys.passkeys.credentials,
    queryFn: async (): Promise<PasskeyCredential[]> => {
      authLogger.debug('Fetching passkey credentials')

      const response = await apiRequest<PasskeyCredentialsResponse>(
        API_ENDPOINTS.AUTH.PASSKEY_CREDENTIALS,
        {
          method: 'GET',
          credentials: 'include',
        }
      )

      if (!response.success) {
        throw new AuthError(
          response.message || 'Failed to load passkeys',
          (response.error_code as AuthErrorCodeType) || AuthErrorCode.UNKNOWN,
          {
            requestId: response.request_id,
            status: 400,
          }
        )
      }

      return response.credentials ?? []
    },
    enabled,
  })
}

// Delete one passkey credential, then invalidate the list (PSY-1102).
export const useDeletePasskey = () => {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (credentialId: number): Promise<DeletePasskeyResponse> => {
      authLogger.debug('Deleting passkey credential', { credentialId })

      const response = await apiRequest<DeletePasskeyResponse>(
        API_ENDPOINTS.AUTH.PASSKEY_CREDENTIAL(credentialId),
        {
          method: 'DELETE',
          credentials: 'include',
        }
      )

      if (!response.success) {
        throw new AuthError(
          response.message || 'Failed to delete passkey',
          (response.error_code as AuthErrorCodeType) || AuthErrorCode.UNKNOWN,
          {
            requestId: response.request_id,
            status: 400,
          }
        )
      }

      return response
    },
    // Optimistically drop the row so the delete feels instant (matching the
    // pre-TanStack local-filter behavior) AND so the now-removed row can't be
    // clicked a second time while its DELETE is in flight — which would fire a
    // duplicate DELETE that 404s and surfaces a spurious failure (PSY-1102
    // adversarial review). Snapshot for rollback on error.
    onMutate: async (credentialId: number) => {
      await queryClient.cancelQueries({ queryKey: queryKeys.passkeys.credentials })
      const previous = queryClient.getQueryData<PasskeyCredential[]>(
        queryKeys.passkeys.credentials
      )
      queryClient.setQueryData<PasskeyCredential[]>(
        queryKeys.passkeys.credentials,
        old => (old ?? []).filter(c => c.id !== credentialId)
      )
      return { previous }
    },
    onError: (_error, _credentialId, context) => {
      // Roll back the optimistic removal so the row reappears on failure.
      if (context?.previous !== undefined) {
        queryClient.setQueryData(queryKeys.passkeys.credentials, context.previous)
      }
    },
    onSuccess: data => {
      authLogger.info(
        'Passkey deleted successfully',
        { message: data.message },
        data.request_id
      )
    },
    onSettled: () => {
      // Reconcile with server truth after the mutation resolves either way.
      // The profile carries the passkey count, so it is refetched too.
      void refetchAfterMutation(queryClient, queryKeys.passkeys.credentials)
      void refetchSignedInProfile(queryClient)
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
  user?: AuthApiUser
  error_code?: AuthErrorCodeType
  request_id?: string
}

interface RequestAccountRecoveryRequest {
  email: string
}

// RequestAccountRecoveryResponse — enumeration-safe. The backend never
// returns `has_password` or `days_remaining` here: those fields would leak
// account state pre-token-confirmation. Treat the response as a single
// generic "if eligible, sent" acknowledgement.
interface RequestAccountRecoveryResponse {
  success: boolean
  message: string
  error_code?: string
  request_id?: string
}

interface ConfirmAccountRecoveryResponse {
  success: boolean
  message: string
  user?: AuthApiUser
  error_code?: AuthErrorCodeType
  request_id?: string
}

// Recover account with password mutation
export const useRecoverAccount = () => {
  const queryClient = useQueryClient()

  return useMutation({
    mutationKey: sessionChangeKey('recover-account'),
    mutationFn: async (
      request: RecoverAccountRequest
    ): Promise<RecoverAccountResponse> => {
      await settlePendingSessionRenewal()
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
        // Bring the profile query, which every consumer reads, up to date
        // with the session this response just established.
        await refreshCachesForNewSession(queryClient)
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

      // Response is enumeration-safe — the body is the same generic
      // "if eligible, sent" acknowledgement regardless of account state.
      return response
    },
    onSuccess: data => {
      authLogger.info(
        'Account recovery request processed',
        {
          message: data.message,
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
    mutationKey: sessionChangeKey('confirm-account-recovery'),
    mutationFn: async (
      token: string
    ): Promise<ConfirmAccountRecoveryResponse> => {
      await settlePendingSessionRenewal()
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
        // Bring the profile query, which every consumer reads, up to date
        // with the session this response just established.
        await refreshCachesForNewSession(queryClient)
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

// Re-export APIToken type from types
export type { APIToken }

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
      void refetchAfterMutation(queryClient, ['admin', 'api-tokens'])
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
    onSuccess: data => {
      authLogger.info('API token revoked successfully', {
        message: data.message,
      })
      void refetchAfterMutation(queryClient, ['admin', 'api-tokens'])
    },
  })
}
