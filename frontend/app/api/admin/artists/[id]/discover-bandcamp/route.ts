import { NextRequest, NextResponse } from 'next/server'
import { cookies } from 'next/headers'
import Anthropic from '@anthropic-ai/sdk'
import * as Sentry from '@sentry/nextjs'

const BACKEND_URL = process.env.BACKEND_URL || 'http://localhost:8080'

const SYSTEM_PROMPT = `You are a music research assistant helping to find Bandcamp album pages for artists.

Your task is to find the official Bandcamp album page for the given artist name.

Rules:
1. Search for the artist's official Bandcamp page
2. Return an album or track URL in the format: https://[artist].bandcamp.com/album/[name] or https://[artist].bandcamp.com/track/[name]
3. Do NOT return just the profile URL (e.g., https://artist.bandcamp.com)
4. Prefer full albums over single tracks when available
5. Prefer the most recent album, or the artist's most popular/representative work
6. If the artist has multiple Bandcamp pages, prefer the official/verified one
7. If you cannot find a Bandcamp page for this artist, return exactly: NOT_FOUND

Important:
- Only return Bandcamp URLs, not Spotify, SoundCloud, or other platforms
- The URL must be embeddable (album or track page, not profile page)
- Return ONLY the URL on a single line, or NOT_FOUND - no other text`

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
  } catch (error) {
    Sentry.captureException(error, {
      level: 'error',
      tags: { service: 'discover-bandcamp', error_type: 'auth_fetch' },
    })
    return null
  }
}

async function getArtist(artistId: string): Promise<Artist | null> {
  try {
    const response = await fetch(`${BACKEND_URL}/artists/${artistId}`)

    if (!response.ok) {
      return null
    }

    return await response.json()
  } catch (error) {
    Sentry.captureException(error, {
      level: 'error',
      tags: { service: 'discover-bandcamp', error_type: 'artist_fetch' },
      extra: { artistId },
    })
    return null
  }
}

async function updateArtistBandcamp(
  artistId: string,
  bandcampUrl: string,
  authToken: string
): Promise<Artist | null> {
  try {
    const response = await fetch(
      `${BACKEND_URL}/admin/artists/${artistId}/bandcamp`,
      {
        method: 'PATCH',
        headers: {
          'Content-Type': 'application/json',
          Cookie: `auth_token=${authToken}`,
        },
        body: JSON.stringify({ bandcamp_embed_url: bandcampUrl }),
      }
    )

    if (!response.ok) {
      console.error('Failed to update artist:', await response.text())
      return null
    }

    return await response.json()
  } catch (error) {
    Sentry.captureException(error, {
      level: 'error',
      tags: { service: 'discover-bandcamp', error_type: 'artist_update' },
      extra: { artistId, bandcampUrl },
    })
    console.error('Error updating artist:', error)
    return null
  }
}

async function validateBandcampUrl(url: string): Promise<boolean> {
  try {
    // Use our existing album-id endpoint to validate the URL
    const response = await fetch(
      `${process.env.NEXT_PUBLIC_URL || 'http://localhost:3000'}/api/bandcamp/album-id?url=${encodeURIComponent(url)}`
    )

    if (!response.ok) {
      return false
    }

    const data = await response.json()
    return !!data.albumId
  } catch (error) {
    Sentry.captureException(error, {
      level: 'error',
      tags: { service: 'discover-bandcamp', error_type: 'validation' },
      extra: { url },
    })
    return false
  }
}

function extractBandcampUrl(text: string): string | null {
  // Try to find a Bandcamp URL in the response
  const urlMatch = text.match(
    /https?:\/\/[a-zA-Z0-9-]+\.bandcamp\.com\/(album|track)\/[a-zA-Z0-9-]+/
  )
  return urlMatch ? urlMatch[0] : null
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

  // Get artist details
  const artist = await getArtist(artistId)
  if (!artist) {
    return NextResponse.json({ error: 'Artist not found' }, { status: 404 })
  }

  // Check if Anthropic API key is configured
  const apiKey = process.env.ANTHROPIC_API_KEY
  if (!apiKey) {
    Sentry.captureMessage('ANTHROPIC_API_KEY not configured', {
      level: 'error',
      tags: { service: 'discover-bandcamp', error_type: 'missing_config' },
    })
    console.error('ANTHROPIC_API_KEY not configured')
    return NextResponse.json(
      { error: 'AI service not configured' },
      { status: 503 }
    )
  }

  try {
    // Initialize Anthropic client
    const anthropic = new Anthropic({ apiKey })

    // Call Claude with web search to find Bandcamp album
    const response = await anthropic.messages.create({
      model: 'claude-haiku-4-5-20251001',
      max_tokens: 512,
      tools: [
        {
          type: 'web_search_20250305',
          name: 'web_search',
          max_uses: 3,
        },
      ],
      system: SYSTEM_PROMPT,
      messages: [
        {
          role: 'user',
          content: `Find the official Bandcamp album page for: ${artist.name}`,
        },
      ],
    })

    // Extract text response
    let responseText = ''
    for (const block of response.content) {
      if (block.type === 'text') {
        responseText += block.text
      }
    }

    // Check for NOT_FOUND
    if (responseText.trim() === 'NOT_FOUND' || responseText.includes('NOT_FOUND')) {
      return NextResponse.json(
        {
          success: false,
          error: 'NOT_FOUND',
          message: `Could not find a Bandcamp album for "${artist.name}"`,
        },
        { status: 404 }
      )
    }

    // Extract URL from response
    const bandcampUrl = extractBandcampUrl(responseText)

    if (!bandcampUrl) {
      console.error('AI did not return a valid Bandcamp URL:', responseText)
      return NextResponse.json(
        {
          success: false,
          error: 'INVALID_RESPONSE',
          message: 'AI did not return a valid Bandcamp URL. Try manual entry.',
        },
        { status: 422 }
      )
    }

    // Validate the URL is actually embeddable
    const isValid = await validateBandcampUrl(bandcampUrl)
    if (!isValid) {
      console.error('Bandcamp URL validation failed:', bandcampUrl)
      return NextResponse.json(
        {
          success: false,
          error: 'INVALID_URL',
          message:
            'The discovered URL could not be validated. Try manual entry.',
          discovered_url: bandcampUrl,
        },
        { status: 422 }
      )
    }

    // Update artist with the discovered URL
    const updatedArtist = await updateArtistBandcamp(
      artistId,
      bandcampUrl,
      authToken.value
    )

    if (!updatedArtist) {
      return NextResponse.json(
        {
          success: false,
          error: 'UPDATE_FAILED',
          message: 'Failed to save the discovered URL',
          discovered_url: bandcampUrl,
        },
        { status: 500 }
      )
    }

    return NextResponse.json({
      success: true,
      bandcamp_url: bandcampUrl,
      artist: updatedArtist,
    })
  } catch (error) {
    Sentry.captureException(error, {
      level: 'error',
      tags: { service: 'discover-bandcamp' },
      extra: { artistId, artistName: artist.name },
    })
    console.error('Bandcamp discovery error:', error)

    if (error instanceof Anthropic.APIError) {
      return NextResponse.json(
        {
          error: 'AI service error',
          message: error.message,
        },
        { status: 503 }
      )
    }

    return NextResponse.json(
      {
        error: 'Discovery failed',
        message: error instanceof Error ? error.message : 'Unknown error',
      },
      { status: 500 }
    )
  }
}
