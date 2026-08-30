'use client'

import { useAuthContext } from '@/lib/context/AuthContext'

/**
 * Auth predicate for feature components that read identity without owning a
 * profile query.
 *
 * A thin adapter over `AuthContext`, and thin on purpose: every value here is
 * DERIVED there, so this hook and the brackets rendered beside its consumers
 * answer from one derivation and cannot disagree about the same viewer. A
 * local re-derivation from the profile query is what let a page paint anonymous
 * chrome next to a disabled bracket.
 *
 * `authStatus` is forwarded because `isAuthenticated` cannot separate "no
 * session" from "profile has not arrived yet": it is false for both. Anything
 * that acts on "this viewer is anonymous" gates on `authStatus === 'anonymous'`
 * and anything auth-gated stays inert while it is `'pending'`. See
 * {@link import('@/lib/context/AuthContext').AuthStatus}.
 *
 * `user` is null (not undefined) for a viewer with no session, and `error` is
 * the context's message string rather than the raw query error.
 */
export const useIsAuthenticated = () => {
  const { user, isAuthenticated, authStatus, isLoading, error } =
    useAuthContext()

  return { isAuthenticated, authStatus, isLoading, user, error }
}
