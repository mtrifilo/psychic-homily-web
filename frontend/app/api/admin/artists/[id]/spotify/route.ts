import { NextRequest, NextResponse } from 'next/server'
import { resolveSpotifyArtist } from '@/lib/spotify'
import { requireAdmin, forwardArtistMusicUpdate } from '@/lib/admin-artist-route'

interface UpdateSpotifyRequest {
  spotify_url: string
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

  const auth = await requireAdmin()
  if (!auth.ok) return auth.response

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
    return NextResponse.json({ error: validation.error }, { status: 400 })
  }

  return forwardArtistMusicUpdate({
    artistId,
    authToken: auth.authToken,
    field: 'spotify',
    body: { spotify_url: validation.canonicalUrl },
    sentryService: 'admin-spotify',
    sentryOperation: 'update',
    failureMessage: 'Failed to update artist',
  })
}

// Also support DELETE to clear the Spotify URL
export async function DELETE(
  request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const { id: artistId } = await params

  const auth = await requireAdmin()
  if (!auth.ok) return auth.response

  return forwardArtistMusicUpdate({
    artistId,
    authToken: auth.authToken,
    field: 'spotify',
    body: { spotify_url: null },
    sentryService: 'admin-spotify',
    sentryOperation: 'clear',
    failureMessage: 'Failed to clear Spotify URL',
  })
}
