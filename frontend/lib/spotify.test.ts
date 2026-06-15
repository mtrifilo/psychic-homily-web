import { describe, it, expect, vi, afterEach } from 'vitest'
import {
  parseSpotifyArtistId,
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
  it('rejects a non-22-char id (hallucinated short/long)', () => {
    expect(parseSpotifyArtistId('https://open.spotify.com/artist/abc123')).toBeNull()
    expect(
      parseSpotifyArtistId(`https://open.spotify.com/artist/${ID}EXTRA`)
    ).toBeNull()
  })
  it('rejects a non-artist path, the wrong host, and garbage', () => {
    expect(parseSpotifyArtistId(`https://open.spotify.com/track/${ID}`)).toBeNull()
    expect(parseSpotifyArtistId(`https://evil.test/artist/${ID}`)).toBeNull()
    expect(
      parseSpotifyArtistId(`https://open.spotify.com.evil.test/artist/${ID}`)
    ).toBeNull()
    expect(parseSpotifyArtistId('not a url')).toBeNull()
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
