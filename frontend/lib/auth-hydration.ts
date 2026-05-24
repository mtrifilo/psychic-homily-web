/**
 * Server-only auth profile hydration helper.
 *
 * Pre-seeds the TanStack Query cache for `/auth/profile` on the server so
 * `useProfile()` resolves from cache on first paint. Without this, hydrated
 * detail pages paint instantly but auth-gated action buttons
 * (AttendanceButton, SaveButton, etc.) are interactive while
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

import { cache } from 'react'
import { cookies } from 'next/headers'
import { dehydrate, type DehydratedState } from '@tanstack/react-query'
import * as Sentry from '@sentry/nextjs'
import { getQueryClient, queryKeys } from '@/lib/queryClient'
import { API_BASE_URL } from '@/lib/api-base'
import { AuthErrorCode } from '@/lib/errors'

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
  error_code: AuthErrorCode.TOKEN_MISSING,
}

/**
 * Fetch `/auth/profile` server-side and hydrate the result into a
 * request-scoped QueryClient. Called once per request from the root
 * layout — `getQueryClient` returns a fresh client on the server, so
 * there's no cross-request cache leak. Wrapped in `React.cache()` so
 * multiple server components in the same render can call it without
 * triggering a duplicate backend fetch.
 */
export const prefetchAuthProfile = cache(
  async (): Promise<DehydratedState> => {
    const queryClient = getQueryClient()

    const profile = await fetchAuthProfile()
    await queryClient.prefetchQuery({
      queryKey: queryKeys.auth.profile,
      queryFn: () => profile,
    })

    return dehydrate(queryClient)
  }
)

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
      // 5xx is a real backend outage that would otherwise be invisible
      // — every authenticated viewer silently falls back to the
      // "logged out" sentinel until staleTime elapses and the client
      // refetches. Capture so on-call sees the signal without waiting
      // on user reports. 4xx (401/403) is the expected unauthenticated
      // path and intentionally not captured.
      if (response.status >= 500) {
        Sentry.captureMessage(`SSR auth profile fetch failed: ${response.status}`, {
          level: 'error',
          tags: { service: 'auth', error_type: 'ssr_prefetch_failure' },
          extra: { status: response.status },
        })
      }
      return UNAUTHENTICATED_PROFILE
    }

    return (await response.json()) as AuthProfilePayload
  } catch (error) {
    // Network failure (backend unreachable from the Next server, DNS,
    // etc.) — fall back to the sentinel so first paint isn't an error
    // page, but log so a sustained outage doesn't masquerade as
    // "everyone is logged out".
    Sentry.captureException(error, {
      level: 'error',
      tags: { service: 'auth', error_type: 'ssr_prefetch_network_failure' },
    })
    return UNAUTHENTICATED_PROFILE
  }
}
