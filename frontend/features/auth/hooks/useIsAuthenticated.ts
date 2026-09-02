'use client'

import { useAuthContext } from '@/lib/context/AuthContext'

/**
 * Compatibility seam for the five feature-detail components that already call
 * this name (Artist / Release / Label / Festival detail, RelatedArtists). NEW
 * code calls `useAuthContext()` directly; this exists so those five did not
 * have to change to stop re-deriving auth.
 *
 * It must stay a pass-through and must NOT hold a profile-query observer of its
 * own: a second derivation is what let a page paint anonymous chrome beside a
 * disabled bracket, and an observer here is what `resetViewerTierQueries`
 * (lib/queryClient.ts) documents as orphaned by `queryClient.clear()`.
 *
 * `user` is null (not undefined) for a viewer with no session, and `error` is
 * the context's message string rather than the raw query error.
 */
export const useIsAuthenticated = () => {
  const { user, isAuthenticated, authStatus, isLoading, error } =
    useAuthContext()

  return { isAuthenticated, authStatus, isLoading, user, error }
}
