import { NextRequest, NextResponse } from 'next/server'
import { cookies } from 'next/headers'
import Anthropic from '@anthropic-ai/sdk'
import * as Sentry from '@sentry/nextjs'
import { isValidSpotifyArtistUrl } from '@/lib/spotify'
import { getAuthenticatedUser } from '@/lib/admin-artist-route'

const BACKEND_URL = process.env.BACKEND_URL || 'http://localhost:8080'

// web_search on Haiku (up to 5 uses) can run long. Bound the platform function
// and the Anthropic call so a stalled upstream returns a clean error instead of
// an opaque platform 504 that skips the error taxonomy below. The SDK timeout
// sits just under maxDuration so it fires first.
export const maxDuration = 60
const DISCOVERY_TIMEOUT_MS = 55_000

// Discovery returns CANDIDATES only — the admin reviews and explicitly picks
// before anything is saved. Same-name bands routinely collide; a name-only
// auto-pick attached the wrong act's streaming links.
const SYSTEM_PROMPT = `You are helping disambiguate an artist for a music database. Many bands share the same name. For the artist provided, search the web and return UP TO 4 plausible official Bandcamp album/track pages and UP TO 4 plausible official Spotify artist pages.

Output STRICT JSON only — no prose, no markdown code fences. Shape:
{
  "bandcamp": [{"url": "...", "name_as_listed": "...", "location": "..."|null, "notable_release": "..."|null, "genres": "..."|null, "popularity": "..."|null, "confidence": "high"|"medium"|"low", "why_might_match": "..."|null}],
  "spotify":  [{"url": "...", "name_as_listed": "...", "location": "..."|null, "notable_release": "..."|null, "genres": "..."|null, "popularity": "..."|null, "confidence": "high"|"medium"|"low", "why_might_match": "..."|null}]
}

Rules:
- Bandcamp URLs MUST be album or track URLs (e.g. https://artist.bandcamp.com/album/name), NOT profile URLs.
- A standalone single lives at https://artist.bandcamp.com/track/<slug>; a multi-track release at https://artist.bandcamp.com/album/<slug>. Copy the EXACT URL from the search result — do NOT guess the /album/ vs /track/ segment or invent a slug.
- Spotify URLs MUST be artist URLs in the form https://open.spotify.com/artist/{22-char-id}.
- Include EVERY plausible same-name candidate you find, not just the most popular. The admin will disambiguate.
- "popularity" is freeform text — for Spotify, include monthly listeners or follower count if known.
- "confidence" reflects how likely this is the correct artist match.
- If a field is unknown, use null. If no candidates for a platform, return an empty array.
- Output ONLY the JSON object.`

interface Artist {
  id: number
  name: string
}

export interface DiscoveryCandidate {
  url: string
  name_as_listed: string | null
  location: string | null
  notable_release: string | null
  genres: string | null
  popularity: string | null
  confidence: 'high' | 'medium' | 'low'
  why_might_match: string | null
}

async function getArtist(artistId: string): Promise<Artist | null> {
  try {
    const response = await fetch(`${BACKEND_URL}/artists/${artistId}`)
    if (!response.ok) return null
    return await response.json()
  } catch {
    return null
  }
}

// Haiku sometimes wraps JSON in ```json fences with preamble prose despite a
// "JSON only" instruction; strip the fence then locate the outer object.
function extractCandidateJson(
  text: string
): { bandcamp: unknown[]; spotify: unknown[] } | null {
  let cleaned = text.trim()
  const fence = cleaned.match(/```(?:json)?\s*([\s\S]*?)```/i)
  if (fence) cleaned = fence[1].trim()
  const obj = cleaned.match(/\{[\s\S]*\}/)
  if (!obj) return null
  try {
    const parsed = JSON.parse(obj[0]) as Record<string, unknown>
    return {
      bandcamp: Array.isArray(parsed.bandcamp) ? parsed.bandcamp : [],
      spotify: Array.isArray(parsed.spotify) ? parsed.spotify : [],
    }
  } catch {
    return null
  }
}

const BANDCAMP_ALBUM_RE =
  /^https?:\/\/[a-zA-Z0-9-]+\.bandcamp\.com\/(album|track)\/[a-zA-Z0-9-]+/
// Spotify candidates are kept when the shared parser accepts them (it tolerates
// the `?si=...` suffix the old anchored regex here dropped). The raw candidate
// URL is forwarded to the admin as-is; the save route canonicalizes it.

function normalizeCandidate(raw: unknown): DiscoveryCandidate | null {
  if (!raw || typeof raw !== 'object') return null
  const r = raw as Record<string, unknown>
  if (typeof r.url !== 'string' || !r.url) return null
  const confidence =
    r.confidence === 'high' || r.confidence === 'medium' || r.confidence === 'low'
      ? r.confidence
      : 'low'
  const str = (v: unknown): string | null =>
    typeof v === 'string' && v.length > 0 ? v : null
  return {
    url: r.url,
    name_as_listed: str(r.name_as_listed),
    location: str(r.location),
    notable_release: str(r.notable_release),
    genres: str(r.genres),
    popularity: str(r.popularity),
    confidence,
    why_might_match: str(r.why_might_match),
  }
}

function isCreditsError(error: unknown): boolean {
  if (error instanceof Anthropic.APIError) {
    const m = error.message.toLowerCase()
    return m.includes('credit') || m.includes('billing') || m.includes('balance')
  }
  return false
}

function isRateLimitError(error: unknown): boolean {
  return error instanceof Anthropic.APIError && error.status === 429
}

export async function POST(
  request: NextRequest,
  { params }: { params: Promise<{ id: string }> }
) {
  const { id: artistId } = await params

  // Admin session is required end-to-end. The X-Internal-Secret bypass was
  // retired now that no autonomous caller invokes this route.
  const cookieStore = await cookies()
  const authCookie = cookieStore.get('auth_token')
  if (!authCookie) {
    return NextResponse.json(
      { error: 'Authentication required' },
      { status: 401 }
    )
  }
  // Auth and artist lookup are independent (getArtist uses only the path id),
  // so kick both off concurrently. We still AWAIT auth first and return 403
  // before awaiting the artist — the gate runs before any artist result is
  // consumed, and the unauthorized path just discards an in-flight fetch.
  const profilePromise = getAuthenticatedUser(authCookie.value)
  const artistPromise = getArtist(artistId)

  const profile = await profilePromise
  if (!profile?.success || !profile.user?.is_admin) {
    return NextResponse.json(
      { error: 'Admin access required' },
      { status: 403 }
    )
  }

  const artist = await artistPromise
  if (!artist) {
    return NextResponse.json({ error: 'Artist not found' }, { status: 404 })
  }

  const apiKey = process.env.ANTHROPIC_API_KEY
  if (!apiKey) {
    Sentry.captureException(new Error('ANTHROPIC_API_KEY not configured'), {
      level: 'error',
      tags: { service: 'music-discovery' },
    })
    return NextResponse.json(
      { error: 'AI service not configured' },
      { status: 503 }
    )
  }

  try {
    const anthropic = new Anthropic({ apiKey })
    const response = await anthropic.messages.create({
      model: 'claude-haiku-4-5-20251001',
      max_tokens: 2048,
      tools: [
        {
          type: 'web_search_20250305',
          name: 'web_search',
          max_uses: 5,
        },
      ],
      system: SYSTEM_PROMPT,
      messages: [
        {
          role: 'user',
          content: `Find candidates for the artist named "${artist.name}".`,
        },
      ],
      // Single bounded attempt: the SDK retries timeouts twice by default, which
      // would blow past maxDuration and 504 before APIConnectionTimeoutError is
      // thrown. maxRetries: 0 keeps the clean timeout error path reachable.
    }, { timeout: DISCOVERY_TIMEOUT_MS, maxRetries: 0 })

    let responseText = ''
    for (const block of response.content) {
      if (block.type === 'text') responseText += block.text
    }

    const parsed = extractCandidateJson(responseText)
    if (!parsed) {
      Sentry.captureMessage(
        'music-discovery: failed to parse candidate JSON',
        {
          level: 'error',
          tags: { service: 'music-discovery', error_type: 'parse_failed' },
          extra: {
            artistId,
            artistName: artist.name,
            sample: responseText.slice(0, 800),
          },
        }
      )
      return NextResponse.json(
        {
          error: 'PARSE_FAILED',
          message:
            'AI returned an unexpected response shape. Try again or use manual entry.',
        },
        { status: 502 }
      )
    }

    // Normalize + filter to URL-shape-valid candidates per platform.
    const bandcamp = parsed.bandcamp
      .map(normalizeCandidate)
      .filter((c): c is DiscoveryCandidate => !!c && BANDCAMP_ALBUM_RE.test(c.url))
    const spotify = parsed.spotify
      .map(normalizeCandidate)
      .filter((c): c is DiscoveryCandidate => !!c && isValidSpotifyArtistUrl(c.url))

    return NextResponse.json({ bandcamp, spotify })
  } catch (error) {
    if (isRateLimitError(error)) {
      Sentry.captureException(error, {
        level: 'warning',
        tags: { service: 'music-discovery', error_type: 'rate_limit' },
      })
      return NextResponse.json(
        {
          error: 'RATE_LIMIT',
          message:
            'Discovery is rate-limited right now — try again in about a minute.',
        },
        { status: 429 }
      )
    }
    if (isCreditsError(error)) {
      Sentry.captureException(error, {
        level: 'error',
        tags: {
          service: 'music-discovery',
          error_type: 'credits_exhausted',
        },
      })
      return NextResponse.json(
        {
          error: 'API_CREDITS_EXHAUSTED',
          message:
            'Anthropic API credits exhausted. Add credits to use AI discovery.',
        },
        { status: 503 }
      )
    }
    if (error instanceof Anthropic.APIConnectionTimeoutError) {
      Sentry.captureException(error, {
        level: 'warning',
        tags: { service: 'music-discovery', error_type: 'timeout' },
      })
      return NextResponse.json(
        {
          error: 'TIMEOUT',
          message: 'Discovery timed out — try again, or use manual entry.',
        },
        { status: 504 }
      )
    }
    if (error instanceof Anthropic.APIError) {
      Sentry.captureException(error, {
        level: 'error',
        tags: { service: 'music-discovery', error_type: 'api_error' },
      })
      return NextResponse.json(
        { error: 'AI service error', message: error.message },
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
