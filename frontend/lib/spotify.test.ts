import { describe, it, expect, vi, afterEach } from 'vitest'
import {
  parseSpotifyArtistId,
  parseSpotifyEmbed,
  isValidSpotifyArtistUrl,
  resolveSpotifyArtist,
} from './spotify'

afterEach(() => {
  vi.restoreAllMocks()
})

// A real-shape 22-char base62 artist id.
const ID = '4Z8W4fKeB5YxbusRsdQVPb'

describe('parseSpotifyArtistId', () => {
  it('extracts the id from a canonical web URL', () => {
    expect(parseSpotifyArtistId(`https://open.spotify.com/artist/${ID}`)).toBe(ID)
  })
  it('tolerates a `?si=` share suffix (the bug this fixes) and a trailing slash', () => {
    expect(
      parseSpotifyArtistId(`https://open.spotify.com/artist/${ID}?si=abc123XYZ`)
    ).toBe(ID)
    expect(parseSpotifyArtistId(`https://open.spotify.com/artist/${ID}/`)).toBe(ID)
  })
  it('extracts the id from a spotify:artist: URI', () => {
    expect(parseSpotifyArtistId(`spotify:artist:${ID}`)).toBe(ID)
  })
  it('tolerates a locale prefix and a trailing path segment', () => {
    expect(parseSpotifyArtistId(`https://open.spotify.com/intl-de/artist/${ID}`)).toBe(ID)
    expect(parseSpotifyArtistId(`https://open.spotify.com/artist/${ID}/about`)).toBe(ID)
  })
  it('tolerates a scheme-less stored URL (legacy data) but still checks the host', () => {
    expect(parseSpotifyArtistId(`open.spotify.com/artist/${ID}`)).toBe(ID)
    expect(parseSpotifyArtistId(`open.spotify.com/artist/${ID}?si=x`)).toBe(ID)
    // scheme-less must NOT bypass the host allowlist
    expect(parseSpotifyArtistId(`evil.test/artist/${ID}`)).toBeNull()
  })
  it('rejects a non-22-char id (hallucinated short/long)', () => {
    expect(parseSpotifyArtistId('https://open.spotify.com/artist/abc123')).toBeNull()
    expect(
      parseSpotifyArtistId(`https://open.spotify.com/artist/${ID}EXTRA`)
    ).toBeNull()
  })
  it('rejects a non-artist path, the wrong host, and garbage', () => {
    // The save/validation flow must still reject album/track URLs even though
    // parseSpotifyEmbed (PSY-1195) now accepts them for embedding.
    expect(parseSpotifyArtistId(`https://open.spotify.com/track/${ID}`)).toBeNull()
    expect(parseSpotifyArtistId(`https://open.spotify.com/album/${ID}`)).toBeNull()
    expect(parseSpotifyArtistId(`https://evil.test/artist/${ID}`)).toBeNull()
    expect(
      parseSpotifyArtistId(`https://open.spotify.com.evil.test/artist/${ID}`)
    ).toBeNull()
    expect(parseSpotifyArtistId('not a url')).toBeNull()
  })
})

describe('parseSpotifyEmbed', () => {
  it('parses an artist URL (the artist-page path, unchanged)', () => {
    expect(parseSpotifyEmbed(`https://open.spotify.com/artist/${ID}`)).toEqual({
      kind: 'artist',
      id: ID,
    })
  })
  it('parses an album URL (the release-page path, PSY-1195)', () => {
    expect(parseSpotifyEmbed(`https://open.spotify.com/album/${ID}`)).toEqual({
      kind: 'album',
      id: ID,
    })
  })
  it('parses a track URL', () => {
    expect(parseSpotifyEmbed(`https://open.spotify.com/track/${ID}`)).toEqual({
      kind: 'track',
      id: ID,
    })
  })
  it('parses spotify: URIs for all three kinds', () => {
    expect(parseSpotifyEmbed(`spotify:artist:${ID}`)).toEqual({ kind: 'artist', id: ID })
    expect(parseSpotifyEmbed(`spotify:album:${ID}`)).toEqual({ kind: 'album', id: ID })
    expect(parseSpotifyEmbed(`spotify:track:${ID}`)).toEqual({ kind: 'track', id: ID })
  })
  it('tolerates `?si=` share suffix, trailing slash, locale prefix, trailing segment', () => {
    expect(parseSpotifyEmbed(`https://open.spotify.com/album/${ID}?si=abc123XYZ`)).toEqual({
      kind: 'album',
      id: ID,
    })
    expect(parseSpotifyEmbed(`https://open.spotify.com/album/${ID}/`)).toEqual({
      kind: 'album',
      id: ID,
    })
    expect(parseSpotifyEmbed(`https://open.spotify.com/intl-de/album/${ID}`)).toEqual({
      kind: 'album',
      id: ID,
    })
  })
  it('tolerates a scheme-less stored URL but still checks the host', () => {
    expect(parseSpotifyEmbed(`open.spotify.com/album/${ID}`)).toEqual({
      kind: 'album',
      id: ID,
    })
    // scheme-less must NOT bypass the host allowlist
    expect(parseSpotifyEmbed(`evil.test/album/${ID}`)).toBeNull()
  })
  it('rejects a look-alike host, a non-22-char id, and garbage', () => {
    expect(parseSpotifyEmbed(`https://open.spotify.com.evil.test/album/${ID}`)).toBeNull()
    expect(parseSpotifyEmbed('https://open.spotify.com/album/abc123')).toBeNull()
    expect(parseSpotifyEmbed(`https://open.spotify.com/album/${ID}EXTRA`)).toBeNull()
    expect(parseSpotifyEmbed('not a url')).toBeNull()
  })
  it('rejects non-embeddable Spotify entity types (playlist, show, episode)', () => {
    expect(parseSpotifyEmbed(`https://open.spotify.com/playlist/${ID}`)).toBeNull()
    expect(parseSpotifyEmbed(`https://open.spotify.com/show/${ID}`)).toBeNull()
    expect(parseSpotifyEmbed(`https://open.spotify.com/episode/${ID}`)).toBeNull()
  })
})

describe('isValidSpotifyArtistUrl', () => {
  it('mirrors parseSpotifyArtistId', () => {
    expect(isValidSpotifyArtistUrl(`https://open.spotify.com/artist/${ID}?si=x`)).toBe(true)
    expect(isValidSpotifyArtistUrl('https://open.spotify.com/artist/abc123')).toBe(false)
  })
})

describe('resolveSpotifyArtist', () => {
  it('rejects an invalid URL without any network request', async () => {
    const spy = vi.spyOn(global, 'fetch')
    const result = await resolveSpotifyArtist('https://open.spotify.com/track/x')
    expect(result.ok).toBe(false)
    expect(spy).not.toHaveBeenCalled()
  })

  it('returns the canonical URL (stripping `?si=`) when oEmbed confirms the artist', async () => {
    vi.spyOn(global, 'fetch').mockResolvedValueOnce({
      ok: true,
      status: 200,
    } as Response)

    const result = await resolveSpotifyArtist(
      `https://open.spotify.com/artist/${ID}?si=tracking`
    )

    expect(result).toEqual({
      ok: true,
      id: ID,
      canonicalUrl: `https://open.spotify.com/artist/${ID}`,
    })
  })

  it('fetches oEmbed with the canonical (sanitized) URL, not the raw input', async () => {
    const spy = vi.spyOn(global, 'fetch').mockResolvedValueOnce({
      ok: true,
      status: 200,
    } as Response)

    await resolveSpotifyArtist(`https://open.spotify.com/artist/${ID}?si=tracking`)

    const calledUrl = String(spy.mock.calls[0][0])
    expect(calledUrl).toContain('open.spotify.com/oembed')
    // The canonical id-only URL is what gets passed to oEmbed — no `?si=`.
    expect(calledUrl).toContain(encodeURIComponent(`https://open.spotify.com/artist/${ID}`))
    expect(calledUrl).not.toContain('tracking')
  })

  it('maps a 404 oEmbed to "not found"', async () => {
    vi.spyOn(global, 'fetch').mockResolvedValueOnce({
      ok: false,
      status: 404,
    } as Response)

    const result = await resolveSpotifyArtist(`https://open.spotify.com/artist/${ID}`)
    expect(result).toEqual({
      ok: false,
      status: 404,
      error: 'No Spotify artist found at that URL',
    })
  })

  it('surfaces a non-404 oEmbed failure', async () => {
    vi.spyOn(global, 'fetch').mockResolvedValueOnce({
      ok: false,
      status: 503,
    } as Response)

    const result = await resolveSpotifyArtist(`https://open.spotify.com/artist/${ID}`)
    expect(result.ok).toBe(false)
    if (!result.ok) expect(result.status).toBe(503)
  })

  it('handles a thrown fetch (network/timeout)', async () => {
    vi.spyOn(global, 'fetch').mockRejectedValueOnce(new Error('network'))

    const result = await resolveSpotifyArtist(`https://open.spotify.com/artist/${ID}`)
    expect(result).toEqual({
      ok: false,
      status: null,
      error: 'Failed to verify Spotify artist',
    })
  })
})
