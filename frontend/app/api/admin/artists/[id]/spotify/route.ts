import { NextRequest, NextResponse } from 'next/server'
import { cookies } from 'next/headers'
import * as Sentry from '@sentry/nextjs'
import { revalidateArtistDetail } from '@/lib/revalidate-entity'
import { resolveSpotifyArtist } from '@/lib/spotify'

const BACKEND_URL = process.env.BACKEND_URL || 'http://localhost:8080'

interface UserProfile {
  success: boolean
  user?: {
    id: string
    is_admin?: boolean
  }
}

interface Artist {
  id: number
  slug: string
  name: string
  social?: {
    spotify?: string | null
  }
}

interface UpdateSpotifyRequest {
  spotify_url: string
}

async function getAuthenticatedUser(
  authToken: string
): Promise<UserProfile | null> {
  try {
    const response = await fetch(`${BACKEND_URL}/auth/profile`, {
      headers: {
        Cookie: `auth_token=${authToken}`,
      },
    })

    if (!response.ok) {
      return null
    }

    return await response.json()
  } catch {
    return null
  }
}

// Validates shape AND existence (via Spotify oEmbed — mirrors the Bandcamp save
// path), returning the canonical artist URL so a `?si=` share suffix is stripped
// before persisting. See lib/spotify.
async function validateSpotifyUrl(
  url: string
): Promise<{ valid: true; canonicalUrl: string } | { valid: false; error: string }> {
  const result = await resolveSpotifyArtist(url)
  if (!result.ok) {
    return { valid: false, error: result.error }
  }
  return { valid: true, canonicalUrl: result.canonicalUrl }
}

export async function POST(
  request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const { id: artistId } = await params

  // Get auth token from cookies
  const cookieStore = await cookies()
  const authToken = cookieStore.get('auth_token')

  if (!authToken) {
    return NextResponse.json(
      { error: 'Authentication required' },
      { status: 401 }
    )
  }

  // Validate admin access
  const profile = await getAuthenticatedUser(authToken.value)
  if (!profile?.success || !profile.user?.is_admin) {
    return NextResponse.json({ error: 'Admin access required' }, { status: 403 })
  }

  // Parse request body
  let body: UpdateSpotifyRequest
  try {
    body = await request.json()
  } catch {
    return NextResponse.json({ error: 'Invalid JSON body' }, { status: 400 })
  }

  const { spotify_url } = body

  if (!spotify_url) {
    return NextResponse.json(
      { error: 'spotify_url is required' },
      { status: 400 }
    )
  }

  // Validate the URL (shape + existence); persist the canonical artist URL.
  const validation = await validateSpotifyUrl(spotify_url)
  if (!validation.valid) {
    return NextResponse.json(
      { error: validation.error },
      { status: 400 }
    )
  }

  // Forward to backend
  try {
    const response = await fetch(
      `${BACKEND_URL}/admin/artists/${artistId}/spotify`,
      {
        method: 'PATCH',
        headers: {
          'Content-Type': 'application/json',
          Cookie: `auth_token=${authToken.value}`,
        },
        body: JSON.stringify({ spotify_url: validation.canonicalUrl }),
      }
    )

    if (!response.ok) {
      const error = await response.json().catch(() => ({ detail: 'Unknown error' }))
      return NextResponse.json(
        { error: error.detail || 'Failed to update artist' },
        { status: response.status }
      )
    }

    const artist: Artist = await response.json()

    revalidateArtistDetail(artist.slug)

    return NextResponse.json({
      success: true,
      artist,
    })
  } catch (error) {
    Sentry.captureException(error, {
      level: 'error',
      tags: { service: 'admin-spotify', operation: 'update' },
      extra: { artistId },
    })
    return NextResponse.json(
      { error: 'Failed to update artist' },
      { status: 500 }
    )
  }
}

// Also support DELETE to clear the Spotify URL
export async function DELETE(
  request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const { id: artistId } = await params

  // Get auth token from cookies
  const cookieStore = await cookies()
  const authToken = cookieStore.get('auth_token')

  if (!authToken) {
    return NextResponse.json(
      { error: 'Authentication required' },
      { status: 401 }
    )
  }

  // Validate admin access
  const profile = await getAuthenticatedUser(authToken.value)
  if (!profile?.success || !profile.user?.is_admin) {
    return NextResponse.json({ error: 'Admin access required' }, { status: 403 })
  }

  // Forward to backend with null to clear
  try {
    const response = await fetch(
      `${BACKEND_URL}/admin/artists/${artistId}/spotify`,
      {
        method: 'PATCH',
        headers: {
          'Content-Type': 'application/json',
          Cookie: `auth_token=${authToken.value}`,
        },
        body: JSON.stringify({ spotify_url: null }),
      }
    )

    if (!response.ok) {
      const error = await response.json().catch(() => ({ detail: 'Unknown error' }))
      return NextResponse.json(
        { error: error.detail || 'Failed to clear Spotify URL' },
        { status: response.status }
      )
    }

    const artist: Artist = await response.json()

    revalidateArtistDetail(artist.slug)

    return NextResponse.json({
      success: true,
      artist,
    })
  } catch (error) {
    Sentry.captureException(error, {
      level: 'error',
      tags: { service: 'admin-spotify', operation: 'clear' },
      extra: { artistId },
    })
    return NextResponse.json(
      { error: 'Failed to clear Spotify URL' },
      { status: 500 }
    )
  }
}
