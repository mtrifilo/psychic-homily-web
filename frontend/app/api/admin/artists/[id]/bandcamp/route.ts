import { NextRequest, NextResponse } from 'next/server'
import { resolveBandcampEmbed, isAllowedBandcampUrl } from '@/lib/bandcamp'
import { requireAdmin, forwardArtistMusicUpdate } from '@/lib/admin-artist-route'

interface UpdateBandcampRequest {
  bandcamp_url: string
}

// Validates the URL is an embeddable Bandcamp album/track page and returns the
// URL that actually resolved — which may differ from the input when an
// /album/ <-> /track/ path mismatch was auto-corrected (see lib/bandcamp). The
// caller persists `resolvedUrl` so a corrected path is what gets stored.
async function validateBandcampUrl(
  url: string
): Promise<{ valid: true; resolvedUrl: string } | { valid: false; error: string }> {
  // Host must be a real bandcamp.com (sub)domain — the URL is fetched
  // server-side below, so a substring check would allow SSRF. See lib/bandcamp.
  if (!isAllowedBandcampUrl(url)) {
    return { valid: false, error: 'URL must be a Bandcamp URL' }
  }

  if (!url.includes('/album/') && !url.includes('/track/')) {
    return {
      valid: false,
      error: 'URL must be a Bandcamp album or track URL, not a profile URL',
    }
  }

  const result = await resolveBandcampEmbed(url)
  if (!result.ok) {
    return { valid: false, error: result.error }
  }
  return { valid: true, resolvedUrl: result.embed.resolvedUrl }
}

export async function POST(
  request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const { id: artistId } = await params

  const auth = await requireAdmin()
  if (!auth.ok) return auth.response

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

  return forwardArtistMusicUpdate({
    artistId,
    authToken: auth.authToken,
    field: 'bandcamp',
    // Persist the resolved URL (path auto-corrected if it was wrong).
    body: { bandcamp_embed_url: validation.resolvedUrl },
    sentryService: 'admin-bandcamp',
    sentryOperation: 'update',
    failureMessage: 'Failed to update artist',
  })
}

// Also support DELETE to clear the Bandcamp URL
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
    field: 'bandcamp',
    body: { bandcamp_embed_url: null },
    sentryService: 'admin-bandcamp',
    sentryOperation: 'clear',
    failureMessage: 'Failed to clear Bandcamp URL',
  })
}
