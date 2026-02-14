import { NextRequest, NextResponse } from 'next/server'
import { cookies } from 'next/headers'
import * as Sentry from '@sentry/nextjs'

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
  bandcamp_embed_url: string | null
}

interface UpdateBandcampRequest {
  bandcamp_url: string
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

async function validateBandcampUrl(url: string): Promise<{ valid: boolean; error?: string }> {
  // Basic format validation
  if (!url.includes('bandcamp.com')) {
    return { valid: false, error: 'URL must be a Bandcamp URL' }
  }

  if (!url.includes('/album/') && !url.includes('/track/')) {
    return {
      valid: false,
      error: 'URL must be a Bandcamp album or track URL, not a profile URL',
    }
  }

  try {
    // Fetch the Bandcamp page directly to validate it's embeddable
    const response = await fetch(url, {
      headers: {
        'User-Agent': 'Mozilla/5.0 (compatible; MusicEmbed/1.0)',
      },
    })

    if (!response.ok) {
      return { valid: false, error: `Failed to fetch Bandcamp page: ${response.status}` }
    }

    const html = await response.text()
    const match = html.match(/album=(\d+)/)
    if (!match) {
      return { valid: false, error: 'Could not extract album ID from URL' }
    }

    return { valid: true }
  } catch {
    return { valid: false, error: 'Failed to validate URL' }
  }
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
  let body: UpdateBandcampRequest
  try {
    body = await request.json()
  } catch {
    return NextResponse.json({ error: 'Invalid JSON body' }, { status: 400 })
  }

  const { bandcamp_url } = body

  if (!bandcamp_url) {
    return NextResponse.json(
      { error: 'bandcamp_url is required' },
      { status: 400 }
    )
  }

  // Validate the URL
  const validation = await validateBandcampUrl(bandcamp_url)
  if (!validation.valid) {
    return NextResponse.json(
      { error: validation.error || 'Invalid Bandcamp URL' },
      { status: 400 }
    )
  }

  // Forward to backend
  try {
    const response = await fetch(
      `${BACKEND_URL}/admin/artists/${artistId}/bandcamp`,
      {
        method: 'PATCH',
        headers: {
          'Content-Type': 'application/json',
          Cookie: `auth_token=${authToken.value}`,
        },
        body: JSON.stringify({ bandcamp_embed_url: bandcamp_url }),
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
    Sentry.captureException(error, {
      level: 'error',
      tags: { service: 'admin-bandcamp', operation: 'update' },
      extra: { artistId },
    })
    return NextResponse.json(
      { error: 'Failed to update artist' },
      { status: 500 }
    )
  }
}

// Also support DELETE to clear the Bandcamp URL
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
      `${BACKEND_URL}/admin/artists/${artistId}/bandcamp`,
      {
        method: 'PATCH',
        headers: {
          'Content-Type': 'application/json',
          Cookie: `auth_token=${authToken.value}`,
        },
        body: JSON.stringify({ bandcamp_embed_url: null }),
      }
    )

    if (!response.ok) {
      const error = await response.json().catch(() => ({ detail: 'Unknown error' }))
      return NextResponse.json(
        { error: error.detail || 'Failed to clear Bandcamp URL' },
        { status: response.status }
      )
    }

    const artist: Artist = await response.json()

    return NextResponse.json({
      success: true,
      artist,
    })
  } catch (error) {
    Sentry.captureException(error, {
      level: 'error',
      tags: { service: 'admin-bandcamp', operation: 'clear' },
      extra: { artistId },
    })
    return NextResponse.json(
      { error: 'Failed to clear Bandcamp URL' },
      { status: 500 }
    )
  }
}
