'use client'

import {
  createContext,
  useContext,
  useState,
  useMemo,
  useCallback,
  ReactNode,
} from 'react'
import { useProfile, useLogout } from '@/features/auth'
import type { UserTier } from '@/features/auth'
import type { NavMode } from '@/lib/nav-mode'
import { AuthError } from '@/lib/errors'

interface User {
  id: string
  email: string
  username?: string
  display_name?: string
  first_name?: string
  last_name?: string
  bio?: string
  // Free-text "City, state" (PSY-1416). Optional on the public profile meta line.
  location?: string
  // OAuth / profile avatar URL (PSY-1488). Passed through from /auth/profile.
  avatar_url?: string
  email_verified: boolean
  is_admin?: boolean
  user_tier?: UserTier
  // Saved nav-style preference (PSY-1117). Read by the appearance settings
  // toggle to seed its control; the server shell (AppShell) reads it directly
  // from the profile for first-paint rendering.
  nav_mode?: NavMode
}

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
 * The guarantee, stated exactly, because a weaker version of this sentence was
 * wrong once already: 'anonymous' is reached only from a DEFINITIVE answer:
 * the profile query resolved with no user, or it failed with a 401/403, which
 * is the backend saying this viewer has no session. A failure that is not an
 * answer (5xx, network, unknown) reads 'pending', and so does an SSR prefetch
 * that could not reach the backend, which now seeds nothing rather than
 * fabricating a logged-out payload (see lib/auth-hydration.ts).
 *
 * That distinction is load-bearing rather than pedantic. Anonymous-on-failure
 * is forgeable by a single transient 5xx, and it does not heal: production has
 * `refetchOnWindowFocus: false`, and `AuthProvider` mounts once in the root
 * layout, so a fabricated 'anonymous' would survive for the whole SPA session
 * and make every gate built on this primitive wrong at once.
 */
export type AuthStatus = 'pending' | 'authenticated' | 'anonymous'

interface AuthState {
  user: User | null
  isAuthenticated: boolean
  /** Settled-auth signal. See {@link AuthStatus} before gating behavior on it. */
  authStatus: AuthStatus
  isLoading: boolean
  error: string | null
}

interface AuthContextType extends AuthState {
  setUser: (user: User | null) => void
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

  // Derive user from profile data or override
  const user = useMemo(() => {
    // If there's an explicit user override (truthy), use it.
    // Note: null means "no override" - logout clears via queryClient.clear().
    // Login/signup build the override from the minimal auth response, which
    // omits nav_mode; backfill it from the full profile so the appearance
    // settings control (PSY-1117) seeds from the saved preference for the rest
    // of the SPA session, not the default. The override still wins for every
    // field it actually sets.
    if (userOverride) {
      return {
        ...userOverride,
        nav_mode: userOverride.nav_mode ?? profileData?.user?.nav_mode,
      }
    }

    // Otherwise derive from profile data
    if (profileData?.success && profileData?.user) {
      return {
        id: profileData.user.id,
        email: profileData.user.email,
        username: profileData.user.username,
        display_name: profileData.user.display_name,
        first_name: profileData.user.first_name,
        last_name: profileData.user.last_name,
        bio: profileData.user.bio,
        location: profileData.user.location,
        avatar_url: profileData.user.avatar_url,
        email_verified: profileData.user.email_verified ?? false,
        is_admin: profileData.user.is_admin,
        user_tier: profileData.user.user_tier as UserTier | undefined,
        nav_mode: profileData.user.nav_mode,
      }
    }

    return null
  }, [profileData, userOverride])

  // Derive the settled-auth signal. Order is load-bearing:
  //
  // 1. A user we already hold is an answer, whatever the query is doing — the
  //    login/signup override lands before the profile refetch it triggers, and
  //    a background refetch of a cached profile must not re-open the window.
  // 2. A logout in flight is a TRANSITION, not an answer, so it may resolve
  //    DOWN to 'pending' but never up to a settled 'anonymous'. Be precise
  //    about what this clause does and does not do: in the ordinary logout it
  //    is NOT the thing holding the line. `logout()` clears only the OVERRIDE,
  //    so `user` falls back to the still-cached profile and clause 1 answers
  //    'authenticated' until `useLogout`'s onSuccess reaches
  //    `queryClient.clear()`, at which point `isProfilePending` covers the gap
  //    on its own. The clause is insurance against that ordering changing: it
  //    makes "a logout is running" sufficient by itself to hold the pending
  //    posture, rather than something merely implied by two other facts.
  //    Either way both unresolved cases collapse to 'pending', which is the
  //    conservative direction: consumers keep their disabled/loading posture.
  // 3. Only a profile query that has actually resolved without a user yields
  //    'anonymous'.
  // 4. A profile query that FAILED has not answered either, unless the failure
  //    is itself the answer. A 401/403 (expired, missing or invalid token) is
  //    the backend saying "this viewer has no session", which settles to
  //    'anonymous'. A 5xx, a network failure or an unknown error is the
  //    backend failing to say anything, and must not be read as "nobody".
  //    Classified by STATUS first and error code second, and that order is the
  //    fix for a trap rather than a stylistic choice. `apiRequest` throws
  //    `AuthError` with `code = errorBody.error_code || UNAUTHORIZED` on a
  //    401/403, and `shouldRedirectToLogin` only covers the expired / missing /
  //    invalid token codes. The backend does send TOKEN_MISSING today, so a
  //    code-only test happens to work, but any 401 without that body (a proxy,
  //    a gateway, a future handler) would then read as "not definitive" and
  //    strand a genuinely anonymous viewer at 'pending' forever, with the
  //    bracket permanently disabled. A 401 or 403 from the profile endpoint IS
  //    the backend answering "no valid session", whatever it attaches to it.
  //    `shouldRedirectToLogin` is kept as the second arm so this agrees with the
  //    predicate `useProfile`'s retry policy uses to call a failure terminal.
  const profileErrorIsDefinitive = useMemo(() => {
    if (!profileError) return false
    const authError =
      profileError instanceof AuthError
        ? profileError
        : AuthError.fromUnknown(profileError)
    return (
      authError.status === 401 ||
      authError.status === 403 ||
      authError.shouldRedirectToLogin
    )
  }, [profileError])

  const authStatus: AuthStatus = useMemo(() => {
    if (user) return 'authenticated'
    if (isProfilePending || logoutMutation.isPending) return 'pending'
    if (profileError && !profileErrorIsDefinitive) return 'pending'
    return 'anonymous'
  }, [
    user,
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

  const setUser = useCallback((newUser: User | null) => {
    setUserOverride(newUser)
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
