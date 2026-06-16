// Shared helpers for the admin artist BFF routes (bandcamp / spotify /
// discover-music), which had the admin auth gate and the backend-forward
// copy-pasted across every handler (PSY-1111). The lower-level /auth/profile
// fetch lives in lib/auth-profile (also used by the AI extract routes).

import { cookies } from 'next/headers'
import { NextResponse } from 'next/server'
import * as Sentry from '@sentry/nextjs'
import { revalidateArtistDetail } from '@/lib/revalidate-entity'
import { getAuthenticatedUser } from '@/lib/auth-profile'

const BACKEND_URL = process.env.BACKEND_URL || 'http://localhost:8080'

// Gate an admin-only route: requires the auth_token cookie and an is_admin
// profile, returning the session token on success. On failure returns the exact
// 401/403 envelopes the routes returned inline, so the caller does:
//   const auth = await requireAdmin()
//   if (!auth.ok) return auth.response
// (discover-music intentionally does NOT use this — it inlines the same gate so
// it can run the profile fetch concurrently with its artist lookup.)
export async function requireAdmin(): Promise<
  { ok: true; authToken: string } | { ok: false; response: NextResponse }
> {
  const cookieStore = await cookies()
  const authToken = cookieStore.get('auth_token')
  if (!authToken) {
    return {
      ok: false,
      response: NextResponse.json(
        { error: 'Authentication required' },
        { status: 401 }
      ),
    }
  }
  const profile = await getAuthenticatedUser(authToken.value)
  if (!profile?.success || !profile.user?.is_admin) {
    return {
      ok: false,
      response: NextResponse.json(
        { error: 'Admin access required' },
        { status: 403 }
      ),
    }
  }
  return { ok: true, authToken: authToken.value }
}

// Forward an artist Bandcamp/Spotify mutation to the backend, revalidate the
// artist ISR page on success, and map failures to the same responses the routes
// returned inline. `failureMessage` is the human fallback for both the upstream
// non-2xx (after the backend's `detail`) and the 500 catch.
export async function forwardArtistMusicUpdate(opts: {
  artistId: string
  authToken: string
  field: 'bandcamp' | 'spotify'
  body: Record<string, unknown>
  sentryService: string
  sentryOperation: string
  failureMessage: string
}): Promise<NextResponse> {
  try {
    const response = await fetch(
      `${BACKEND_URL}/admin/artists/${opts.artistId}/${opts.field}`,
      {
        method: 'PATCH',
        headers: {
          'Content-Type': 'application/json',
          Cookie: `auth_token=${opts.authToken}`,
        },
        body: JSON.stringify(opts.body),
      }
    )

    if (!response.ok) {
      const error = await response
        .json()
        .catch(() => ({ detail: 'Unknown error' }))
      return NextResponse.json(
        { error: error.detail || opts.failureMessage },
        { status: response.status }
      )
    }

    const artist = (await response.json()) as { slug: string }
    revalidateArtistDetail(artist.slug)
    return NextResponse.json({ success: true, artist })
  } catch (error) {
    Sentry.captureException(error, {
      level: 'error',
      tags: { service: opts.sentryService, operation: opts.sentryOperation },
      extra: { artistId: opts.artistId },
    })
    return NextResponse.json({ error: opts.failureMessage }, { status: 500 })
  }
}
