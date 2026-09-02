import { NextRequest, NextResponse } from 'next/server'
import {
  resolveBandcampEmbed,
  isAllowedBandcampUrl,
  isBandcampReleaseUrl,
} from '@/lib/bandcamp'
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
  //
  // Subsumed by the release check below, which starts with this same host
  // anchor. Kept because the two failures need different words: "that is not
  // Bandcamp" and "that is Bandcamp, but not a release" send an admin to
  // different fixes.
  if (!isAllowedBandcampUrl(url)) {
    return { valid: false, error: 'URL must be a Bandcamp URL' }
  }

  // Release-page shape, read off the parsed PATHNAME, so a Bandcamp page that
  // merely carries "/album/" somewhere it does not mean it (/merch/shirt?ref=
  // /album/x) is not taken for a release. Same predicate the render gate uses;
  // the backend write gate stays the authority on what is stored, and is
  // stricter still (it also excludes the bandcamp.com apex).
  if (!isBandcampReleaseUrl(url)) {
    return {
      valid: false,
      error: 'URL must be a Bandcamp album or track URL, not a profile URL',
    }
  }

  // The backend stores only artist-subdomain release pages, so the apex is
  // refused HERE rather than after a wasted page fetch and a backend 422 whose
  // message is about neither. Releases always live on <artist>.bandcamp.com.
  if (new URL(url).hostname.toLowerCase() === 'bandcamp.com') {
    return {
      valid: false,
      error: 'URL must be on the artist subdomain, e.g. https://artist.bandcamp.com/album/title',
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

  // Trimmed HERE, at the boundary. `new URL()` and `fetch()` both ignore
  // surrounding whitespace, so an untrimmed value validates and resolves
  // happily and is then refused by the backend, which deliberately stores only
  // what it validated, with a message about the URL shape that says nothing
  // about the invisible characters actually at fault. The admin UI already
  // trims; this covers direct API callers.
  const bandcamp_url =
    typeof body.bandcamp_url === 'string' ? body.bandcamp_url.trim() : body.bandcamp_url
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
