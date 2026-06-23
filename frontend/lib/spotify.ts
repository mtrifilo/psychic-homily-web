// Shared Spotify artist-URL parsing + validation.
//
// "Valid Spotify artist URL" was previously defined four different ways across
// the codebase (an anchored discovery regex that dropped `?si=` share links, a
// loose save-route check, a backend substring check, and the embed extractor),
// which let a URL that one layer accepted be silently rejected by another. This
// module is the single source of truth: parse the URL, require the canonical
// host, and enforce the 22-char base62 artist id.

// Spotify entity IDs are base62, exactly 22 characters across all entity types
// (artist, album, track). Keep the id pattern in one place so the URI and path
// matchers stay in agreement.
const SPOTIFY_ID = '[A-Za-z0-9]{22}'
const SPOTIFY_URI_RE = new RegExp(`^spotify:artist:(${SPOTIFY_ID})$`)
const SPOTIFY_PATH_RE = new RegExp(`/artist/(${SPOTIFY_ID})(?:/|$)`)
const SPOTIFY_OEMBED_TIMEOUT_MS = 8000

// Extract the artist id from a web URL or a `spotify:artist:<id>` URI. Tolerant
// of the shapes that legitimately occur so existing/pasted links all resolve to
// the same id: a `?si=...` share suffix, a missing scheme (legacy stored
// values), a locale prefix (`/intl-de/artist/...`), and a trailing segment
// (`/artist/<id>/about`). But it REQUIRES the canonical open.spotify.com host
// and the 22-char base62 id — a slug or hallucinated id returns null rather than
// resolving to a broken embed.
export function parseSpotifyArtistId(input: string): string | null {
  const uri = input.match(SPOTIFY_URI_RE)
  if (uri) return uri[1]

  let parsed: URL
  try {
    parsed = new URL(input)
  } catch {
    // Retry as a scheme-less value (e.g. a stored "open.spotify.com/artist/...").
    try {
      parsed = new URL(`https://${input}`)
    } catch {
      return null
    }
  }
  if (parsed.hostname.toLowerCase() !== 'open.spotify.com') return null
  // pathname excludes the query string, so `?si=...` is tolerated; matching the
  // segment anywhere in the path tolerates locale prefixes and trailing parts.
  const path = parsed.pathname.match(SPOTIFY_PATH_RE)
  return path ? path[1] : null
}

export function isValidSpotifyArtistUrl(input: string): boolean {
  return parseSpotifyArtistId(input) !== null
}

// The Spotify entity types MusicEmbed can render as a playable iframe. An
// artist URL drives the artist-page embed; album/track URLs drive the
// release-page embed (PSY-1195).
export type SpotifyEmbedKind = 'artist' | 'album' | 'track'

const SPOTIFY_EMBED_URI_RE = new RegExp(
  `^spotify:(artist|album|track):(${SPOTIFY_ID})$`
)
const SPOTIFY_EMBED_PATH_RE = new RegExp(
  `/(artist|album|track)/(${SPOTIFY_ID})(?:/|$)`
)

// Parse an artist, album, OR track Spotify URL/URI into its embed kind + id.
//
// This is the embed-rendering counterpart to parseSpotifyArtistId (which stays
// artist-only for the save/validation flows that must reject non-artist URLs).
// It applies the SAME host-anchoring and 22-char base62 id discipline so a
// hallucinated id, a slug, or a look-alike host (`open.spotify.com.evil.test`)
// returns null rather than resolving to a broken — or attacker-pointed — embed.
// The id+kind it returns are the only values that reach the iframe src, so the
// caller's raw input never flows into the embed URL untrusted.
export function parseSpotifyEmbed(
  input: string
): { kind: SpotifyEmbedKind; id: string } | null {
  const uri = input.match(SPOTIFY_EMBED_URI_RE)
  if (uri) return { kind: uri[1] as SpotifyEmbedKind, id: uri[2] }

  let parsed: URL
  try {
    parsed = new URL(input)
  } catch {
    // Retry as a scheme-less value (e.g. a stored "open.spotify.com/album/...").
    try {
      parsed = new URL(`https://${input}`)
    } catch {
      return null
    }
  }
  if (parsed.hostname.toLowerCase() !== 'open.spotify.com') return null
  // pathname excludes the query string, so `?si=...` is tolerated; matching the
  // segment anywhere in the path tolerates locale prefixes (`/intl-de/album/…`)
  // and trailing parts (`/album/<id>/...`).
  const path = parsed.pathname.match(SPOTIFY_EMBED_PATH_RE)
  return path ? { kind: path[1] as SpotifyEmbedKind, id: path[2] } : null
}

export type ResolveSpotifyResult =
  | { ok: true; id: string; canonicalUrl: string }
  | { ok: false; status: number | null; error: string }

// Verify a Spotify artist actually exists — the Spotify analogue of the
// Bandcamp save-time existence check (same intent; the fetch details differ).
// Spotify's public oEmbed endpoint 404s for non-existent artists and needs no
// auth. The outbound request targets the HARDCODED oEmbed host and passes the
// CANONICAL url we rebuild from the validated id — never the raw caller input —
// so this is not an SSRF/injection vector.
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
