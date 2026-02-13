import { NextRequest, NextResponse } from 'next/server'
import { cookies } from 'next/headers'
import Anthropic from '@anthropic-ai/sdk'
import * as Sentry from '@sentry/nextjs'

const BACKEND_URL = process.env.BACKEND_URL || 'http://localhost:8080'

const BANDCAMP_SYSTEM_PROMPT = `You are a music research assistant helping to find Bandcamp album pages for artists.

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

const SPOTIFY_SYSTEM_PROMPT = `You are a music research assistant helping to find Spotify artist pages.

Your task is to find the official Spotify artist page for the given artist name.

Rules:
1. Search for the artist's official Spotify page
2. Return the artist page URL in the format: https://open.spotify.com/artist/[artist_id]
3. Make sure it's the correct artist - verify by checking their discography matches
4. If the artist has multiple Spotify pages (common for artists with similar names), prefer the one with more followers/streams
5. If you cannot find a Spotify page for this artist, return exactly: NOT_FOUND

Important:
- Only return Spotify artist page URLs
- The URL must be in the format: https://open.spotify.com/artist/[22-character-id]
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
  social?: {
    spotify?: string | null
  }
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

async function getArtist(artistId: string): Promise<Artist | null> {
  try {
    const response = await fetch(`${BACKEND_URL}/artists/${artistId}`)

    if (!response.ok) {
      return null
    }

    return await response.json()
  } catch {
    return null
  }
}

async function updateArtistBandcamp(
  artistId: string,
  bandcampUrl: string,
  authToken?: string
): Promise<Artist | null> {
  try {
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
    }

    // Use auth token if available, otherwise use internal secret
    if (authToken) {
      headers['Cookie'] = `auth_token=${authToken}`
    } else {
      const internalSecret = process.env.INTERNAL_API_SECRET
      if (internalSecret) {
        headers['X-Internal-Secret'] = internalSecret
      }
    }

    const response = await fetch(
      `${BACKEND_URL}/admin/artists/${artistId}/bandcamp`,
      {
        method: 'PATCH',
        headers,
        body: JSON.stringify({ bandcamp_embed_url: bandcampUrl }),
      }
    )

    if (!response.ok) {
      Sentry.captureMessage('Failed to update artist bandcamp', {
        level: 'error',
        tags: { service: 'music-discovery', error_type: 'bandcamp_update' },
        extra: { artistId, status: response.status },
      })
      return null
    }

    return await response.json()
  } catch (error) {
    Sentry.captureException(error, {
      level: 'error',
      tags: { service: 'music-discovery', error_type: 'bandcamp_update' },
      extra: { artistId },
    })
    return null
  }
}

async function updateArtistSpotify(
  artistId: string,
  spotifyUrl: string,
  authToken?: string
): Promise<Artist | null> {
  try {
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
    }

    // Use auth token if available, otherwise use internal secret
    if (authToken) {
      headers['Cookie'] = `auth_token=${authToken}`
    } else {
      const internalSecret = process.env.INTERNAL_API_SECRET
      if (internalSecret) {
        headers['X-Internal-Secret'] = internalSecret
      }
    }

    const response = await fetch(
      `${BACKEND_URL}/admin/artists/${artistId}/spotify`,
      {
        method: 'PATCH',
        headers,
        body: JSON.stringify({ spotify_url: spotifyUrl }),
      }
    )

    if (!response.ok) {
      Sentry.captureMessage('Failed to update artist spotify', {
        level: 'error',
        tags: { service: 'music-discovery', error_type: 'spotify_update' },
        extra: { artistId, status: response.status },
      })
      return null
    }

    return await response.json()
  } catch (error) {
    Sentry.captureException(error, {
      level: 'error',
      tags: { service: 'music-discovery', error_type: 'spotify_update' },
      extra: { artistId },
    })
    return null
  }
}

async function validateBandcampUrl(url: string): Promise<boolean> {
  try {
    // Fetch the Bandcamp page directly and check for album ID pattern
    // (avoids self-referencing HTTP fetch to /api/bandcamp/album-id)
    const response = await fetch(url, {
      headers: {
        'User-Agent': 'Mozilla/5.0 (compatible; MusicEmbed/1.0)',
      },
    })

    if (!response.ok) {
      return false
    }

    const html = await response.text()
    return /album=(\d+)/.test(html)
  } catch {
    return false
  }
}

function extractBandcampUrl(text: string): string | null {
  // Try to find a Bandcamp album/track URL in the response
  const urlMatch = text.match(
    /https?:\/\/[a-zA-Z0-9-]+\.bandcamp\.com\/(album|track)\/[a-zA-Z0-9-]+/
  )
  return urlMatch ? urlMatch[0] : null
}

function extractBandcampProfileUrl(text: string): string | null {
  // Match a Bandcamp profile URL (not album/track) like https://artist.bandcamp.com
  const urlMatch = text.match(
    /https?:\/\/[a-zA-Z0-9-]+\.bandcamp\.com\/?(?!\/(album|track)\/)/
  )
  if (!urlMatch) return null
  // Clean trailing slash
  return urlMatch[0].replace(/\/$/, '')
}

async function resolveAlbumFromProfile(profileUrl: string): Promise<string | null> {
  try {
    const response = await fetch(profileUrl, {
      headers: {
        'User-Agent': 'Mozilla/5.0 (compatible; MusicEmbed/1.0)',
      },
    })

    if (!response.ok) return null

    const html = await response.text()

    // Bandcamp profile pages contain relative album/track links like href="/album/new-earth"
    // Find all album links first (preferred), then fall back to track links
    const albumMatches = html.match(/href="\/album\/[a-zA-Z0-9-]+"/g)
    if (albumMatches && albumMatches.length > 0) {
      const path = albumMatches[0].match(/href="(\/album\/[a-zA-Z0-9-]+)"/)
      if (path) return `${profileUrl}${path[1]}`
    }

    const trackMatches = html.match(/href="\/track\/[a-zA-Z0-9-]+"/g)
    if (trackMatches && trackMatches.length > 0) {
      const path = trackMatches[0].match(/href="(\/track\/[a-zA-Z0-9-]+)"/)
      if (path) return `${profileUrl}${path[1]}`
    }

    return null
  } catch {
    return null
  }
}

function extractSpotifyUrl(text: string): string | null {
  // Try to find a Spotify artist URL in the response
  const urlMatch = text.match(
    /https?:\/\/open\.spotify\.com\/artist\/[a-zA-Z0-9]+/
  )
  return urlMatch ? urlMatch[0] : null
}

function isValidSpotifyArtistUrl(url: string): boolean {
  // Validate Spotify artist URL format
  return /^https?:\/\/open\.spotify\.com\/artist\/[a-zA-Z0-9]+$/.test(url)
}

type Platform = 'bandcamp' | 'spotify'

interface DiscoveryResult {
  found: boolean
  platform?: Platform
  url?: string
  error?: string
}

function isCreditsError(error: unknown): boolean {
  if (error instanceof Anthropic.APIError) {
    const message = error.message.toLowerCase()
    return (
      message.includes('credit') ||
      message.includes('billing') ||
      message.includes('balance')
    )
  }
  return false
}

async function discoverBandcamp(
  artistName: string,
  anthropic: Anthropic
): Promise<DiscoveryResult> {
  try {
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
      system: BANDCAMP_SYSTEM_PROMPT,
      messages: [
        {
          role: 'user',
          // Include "bandcamp" in query to improve search results
          content: `Find the official Bandcamp album page for: ${artistName} bandcamp`,
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
    if (
      responseText.trim() === 'NOT_FOUND' ||
      responseText.includes('NOT_FOUND')
    ) {
      return { found: false }
    }

    // Extract album/track URL from response
    let bandcampUrl = extractBandcampUrl(responseText)

    // If no album/track URL found, check for a profile URL and try to resolve an album from it
    if (!bandcampUrl) {
      const profileUrl = extractBandcampProfileUrl(responseText)
      if (profileUrl) {
        bandcampUrl = await resolveAlbumFromProfile(profileUrl)
      }
    }

    if (!bandcampUrl) {
      return { found: false, error: 'No valid URL in response' }
    }

    // Validate the URL is actually embeddable
    const isValid = await validateBandcampUrl(bandcampUrl)
    if (!isValid) {
      return { found: false, error: 'URL validation failed' }
    }

    return { found: true, platform: 'bandcamp', url: bandcampUrl }
  } catch (error) {
    // Re-throw credit errors to surface them prominently
    if (isCreditsError(error)) {
      throw error
    }
    Sentry.captureException(error, {
      level: 'error',
      tags: { service: 'music-discovery', error_type: 'bandcamp_discovery' },
      extra: { artistName },
    })
    return {
      found: false,
      error: error instanceof Error ? error.message : 'Unknown error',
    }
  }
}

async function discoverSpotify(
  artistName: string,
  anthropic: Anthropic
): Promise<DiscoveryResult> {
  try {
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
      system: SPOTIFY_SYSTEM_PROMPT,
      messages: [
        {
          role: 'user',
          // Include "spotify" in query to improve search results
          content: `Find the official Spotify artist page for: ${artistName} spotify`,
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
    if (
      responseText.trim() === 'NOT_FOUND' ||
      responseText.includes('NOT_FOUND')
    ) {
      return { found: false }
    }

    // Extract URL from response
    const spotifyUrl = extractSpotifyUrl(responseText)

    if (!spotifyUrl) {
      return { found: false, error: 'No valid URL in response' }
    }

    // Validate URL format
    if (!isValidSpotifyArtistUrl(spotifyUrl)) {
      return { found: false, error: 'Invalid Spotify URL format' }
    }

    return { found: true, platform: 'spotify', url: spotifyUrl }
  } catch (error) {
    // Re-throw credit errors to surface them prominently
    if (isCreditsError(error)) {
      throw error
    }
    Sentry.captureException(error, {
      level: 'error',
      tags: { service: 'music-discovery', error_type: 'spotify_discovery' },
      extra: { artistName },
    })
    return {
      found: false,
      error: error instanceof Error ? error.message : 'Unknown error',
    }
  }
}

// Check if request is from internal backend service
function isInternalServiceRequest(request: NextRequest): boolean {
  const internalSecret = process.env.INTERNAL_API_SECRET
  if (!internalSecret) {
    return false
  }
  const requestSecret = request.headers.get('X-Internal-Secret')
  return requestSecret === internalSecret
}

export async function POST(
  request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const { id: artistId } = await params

  // Check for internal service request (backend auto-discovery)
  const isInternalRequest = isInternalServiceRequest(request)
  let authToken: string | undefined

  if (isInternalRequest) {
    // Internal request - no user auth needed, but we need a way to update the artist
    // We'll use a special internal auth flow
    authToken = undefined
  } else {
    // External request - require admin auth
    const cookieStore = await cookies()
    const authCookie = cookieStore.get('auth_token')

    if (!authCookie) {
      return NextResponse.json(
        { error: 'Authentication required' },
        { status: 401 }
      )
    }

    // Validate admin access
    const profile = await getAuthenticatedUser(authCookie.value)
    if (!profile?.success || !profile.user?.is_admin) {
      return NextResponse.json({ error: 'Admin access required' }, { status: 403 })
    }

    authToken = authCookie.value
  }

  // Get artist details
  const artist = await getArtist(artistId)
  if (!artist) {
    return NextResponse.json({ error: 'Artist not found' }, { status: 404 })
  }

  // Check if Anthropic API key is configured
  const apiKey = process.env.ANTHROPIC_API_KEY
  if (!apiKey) {
    const error = new Error('ANTHROPIC_API_KEY not configured')
    Sentry.captureException(error, {
      level: 'error',
      tags: { service: 'music-discovery' },
    })
    return NextResponse.json(
      { error: 'AI service not configured' },
      { status: 503 }
    )
  }

  try {
    // Initialize Anthropic client
    const anthropic = new Anthropic({ apiKey })

    // Run both discoveries in parallel
    const [bandcampSettled, spotifySettled] = await Promise.allSettled([
      discoverBandcamp(artist.name, anthropic),
      discoverSpotify(artist.name, anthropic),
    ])

    // Check for credit errors from either call
    for (const settled of [bandcampSettled, spotifySettled]) {
      if (settled.status === 'rejected' && isCreditsError(settled.reason)) {
        throw settled.reason
      }
    }

    const bandcampResult: DiscoveryResult =
      bandcampSettled.status === 'fulfilled'
        ? bandcampSettled.value
        : { found: false, error: bandcampSettled.reason?.message }
    const spotifyResult: DiscoveryResult =
      spotifySettled.status === 'fulfilled'
        ? spotifySettled.value
        : { found: false, error: spotifySettled.reason?.message }

    const foundBandcamp = bandcampResult.found && bandcampResult.url
    const foundSpotify = spotifyResult.found && spotifyResult.url

    // Save both results in parallel
    const updatePromises: Promise<Artist | null>[] = []
    if (foundBandcamp) {
      updatePromises.push(
        updateArtistBandcamp(artistId, bandcampResult.url!, authToken)
      )
    }
    if (foundSpotify) {
      updatePromises.push(
        updateArtistSpotify(artistId, spotifyResult.url!, authToken)
      )
    }

    if (updatePromises.length > 0) {
      const updateResults = await Promise.all(updatePromises)
      const anyUpdateFailed = updateResults.some(r => r === null)

      if (anyUpdateFailed && !updateResults.some(r => r !== null)) {
        // All updates failed
        Sentry.captureMessage('Failed to save all discovered music URLs', {
          level: 'error',
          tags: { service: 'music-discovery', error_type: 'update_failed' },
          extra: {
            artistId,
            bandcampUrl: bandcampResult.url,
            spotifyUrl: spotifyResult.url,
          },
        })
        return NextResponse.json(
          {
            success: false,
            error: 'UPDATE_FAILED',
            message: 'Failed to save the discovered music URLs',
            platform: foundBandcamp ? 'bandcamp' : 'spotify',
            discovered_url: foundBandcamp
              ? bandcampResult.url
              : spotifyResult.url,
          },
          { status: 500 }
        )
      }

      // Build platforms response
      const platforms = {
        bandcamp: {
          found: !!foundBandcamp,
          ...(foundBandcamp && { url: bandcampResult.url }),
          ...(!foundBandcamp &&
            bandcampResult.error && { error: bandcampResult.error }),
        },
        spotify: {
          found: !!foundSpotify,
          ...(foundSpotify && { url: spotifyResult.url }),
          ...(!foundSpotify &&
            spotifyResult.error && { error: spotifyResult.error }),
        },
      }

      // Top-level platform/url for backward compatibility (prefer Bandcamp)
      const primaryPlatform = foundBandcamp ? 'bandcamp' : 'spotify'
      const primaryUrl = foundBandcamp
        ? bandcampResult.url
        : spotifyResult.url

      return NextResponse.json({
        success: true,
        platform: primaryPlatform,
        url: primaryUrl,
        platforms,
        artist: updateResults.find(r => r !== null),
      })
    }

    // Neither platform found
    return NextResponse.json(
      {
        success: false,
        error: 'NOT_FOUND',
        message: `Could not find music for "${artist.name}" on Bandcamp or Spotify`,
        platforms: {
          bandcamp: {
            found: false,
            ...(bandcampResult.error && { error: bandcampResult.error }),
          },
          spotify: {
            found: false,
            ...(spotifyResult.error && { error: spotifyResult.error }),
          },
        },
      },
      { status: 404 }
    )
  } catch (error) {
    if (error instanceof Anthropic.APIError) {
      // Check for credit/billing errors and log prominently
      if (isCreditsError(error)) {
        Sentry.captureException(error, {
          level: 'error',
          tags: { service: 'music-discovery', error_type: 'credits_exhausted' },
        })
        return NextResponse.json(
          {
            error: 'API_CREDITS_EXHAUSTED',
            message:
              'Anthropic API credits exhausted. Please add credits to use AI discovery.',
          },
          { status: 503 }
        )
      }

      Sentry.captureException(error, {
        level: 'error',
        tags: { service: 'music-discovery', error_type: 'api_error' },
      })
      return NextResponse.json(
        {
          error: 'AI service error',
          message: error.message,
        },
        { status: 503 }
      )
    }

    Sentry.captureException(error, {
      level: 'error',
      tags: { service: 'music-discovery' },
    })
    return NextResponse.json(
      {
        error: 'Discovery failed',
        message: error instanceof Error ? error.message : 'Unknown error',
      },
      { status: 500 }
    )
  }
}
