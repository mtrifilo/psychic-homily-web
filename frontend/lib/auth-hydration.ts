/**
 * Server-only auth profile hydration helper (PSY-834).
 *
 * Pre-seeds the TanStack Query cache for `/auth/profile` on the server so
 * `useProfile()` resolves from cache on first paint. Without this, hydrated
 * detail pages (PSY-796 / PSY-797) paint instantly but auth-gated action
 * buttons (AttendanceButton, SaveButton, etc.) are interactive while
 * `isAuthenticated` is still `false` — a click before the client profile
 * fetch settles routes the user to `/auth?returnTo=…` instead of firing
 * the intended POST.
 *
 * The helper:
 *   - Reads the viewer's `auth_token` cookie via `next/headers` so the
 *     server fetch sees the same session as the browser would.
 *   - Calls the backend with `cache: 'no-store'` so per-user profile data
 *     never leaks across requests.
 *   - On 401 (or network error) populates the cache with a "no user"
 *     sentinel matching the `UserProfile` body shape the backend returns
 *     for unauthenticated requests. This is what `useProfile`'s queryFn
 *     would resolve to IF apiRequest didn't throw on 401 — the seed lets
 *     the client skip the refetch + auth-error flash entirely.
 *   - Returns `dehydrate(queryClient)` for `<HydrationBoundary>`.
 *
 * Server-only by virtue of importing `next/headers`. Importing this from
 * a client component will throw at build time.
 */

import { cookies } from 'next/headers'
import { dehydrate, type DehydratedState } from '@tanstack/react-query'
import { getQueryClient, queryKeys } from '@/lib/queryClient'
import { API_BASE_URL } from '@/lib/api-base'

// Mirror the relevant subset of `UserProfile` from
// features/auth/hooks/useAuth.ts. Duplicated here (rather than imported)
// because that module is `'use client'` and pulling it into a server
// helper would mark this file as client-only.
interface AuthProfilePayload {
  success: boolean
  message?: string
  error_code?: string
  request_id?: string
  user?: unknown
}

// Sentinel body for the unauthenticated case. Matches the backend's
// /auth/profile 401 body shape so the cache entry is indistinguishable
// from the parsed payload — no special "is this a seed?" branching in
// the client.
const UNAUTHENTICATED_PROFILE: AuthProfilePayload = {
  success: false,
  message: 'Authentication required',
  error_code: 'TOKEN_MISSING',
}

/**
 * Fetch `/auth/profile` server-side and hydrate the result into a
 * request-scoped QueryClient. Called once per request from the root
 * layout — `getQueryClient` returns a fresh client on the server, so
 * there's no cross-request cache leak.
 */
export async function prefetchAuthProfile(): Promise<DehydratedState> {
  const queryClient = getQueryClient()

  const profile = await fetchAuthProfile()
  await queryClient.prefetchQuery({
    queryKey: queryKeys.auth.profile,
    queryFn: () => profile,
  })

  return dehydrate(queryClient)
}

async function fetchAuthProfile(): Promise<AuthProfilePayload> {
  const cookieStore = await cookies()
  const authToken = cookieStore.get('auth_token')

  // Anonymous visitor — short-circuit instead of round-tripping to the
  // backend just to be told there's no session. Same sentinel either
  // way, so the cache entry is identical whether we skip or fetch.
  if (!authToken?.value) {
    return UNAUTHENTICATED_PROFILE
  }

  try {
    const response = await fetch(`${API_BASE_URL}/auth/profile`, {
      headers: { Cookie: `auth_token=${authToken.value}` },
      cache: 'no-store',
    })

    if (!response.ok) {
      // 401, 403, 5xx — fall through to the unauthenticated sentinel so
      // the client doesn't re-fetch and doesn't render an error state on
      // first paint. If the user truly is logged in but the backend is
      // temporarily 5xx, the client's stale-time-elapsed refetch will
      // surface the error normally.
      return UNAUTHENTICATED_PROFILE
    }

    return (await response.json()) as AuthProfilePayload
  } catch {
    // Network failure (backend unreachable from the Next server, DNS,
    // etc.) — same fallback as a non-2xx response. The client will
    // refetch when the user takes an action that touches the query, and
    // the real error will surface there.
    return UNAUTHENTICATED_PROFILE
  }
}
