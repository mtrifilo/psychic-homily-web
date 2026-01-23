import { NextRequest, NextResponse } from 'next/server'
import { cookies } from 'next/headers'

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

function validateSpotifyUrl(url: string): { valid: boolean; error?: string } {
  // Must be a Spotify URL
  if (!url.includes('open.spotify.com')) {
    return { valid: false, error: 'URL must be a Spotify URL' }
  }

  // Must be an artist page URL
  if (!url.includes('/artist/')) {
    return {
      valid: false,
      error: 'URL must be a Spotify artist page URL (open.spotify.com/artist/...)',
    }
  }

  // Validate URL format: open.spotify.com/artist/{id}
  const artistMatch = url.match(/open\.spotify\.com\/artist\/([a-zA-Z0-9]+)/)
  if (!artistMatch) {
    return { valid: false, error: 'Invalid Spotify artist URL format' }
  }

  return { valid: true }
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

  // Validate the URL
  const validation = validateSpotifyUrl(spotify_url)
  if (!validation.valid) {
    return NextResponse.json(
      { error: validation.error || 'Invalid Spotify URL' },
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
        body: JSON.stringify({ spotify_url }),
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

    return NextResponse.json({
      success: true,
      artist,
    })
  } catch (error) {
    console.error('Error updating artist Spotify URL:', error)
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

    return NextResponse.json({
      success: true,
      artist,
    })
  } catch (error) {
    console.error('Error clearing artist Spotify URL:', error)
    return NextResponse.json(
      { error: 'Failed to clear Spotify URL' },
      { status: 500 }
    )
  }
}
