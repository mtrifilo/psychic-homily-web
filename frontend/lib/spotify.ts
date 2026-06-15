// Shared Spotify artist-URL parsing + validation.
//
// "Valid Spotify artist URL" was previously defined four different ways across
// the codebase (an anchored discovery regex that dropped `?si=` share links, a
// loose save-route check, a backend substring check, and the embed extractor),
// which let a URL that one layer accepted be silently rejected by another. This
// module is the single source of truth: parse the URL, require the canonical
// host, and enforce the 22-char base62 artist id.

// Spotify artist IDs are base62, exactly 22 characters.
const SPOTIFY_ARTIST_ID_RE = /^[A-Za-z0-9]{22}$/
const SPOTIFY_OEMBED_TIMEOUT_MS = 8000

// Extract the artist id from a web URL (https://open.spotify.com/artist/<id>,
// with or without a trailing slash or query string like `?si=...`) or a
// `spotify:artist:<id>` URI. Returns null if it isn't a valid artist reference.
export function parseSpotifyArtistId(input: string): string | null {
  const uri = input.match(/^spotify:artist:([A-Za-z0-9]+)$/)
  if (uri) return SPOTIFY_ARTIST_ID_RE.test(uri[1]) ? uri[1] : null

  let parsed: URL
  try {
    parsed = new URL(input)
  } catch {
    return null
  }
  if (parsed.hostname.toLowerCase() !== 'open.spotify.com') return null
  // pathname excludes the query string, so `?si=...` share params are tolerated.
  const path = parsed.pathname.match(/^\/artist\/([A-Za-z0-9]+)\/?$/)
  if (!path) return null
  return SPOTIFY_ARTIST_ID_RE.test(path[1]) ? path[1] : null
}

export function isValidSpotifyArtistUrl(input: string): boolean {
  return parseSpotifyArtistId(input) !== null
}

export type ResolveSpotifyResult =
  | { ok: true; id: string; canonicalUrl: string }
  | { ok: false; status: number | null; error: string }

// Verify a Spotify artist actually exists (mirrors the Bandcamp save-time
// check). Spotify's public oEmbed endpoint 404s for non-existent artists and
// needs no auth. The outbound request targets the HARDCODED oEmbed host and
// passes the CANONICAL url we rebuild from the validated id — never the raw
// caller input — so this is not an SSRF/injection vector.
export async function resolveSpotifyArtist(
  input: string
): Promise<ResolveSpotifyResult> {
  const id = parseSpotifyArtistId(input)
  if (!id) {
    return {
      ok: false,
      status: null,
      error:
        'URL must be a Spotify artist URL (https://open.spotify.com/artist/...)',
    }
  }

  const canonicalUrl = `https://open.spotify.com/artist/${id}`
  const oembedUrl = `https://open.spotify.com/oembed?url=${encodeURIComponent(canonicalUrl)}`
  try {
    const response = await fetch(oembedUrl, {
      headers: { Accept: 'application/json' },
      redirect: 'error',
      signal: AbortSignal.timeout(SPOTIFY_OEMBED_TIMEOUT_MS),
    })
    if (response.status === 404) {
      return { ok: false, status: 404, error: 'No Spotify artist found at that URL' }
    }
    if (!response.ok) {
      return {
        ok: false,
        status: response.status,
        error: `Failed to verify Spotify artist: ${response.status}`,
      }
    }
    return { ok: true, id, canonicalUrl }
  } catch {
    return { ok: false, status: null, error: 'Failed to verify Spotify artist' }
  }
}
