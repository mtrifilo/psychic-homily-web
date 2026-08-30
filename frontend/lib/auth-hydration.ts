/**
 * Server-only auth profile hydration helper.
 *
 * Pre-seeds the TanStack Query cache for `/auth/profile` on the server so
 * `useProfile()` resolves from cache on first paint. Without this, hydrated
 * detail pages paint instantly but auth-gated action buttons
 * (SaveButton, etc.) are interactive while
 * `isAuthenticated` is still `false` — a click before the client profile
 * fetch settles routes the user to `/auth?returnTo=…` instead of firing
 * the intended POST.
 *
 * The helper:
 *   - Reads the viewer's `auth_token` cookie via `next/headers` so the
 *     server fetch sees the same session as the browser would.
 *   - Calls the backend with `cache: 'no-store'` so per-user profile data
 *     never leaks across requests.
 *   - On a DEFINITIVE unauthenticated answer (no cookie at all, or the
 *     backend replying 401/403) populates the cache with a "no user"
 *     sentinel matching the `UserProfile` body shape the backend returns
 *     for unauthenticated requests. This is what `useProfile`'s queryFn
 *     would resolve to IF apiRequest didn't throw on 401 — the seed lets
 *     the client skip the refetch + auth-error flash entirely.
 *   - On a failure to reach the backend at all (5xx, network) seeds
 *     NOTHING, so the client mounts the query pending and asks again
 *     itself. See {@link AuthProfileResolution} for why the two cases
 *     must not share an answer.
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
import { AuthErrorCode, isDefinitiveUnauthenticated } from '@/lib/errors'

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
 * What the server actually learned about the viewer.
 *
 * The distinction this type exists to preserve: "the backend told us there is
 * no session" is an ANSWER, while "we could not reach the backend" is not one.
 * Both used to collapse into the sentinel above, which made a settled
 * "anonymous" forgeable by any transient 5xx.
 *
 * Why that mattered enough to add a type for. The seeded sentinel is
 * deliberately indistinguishable from a real unauthenticated payload, so
 * nothing downstream could tell a genuine logged-out viewer from a signed-in
 * one whose prefetch happened to fail, and the failure did not self-correct:
 * `refetchOnWindowFocus` is `NODE_ENV === 'development'` (lib/queryClient.ts),
 * so it is OFF in production, and `AuthProvider` mounts once in the root
 * layout, which does not re-render on client navigation. A single blip
 * therefore pinned a signed-in viewer to "anonymous" for the rest of the SPA
 * session, across every page they visited.
 *
 * Returning `indeterminate` seeds NOTHING, so the client query mounts pending
 * and asks again with the viewer's own cookie. If that also fails, staying
 * pending is the correct answer during an outage, not a bug.
 */
type AuthProfileResolution =
  | { kind: 'resolved'; profile: AuthProfilePayload }
  | { kind: 'indeterminate' }

/**
 * Best-effort `error_code` from a failed response, so the shared
 * `isDefinitiveUnauthenticated` test can see the same two inputs the client
 * side gives it. A body that will not parse is not an error here: the status
 * alone already decides every case that matters.
 */
async function readErrorCode(
  response: Response
): Promise<string | undefined> {
  try {
    const body = (await response.json()) as { error_code?: string }
    return body?.error_code
  } catch {
    return undefined
  }
}

/**
 * Fetch `/auth/profile` server-side and hydrate the result into a
 * request-scoped QueryClient. Called once per request from the
 * `<AuthHydrator>` server component — `getQueryClient` returns a fresh
 * client on the server, so there's no cross-request cache leak.
 * Wrapped in `React.cache()` so multiple server components in the same
 * render can call it without triggering a duplicate backend fetch.
 */
export const prefetchAuthProfile = cache(
  async (): Promise<DehydratedState> => {
    const queryClient = getQueryClient()

    const resolution = await fetchAuthProfile()

    // Seed ONLY a definitive answer. An indeterminate result is left unseeded
    // on purpose: the dehydrated state carries no profile entry, the client
    // query mounts `pending`, and it refetches with the viewer's own cookie.
    // Seeding the unauthenticated sentinel here instead would hand the client
    // a fabricated answer it cannot tell from a real one. See
    // {@link AuthProfileResolution}.
    if (resolution.kind === 'resolved') {
      const { profile } = resolution
      await queryClient.prefetchQuery({
        queryKey: queryKeys.auth.profile,
        queryFn: () => profile,
      })
    }

    return dehydrate(queryClient)
  }
)

/**
 * Resolve the authenticated viewer's saved nav-mode preference server-side, or
 * `undefined` when there's no session (anonymous or expired) and equally when
 * the read could not be completed (backend outage). Those are different facts
 * now, but not to this reader: either way there is no saved preference to
 * apply, so both collapse to `undefined` here as they always did. AppShell
 * reads this so a
 * logged-in viewer renders their cross-device preference on first paint with no
 * flash, even on a brand-new browser where the `nav_mode` cookie isn't set yet
 * (PSY-1117). Shares `fetchAuthProfile`'s `React.cache()` with the
 * `<AuthHydrator>` prefetch, so calling it adds no extra backend round-trip
 * within a single render.
 *
 * Returns the raw string (not a coerced NavMode) — the caller owns coercion via
 * `parseNavMode`, keeping this helper agnostic of the cookie-layer contract.
 */
export async function getAuthenticatedNavMode(): Promise<string | undefined> {
  const resolution = await fetchAuthProfile()
  // Indeterminate collapses to `undefined` exactly as a failed fetch already
  // did here. This reader only wants a saved preference to avoid a flash; with
  // no answer available the caller's default is the right fallback, and the
  // client corrects it once the profile query settles.
  if (resolution.kind !== 'resolved') return undefined
  const { profile } = resolution
  if (!profile.success) return undefined
  const user = profile.user as { nav_mode?: unknown } | undefined
  return typeof user?.nav_mode === 'string' ? user.nav_mode : undefined
}

// React.cache()-wrapped so the `<AuthHydrator>` prefetch and AppShell's
// nav-mode read in the same render share a single backend fetch (request-scoped
// dedup; getQueryClient already returns a fresh client per server request, so
// there's no cross-request leak).
const fetchAuthProfile = cache(async (): Promise<AuthProfileResolution> => {
  const cookieStore = await cookies()
  const authToken = cookieStore.get('auth_token')

  // Anonymous visitor — short-circuit instead of round-tripping to the
  // backend just to be told there's no session. This one IS a definitive
  // answer: no cookie means no session, and no backend is needed to know it.
  if (!authToken?.value) {
    return { kind: 'resolved', profile: UNAUTHENTICATED_PROFILE }
  }

  try {
    const response = await fetch(`${API_BASE_URL}/auth/profile`, {
      headers: { Cookie: `auth_token=${authToken.value}` },
      cache: 'no-store',
    })

    if (!response.ok) {
      // Shared with AuthContext and useProfile's retry policy rather than
      // re-decided here: a 401/403 (or a token error code) is the backend
      // ANSWERING that this cookie identifies no session, which settles.
      const errorCode = await readErrorCode(response)
      if (isDefinitiveUnauthenticated(response.status, errorCode)) {
        return { kind: 'resolved', profile: UNAUTHENTICATED_PROFILE }
      }

      // Anything else (5xx, and any other unexpected status) is the backend
      // failing to answer, NOT answering "nobody". Report it as indeterminate
      // rather than fabricating a logged-out viewer out of an outage.
      if (response.status >= 500) {
        Sentry.captureMessage(`SSR auth profile fetch failed: ${response.status}`, {
          level: 'error',
          tags: { service: 'auth', error_type: 'ssr_prefetch_failure' },
          extra: { status: response.status },
        })
      }
      return { kind: 'indeterminate' }
    }

    // A 2xx is not by itself an answer, so the body is checked rather than
    // cast. An edge interstitial, an API version change, or a handler that
    // starts returning `{}` all parse as JSON and would otherwise seed a
    // payload whose `success` is undefined: the context reads no user, the
    // query is no longer pending, and a signed-in viewer settles to
    // 'anonymous'. That is the same fabricated answer the indeterminate branch
    // above exists to prevent, arriving through the one branch that was not
    // checking. `success` is the field every consumer keys off, so its
    // presence is the minimum that makes this a profile.
    const body: unknown = await response.json()
    if (typeof (body as AuthProfilePayload)?.success !== 'boolean') {
      Sentry.captureMessage('SSR auth profile returned an unrecognized body', {
        level: 'error',
        tags: { service: 'auth', error_type: 'ssr_prefetch_bad_body' },
        extra: { status: response.status },
      })
      return { kind: 'indeterminate' }
    }

    return { kind: 'resolved', profile: body as AuthProfilePayload }
  } catch (error) {
    // Network failure (backend unreachable from the Next server, DNS, etc.).
    // Also not an answer.
    Sentry.captureException(error, {
      level: 'error',
      tags: { service: 'auth', error_type: 'ssr_prefetch_network_failure' },
    })
    return { kind: 'indeterminate' }
  }
})
