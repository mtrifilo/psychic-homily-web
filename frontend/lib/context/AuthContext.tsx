'use client'

import {
  createContext,
  useContext,
  useState,
  useMemo,
  useCallback,
  ReactNode,
} from 'react'
// Concrete module paths, not the `@/features/auth` barrel. `useIsAuthenticated`
// lives in that barrel and reads this context, so the barrel import would close
// a cycle through it.
import { useProfile, useLogout } from '@/features/auth/hooks/useAuth'
import { toAuthUser, type AuthApiUser, type User } from '@/features/auth/authUser'
import { AuthError, isDefinitiveUnauthenticated } from '@/lib/errors'

/**
 * Whether the viewer's identity is KNOWN yet, and what it turned out to be.
 *
 * `isAuthenticated` cannot answer "is this viewer anonymous?" on its own: it
 * is false in two situations that call for opposite behavior — the viewer
 * really is anonymous, and the viewer is signed in but their profile has not
 * arrived yet. `isLoading` does not separate them either, because it is false
 * BEFORE the profile query starts fetching, so the earliest renders read as a
 * settled anonymous viewer.
 *
 * Rendering a spinner off that ambiguity is harmless. Changing behavior off it
 * is not: an earlier attempt skipped a request for "anonymous" viewers, which
 * shipped an enabled control during the signed-in-but-pending window and
 * bounced logged-in users to /auth. Anything that acts on "this viewer is
 * anonymous" must gate on `authStatus === 'anonymous'`.
 *
 * What 'pending' has to suppress is narrower than "everything an anonymous
 * viewer would see". A control that NAMES a viewer (an avatar, an email, a
 * personal count, an account menu, a CTA offered only to signed-in users)
 * requires `authStatus === 'authenticated'`.
 *
 * A plain NAVIGATION affordance does not. A sign-in link says nothing about
 * who is looking, and reaching 'pending' takes a session cookie the backend
 * did not answer for (lib/auth-hydration.ts settles a viewer holding no
 * cookie without asking anyone), so the viewer may be carrying a dead session
 * and needing exactly that route. Withholding it strands them.
 *
 * That carve-out is for a link, NOT for an action control whose sign-in
 * WORDING is itself the claim. `SaveButton`, `UserFollowButton` and
 * `NotifyMeButton` are disabled while pending and keep their neutral names on
 * purpose, because "Sign in to save" on a control that cannot act names the
 * viewer and points at a dead end. Do not carry this paragraph over to them.
 *
 * The guarantee, stated exactly, because a weaker version of this sentence was
 * wrong once already: 'anonymous' is reached only from a DEFINITIVE answer:
 * the profile query resolved with no user, or it failed with a 401, which is
 * the backend saying this viewer has no session. A failure that is not an
 * answer (5xx, network, unknown, and 403, which means forbidden rather than
 * unidentified) reads 'pending', and so does an SSR prefetch
 * that could not reach the backend, which now seeds nothing rather than
 * fabricating a logged-out payload (see lib/auth-hydration.ts).
 *
 * That distinction is load-bearing rather than pedantic. Anonymous-on-failure
 * is forgeable by a single transient 5xx, and before this ticket it could not
 * heal: the profile query inherited the global `refetchOnWindowFocus`, which is
 * development-only, and `AuthProvider` mounts once in the root layout, so a
 * fabricated 'anonymous' survived for the whole SPA session and made every gate
 * built on this primitive wrong at once. The query now sets its own throttled
 * focus refetch (features/auth/hooks/useAuth.ts), which is what gives an
 * unresolved read a way back.
 */
export type AuthStatus = 'pending' | 'authenticated' | 'anonymous'

/**
 * THE WRITE-SIDE RULE, stated once: a query whose cache key carries the
 * viewer's identity sets `enabled: authStatus !== 'pending'`.
 *
 * Every such key is built from `isAuthenticated` / `user?.id`, both of which
 * read "anonymous" while the profile is in flight, and a request issued then
 * still carries the viewer's cookie. The response is therefore THEIR data,
 * written under the viewer-less key, where it outlives the session that
 * produced it — session expiry clears no cache.
 *
 * The guard belongs in the query's own `enabled`, never in a caller's: the key
 * is shared by every observer of it and by the mutations that write it
 * optimistically, so a guard held by one component cannot protect it.
 *
 * Settled-ANONYMOUS is a separate, per-query call: skip the request only when
 * the response says nothing the control paints (`is_following` for a viewer
 * with no session says nothing; a public save count does).
 *
 * Holders: `useFollowStatus`, `useBatchFollowStatus`, `useUserFollowStatus`,
 * `useShowSaveCount`, `useShowSaveCountBatch`, `useReleaseSaveCount`,
 * `useReleaseSaveCountBatch`.
 */

interface AuthState {
  user: User | null
  isAuthenticated: boolean
  /** Settled-auth signal. See {@link AuthStatus} before gating behavior on it. */
  authStatus: AuthStatus
  isLoading: boolean
  error: string | null
}

interface AuthContextType extends AuthState {
  /**
   * Claim an authenticated viewer from a session-entry response. Takes the API
   * payload rather than a context {@link User}, so the mapping runs in one
   * place and every claim has the same fields (PSY-1945).
   *
   * The claim is read while the profile names no viewer, and while it names a
   * different one; for the same viewer the profile wins, and a definitive
   * failure outranks it entirely. Only `logout()` and `null` remove it, so a
   * claim outlives a session expiry that is later healed by a profile naming
   * someone else. Telling that apart wants a recency signal rather than the id
   * comparison below.
   */
  setUser: (user: AuthApiUser | null) => void
  setError: (error: string | null) => void
  clearError: () => void
  logout: () => void
}

const AuthContext = createContext<AuthContextType | undefined>(undefined)

interface AuthProviderProps {
  children: ReactNode
}

export function AuthProvider({ children }: AuthProviderProps) {
  // Local state for manual user/error overrides (e.g., after signup)
  const [userOverride, setUserOverride] = useState<User | null | undefined>(
    undefined
  )
  const [errorOverride, setErrorOverride] = useState<string | null>(null)

  // Use the useProfile hook to get authentication status.
  //
  // `isPending` rather than `isLoading` is what makes `authStatus` a SETTLED
  // signal: `isLoading` is `isPending && isFetching`, so it reads false in the
  // window between mount and the fetch actually starting. `isPending` is true
  // from the first render until the query resolves to data or an error, which
  // is exactly "we do not know who this viewer is yet".
  const {
    data: profileData,
    isPending: isProfilePending,
    isLoading,
    error: profileError,
  } = useProfile()
  const logoutMutation = useLogout()

  // Is the profile query's failure itself the answer?
  //
  // A 401 (or one of the token error codes) is the backend saying "this viewer
  // has no session", which settles. A 5xx, a network failure, an unknown error
  // or a 403 is the backend failing to say who the viewer is, and must not be
  // read as "nobody".
  //
  // The test for "definitive" is NOT written here. It lives in
  // `isDefinitiveUnauthenticated`, shared with the SSR prefetch, `useProfile`'s
  // retry policy and the query cache's error handler. Local copies of this
  // decision had already drifted apart once (status-only on the server,
  // code-only in the retry policy), so the same bodyless 401 could settle a
  // viewer as logged out in one place while another kept retrying it.
  //
  // Declared before `user` because `user` depends on it: see the note there.
  const profileErrorIsDefinitive = useMemo(() => {
    if (!profileError) return false
    const authError =
      profileError instanceof AuthError
        ? profileError
        : AuthError.fromUnknown(profileError)
    return isDefinitiveUnauthenticated(authError.status, authError.code)
  }, [profileError])

  // Derive user from profile data or override
  const user = useMemo(() => {
    // FIRST, ahead of the override. TanStack retains the last successful `data`
    // when a refetch errors, and `userOverride` is set by every in-session
    // login and cleared only by `logout()`, so both survive an expiry. Checking
    // either before this one reports a viewer as signed in after the backend
    // has answered that they are not. The override exists to bridge the window
    // BEFORE the profile lands, where `profileError` is null, so it loses
    // nothing by yielding here.
    if (profileErrorIsDefinitive) {
      return null
    }

    // Which source wins turns on whether the two name the SAME viewer.
    //
    // For one viewer the profile is the fresher source: it is refetched on
    // every session entry and by `useUpdateProfile`, while the override is set
    // once at login and cleared only by logout or a definitive failure.
    // Letting the override win pins whatever it held, so a saved nav_mode
    // (PSY-1117), a tier change and an email verification could each land in
    // the profile and never reach a consumer.
    //
    // For DIFFERENT viewers the override is the newer session and the profile
    // is a payload TanStack retained from the previous one. `/auth/magic-link`
    // and `/auth/recover` accept a token for another account with a session
    // already open, and the profile refetch their mutations await resolves
    // rather than throwing, so a 5xx or 429 there leaves the old viewer's
    // payload in place. Reading it would run the app as the previous account.
    //
    // Same mapper on both sides, so the two describe a viewer in one shape.
    if (profileData?.success && profileData?.user) {
      const profileUser = toAuthUser(profileData.user)
      // Compared as strings because the declared `id: string` is narrower than
      // the wire, which sends a number (see AuthApiUser): both sides pass
      // through `toAuthUser` unconverted today, and a future normalization on
      // one side alone would otherwise make every id look different.
      if (userOverride && String(userOverride.id) !== String(profileUser.id)) {
        return userOverride
      }
      return profileUser
    }

    // No profile answer yet, or one that names no viewer. `null` means "no
    // override"; logout clears it alongside the query cache.
    if (userOverride) {
      return userOverride
    }

    return null
  }, [profileData, userOverride, profileErrorIsDefinitive])

  // Cells, in the clause order below. `user` is already null whenever
  // `profileErrorIsDefinitive` (see the memo above), so clauses 1 and 2 cannot
  // both match.
  //
  //   user truthy (override, or profile data w/o definitive error)  authenticated
  //   definitive failure (401 / token code), any cached data        anonymous
  //   query never resolved, or logout in flight                     pending
  //   payload received, latest refetch failed indefinitely          anonymous
  //   never resolved, failed indefinitely                           pending
  //   residue (unreachable: query is always pending/success/error)  pending
  //
  // Two cells are the ones prior revisions got wrong, in opposite directions:
  // a definitive failure must outrank retained data, and a payload already
  // received must NOT be un-answered by a failed background refetch.
  //
  // The logout clause does not outrank a definitive failure: a refetch that
  // 401s mid-logout yields 'anonymous', which is the same answer the logout is
  // heading toward.
  const authStatus: AuthStatus = useMemo(() => {
    if (user) return 'authenticated'
    if (profileErrorIsDefinitive) return 'anonymous'
    if (isProfilePending || logoutMutation.isPending) return 'pending'
    if (profileData) return 'anonymous'
    if (profileError) return 'pending'
    // Residue defaults to the conservative answer, matching every other
    // unresolved cell, so a future `enabled`/`select` on the profile query
    // cannot silently settle a viewer as anonymous.
    return 'pending'
  }, [
    user,
    profileData,
    isProfilePending,
    logoutMutation.isPending,
    profileError,
    profileErrorIsDefinitive,
  ])

  // Derive error from profile error or override
  const error = useMemo(() => {
    if (errorOverride !== null) {
      return errorOverride
    }
    if (profileError) {
      return profileError.message || 'Authentication failed'
    }
    return null
  }, [profileError, errorOverride])

  const setUser = useCallback((newUser: AuthApiUser | null) => {
    setUserOverride(newUser ? toAuthUser(newUser) : null)
    setErrorOverride(null)
  }, [])

  const setError = useCallback((newError: string | null) => {
    setErrorOverride(newError)
  }, [])

  const logout = useCallback(async () => {
    try {
      setErrorOverride(null)
      setUserOverride(null)
      // The useProfile hook will automatically handle the logout state
      // when the server clears the HTTP-only cookie
      await logoutMutation.mutateAsync()
    } catch (err) {
      // Logout failure is non-critical — cookie will expire naturally
    }
  }, [logoutMutation])

  const clearError = useCallback(() => {
    setErrorOverride(null)
  }, [])

  const value: AuthContextType = useMemo(
    () => ({
      user,
      // Derived from `authStatus` rather than re-tested from `user`, so the
      // two can never disagree about the same viewer.
      isAuthenticated: authStatus === 'authenticated',
      authStatus,
      isLoading: isLoading || logoutMutation.isPending,
      error,
      setUser,
      setError,
      clearError,
      logout,
    }),
    [
      user,
      authStatus,
      isLoading,
      logoutMutation.isPending,
      error,
      setUser,
      setError,
      clearError,
      logout,
    ]
  )

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuthContext() {
  const context = useContext(AuthContext)
  if (context === undefined) {
    throw new Error('useAuthContext must be used within an AuthProvider')
  }
  return context
}
