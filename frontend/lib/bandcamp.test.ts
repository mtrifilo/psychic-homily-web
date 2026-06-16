import { describe, it, expect, vi, afterEach } from 'vitest'
import {
  parseBandcampEmbedId,
  swapAlbumTrackPath,
  resolveBandcampEmbed,
  isAllowedBandcampUrl,
} from './bandcamp'

// Every test that touches fetch installs a spy via mockFetchSequence; restore
// after each so no spy leaks into the next test.
afterEach(() => {
  vi.restoreAllMocks()
})

// Real-shape `data-embed` attributes (HTML-entity-encoded, as Bandcamp serves).
const TRACK_HTML =
  '<div data-embed="{&quot;tralbum_param&quot;:{&quot;name&quot;:&quot;track&quot;,&quot;value&quot;:2445352951},&quot;art_id&quot;:2137030374}"></div>'
const ALBUM_HTML =
  '<div data-embed="{&quot;tralbum_param&quot;:{&quot;name&quot;:&quot;album&quot;,&quot;value&quot;:123456789}}"></div>'

function mockFetchSequence(
  ...responses: Array<{ ok: boolean; status?: number; html?: string } | Error>
) {
  const spy = vi.spyOn(global, 'fetch')
  for (const r of responses) {
    if (r instanceof Error) {
      spy.mockRejectedValueOnce(r)
    } else {
      spy.mockResolvedValueOnce({
        ok: r.ok,
        status: r.status ?? (r.ok ? 200 : 404),
        text: async () => r.html ?? '',
      } as Response)
    }
  }
  return spy
}

describe('isAllowedBandcampUrl (SSRF guard)', () => {
  it('accepts https bandcamp.com and its subdomains', () => {
    expect(isAllowedBandcampUrl('https://bandcamp.com/album/x')).toBe(true)
    expect(isAllowedBandcampUrl('https://sorochemusic.bandcamp.com/track/leyenda')).toBe(true)
  })
  it('rejects the substring-bypass payloads a naive includes() would allow', () => {
    // These all contain "bandcamp.com" but resolve to attacker/internal hosts.
    expect(isAllowedBandcampUrl('http://169.254.169.254/latest/meta-data/?x=bandcamp.com')).toBe(false)
    expect(isAllowedBandcampUrl('https://bandcamp.com.attacker.test/album/x')).toBe(false)
    expect(isAllowedBandcampUrl('https://evil.test/?x=bandcamp.com')).toBe(false)
    expect(isAllowedBandcampUrl('http://localhost:8080/admin?bandcamp.com')).toBe(false)
  })
  it('rejects non-https schemes and unparseable input', () => {
    expect(isAllowedBandcampUrl('http://x.bandcamp.com/album/x')).toBe(false)
    expect(isAllowedBandcampUrl('not a url')).toBe(false)
    // A subdomain-suffix lookalike must not slip through endsWith.
    expect(isAllowedBandcampUrl('https://notbandcamp.com/album/x')).toBe(false)
  })
})

describe('swapAlbumTrackPath', () => {
  it('swaps /album/ -> /track/', () => {
    expect(swapAlbumTrackPath('https://x.bandcamp.com/album/leyenda')).toBe(
      'https://x.bandcamp.com/track/leyenda'
    )
  })
  it('swaps /track/ -> /album/', () => {
    expect(swapAlbumTrackPath('https://x.bandcamp.com/track/leyenda')).toBe(
      'https://x.bandcamp.com/album/leyenda'
    )
  })
  it('returns null when neither segment is present', () => {
    expect(swapAlbumTrackPath('https://x.bandcamp.com')).toBeNull()
  })
  it('swaps only the path segment, preserving a slug that contains album/track', () => {
    // The literal "/album/" (slash-delimited) only appears as the path type;
    // a single-segment slug can't contain it, so the slug is left intact.
    expect(
      swapAlbumTrackPath('https://x.bandcamp.com/album/my-album-remix')
    ).toBe('https://x.bandcamp.com/track/my-album-remix')
    expect(
      swapAlbumTrackPath('https://x.bandcamp.com/track/album-version?x=1')
    ).toBe('https://x.bandcamp.com/album/album-version?x=1')
  })
})

describe('parseBandcampEmbedId', () => {
  it('reads kind+id from a track data-embed', () => {
    expect(parseBandcampEmbedId(TRACK_HTML)).toEqual({
      kind: 'track',
      id: '2445352951',
    })
  })
  it('reads kind+id from an album data-embed', () => {
    expect(parseBandcampEmbedId(ALBUM_HTML)).toEqual({
      kind: 'album',
      id: '123456789',
    })
  })
  it('falls back to a bare track= identifier', () => {
    expect(
      parseBandcampEmbedId('<iframe src="EmbeddedPlayer/track=999/size=large">')
    ).toEqual({ kind: 'track', id: '999' })
  })
  it('falls back to a bare album= identifier, preferring album', () => {
    expect(
      parseBandcampEmbedId('...album=42... track=7...')
    ).toEqual({ kind: 'album', id: '42' })
  })
  it('returns null when no identifier is present', () => {
    expect(parseBandcampEmbedId('<html>nothing here</html>')).toBeNull()
  })
})

describe('resolveBandcampEmbed', () => {
  it('rejects an off-allowlist URL without making any network request', async () => {
    const spy = vi.spyOn(global, 'fetch')
    const result = await resolveBandcampEmbed(
      'http://169.254.169.254/latest/meta-data/?x=bandcamp.com'
    )
    expect(result.ok).toBe(false)
    expect(spy).not.toHaveBeenCalled()
  })

  it('resolves a reachable track URL without changing the URL', async () => {
    mockFetchSequence({ ok: true, html: TRACK_HTML })
    const result = await resolveBandcampEmbed(
      'https://sorochemusic.bandcamp.com/track/leyenda'
    )
    expect(result).toEqual({
      ok: true,
      embed: {
        kind: 'track',
        id: '2445352951',
        resolvedUrl: 'https://sorochemusic.bandcamp.com/track/leyenda',
      },
    })
  })

  it('auto-corrects /album/ -> /track/ on a 404 and persists the corrected URL', async () => {
    // First fetch (the LLM's /album/ guess) 404s; the /track/ sibling resolves.
    const spy = mockFetchSequence(
      { ok: false, status: 404 },
      { ok: true, html: TRACK_HTML }
    )
    const result = await resolveBandcampEmbed(
      'https://sorochemusic.bandcamp.com/album/leyenda'
    )
    expect(result.ok).toBe(true)
    if (result.ok) {
      expect(result.embed.kind).toBe('track')
      expect(result.embed.id).toBe('2445352951')
      expect(result.embed.resolvedUrl).toBe(
        'https://sorochemusic.bandcamp.com/track/leyenda'
      )
    }
    // Confirms it retried the sibling path.
    expect(spy).toHaveBeenNthCalledWith(
      2,
      'https://sorochemusic.bandcamp.com/track/leyenda',
      expect.anything()
    )
  })

  it('fails with the 404 status when neither path type resolves', async () => {
    mockFetchSequence({ ok: false, status: 404 }, { ok: false, status: 404 })
    const result = await resolveBandcampEmbed(
      'https://x.bandcamp.com/album/ghost'
    )
    expect(result).toEqual({
      ok: false,
      status: 404,
      error: 'Failed to fetch Bandcamp page: 404',
    })
  })

  it('does not retry the sibling on a non-404 error', async () => {
    const spy = mockFetchSequence({ ok: false, status: 503 })
    const result = await resolveBandcampEmbed(
      'https://x.bandcamp.com/album/down'
    )
    expect(result).toEqual({
      ok: false,
      status: 503,
      error: 'Failed to fetch Bandcamp page: 503',
    })
    expect(spy).toHaveBeenCalledTimes(1)
  })

  it('reports a null status when the fetch throws', async () => {
    mockFetchSequence(new Error('network down'))
    const result = await resolveBandcampEmbed(
      'https://x.bandcamp.com/track/x'
    )
    expect(result).toEqual({
      ok: false,
      status: null,
      error: 'Failed to fetch Bandcamp page',
    })
  })

  it('fails when the page loads but has no embed id', async () => {
    mockFetchSequence({ ok: true, html: '<html>no embed</html>' })
    const result = await resolveBandcampEmbed(
      'https://x.bandcamp.com/album/empty'
    )
    expect(result).toEqual({
      ok: false,
      status: 200,
      error: 'Could not extract embed ID from Bandcamp page',
    })
  })
})
