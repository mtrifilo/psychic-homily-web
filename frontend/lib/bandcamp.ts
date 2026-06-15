// Shared resolver for Bandcamp album/track pages.
//
// Bandcamp serves the same slug under EITHER /album/<slug> OR /track/<slug>,
// depending on whether the release is a full album or a standalone single.
// The two namespaces don't overlap, so a caller (the LLM "Discover Music"
// suggester especially) routinely produces an /album/ URL for what is actually
// a /track/ single — which 404s. This module fetches the page, auto-corrects
// the path type on a 404, and reads the canonical embed descriptor so both
// albums and standalone tracks can be embedded.

const BANDCAMP_FETCH_HEADERS = {
  'User-Agent': 'Mozilla/5.0 (compatible; MusicEmbed/1.0)',
}

export type BandcampEmbedKind = 'album' | 'track'

export interface BandcampEmbed {
  kind: BandcampEmbedKind
  id: string
  // The URL that actually resolved. Differs from the input when the
  // album/track path segment was auto-corrected — callers persist this so the
  // stored URL points at the page that really exists.
  resolvedUrl: string
}

export type ResolveBandcampResult =
  | { ok: true; embed: BandcampEmbed }
  | { ok: false; status: number | null; error: string }

// Swap /album/<slug> <-> /track/<slug>. Returns null when the URL has neither
// segment (e.g. a bare profile URL), so callers don't retry meaninglessly.
export function swapAlbumTrackPath(url: string): string | null {
  if (url.includes('/album/')) return url.replace('/album/', '/track/')
  if (url.includes('/track/')) return url.replace('/track/', '/album/')
  return null
}

// The page embeds its canonical player descriptor in a `data-embed` attribute,
// HTML-entity-encoded, e.g.
//   data-embed="{&quot;tralbum_param&quot;:{&quot;name&quot;:&quot;track&quot;,&quot;value&quot;:2445352951},...}"
// This is the authoritative source for BOTH the kind and the numeric id, and
// is present for albums and standalone tracks alike. Quotes may be encoded
// (&quot;) in the attribute or bare (") in inline JSON, so accept either.
const Q = '(?:&quot;|")'
const TRALBUM_PARAM_RE = new RegExp(
  `tralbum_param${Q}\\s*:\\s*\\{\\s*${Q}name${Q}\\s*:\\s*${Q}(album|track)${Q}\\s*,\\s*${Q}value${Q}\\s*:\\s*(\\d+)`
)

export function parseBandcampEmbedId(
  html: string
): { kind: BandcampEmbedKind; id: string } | null {
  const tralbum = html.match(TRALBUM_PARAM_RE)
  if (tralbum) return { kind: tralbum[1] as BandcampEmbedKind, id: tralbum[2] }

  // Fallback for pages where the descriptor isn't found: a bare album=/track=
  // identifier from an EmbeddedPlayer reference. Prefer album (a full release)
  // to preserve prior behavior; standalone-track pages carry only track=.
  const album = html.match(/album=(\d+)/)
  if (album) return { kind: 'album', id: album[1] }
  const track = html.match(/track=(\d+)/)
  if (track) return { kind: 'track', id: track[1] }

  return null
}

async function fetchBandcampPage(
  url: string
): Promise<{ ok: true; html: string } | { ok: false; status: number | null }> {
  try {
    const response = await fetch(url, { headers: BANDCAMP_FETCH_HEADERS })
    if (!response.ok) return { ok: false, status: response.status }
    return { ok: true, html: await response.text() }
  } catch {
    return { ok: false, status: null }
  }
}

// Fetch a Bandcamp album/track URL and resolve it to an embeddable descriptor.
// On a 404 for an /album/ or /track/ URL, retries the sibling path type once
// before giving up.
export async function resolveBandcampEmbed(
  inputUrl: string
): Promise<ResolveBandcampResult> {
  let resolvedUrl = inputUrl
  let page = await fetchBandcampPage(inputUrl)

  if (!page.ok && page.status === 404) {
    const sibling = swapAlbumTrackPath(inputUrl)
    if (sibling) {
      const siblingPage = await fetchBandcampPage(sibling)
      if (siblingPage.ok) {
        resolvedUrl = sibling
        page = siblingPage
      }
    }
  }

  if (!page.ok) {
    return {
      ok: false,
      status: page.status,
      error:
        page.status === null
          ? 'Failed to fetch Bandcamp page'
          : `Failed to fetch Bandcamp page: ${page.status}`,
    }
  }

  const parsed = parseBandcampEmbedId(page.html)
  if (!parsed) {
    return {
      ok: false,
      status: 200,
      error: 'Could not extract embed ID from Bandcamp page',
    }
  }

  return { ok: true, embed: { kind: parsed.kind, id: parsed.id, resolvedUrl } }
}
