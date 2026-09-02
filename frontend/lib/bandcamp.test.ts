import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { describe, it, expect, vi, afterEach } from 'vitest'
import {
  parseBandcampEmbedId,
  swapAlbumTrackPath,
  resolveBandcampEmbed,
  isAllowedBandcampUrl,
  isBandcampReleaseUrl,
  bandcampEmbedSrc,
} from './bandcamp'

describe('bandcampEmbedSrc', () => {
  it('builds the dark-default player src for an album (MusicEmbed usage)', () => {
    const src = bandcampEmbedSrc({ kind: 'album', id: '12345' })
    expect(src).toBe(
      'https://bandcamp.com/EmbeddedPlayer/album=12345/size=large/bgcol=1a1a1a/linkcol=f59e0b/tracklist=false/artwork=small/'
    )
  })
  it('uses the track segment for a track', () => {
    const src = bandcampEmbedSrc({ kind: 'track', id: '777' })
    expect(src).toContain('track=777')
    expect(src).not.toContain('album=')
  })
  it('honors overrides + transparent (blog <Bandcamp> usage)', () => {
    const src = bandcampEmbedSrc({
      kind: 'album',
      id: '1',
      size: 'small',
      bgcol: 'ffffff',
      linkcol: '0687f5',
      artwork: 'big',
      tracklist: true,
      transparent: true,
    })
    for (const part of [
      'album=1',
      'size=small',
      'bgcol=ffffff',
      'linkcol=0687f5',
      'tracklist=true',
      'artwork=big',
      'transparent=true',
    ]) {
      expect(src).toContain(part)
    }
  })
  it('omits transparent unless requested', () => {
    expect(bandcampEmbedSrc({ kind: 'album', id: '1' })).not.toContain('transparent')
  })
})

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

describe('isBandcampReleaseUrl (outbound-link guard)', () => {
  it('accepts an album or track page on bandcamp.com or a subdomain', () => {
    expect(isBandcampReleaseUrl('https://kingbuffalo.bandcamp.com/album/regenerator')).toBe(true)
    expect(isBandcampReleaseUrl('https://x.bandcamp.com/track/leyenda')).toBe(true)
    expect(isBandcampReleaseUrl('https://bandcamp.com/album/x')).toBe(true)
  })
  it('rejects a host that merely mentions bandcamp', () => {
    // The threat this exists for: `bandcamp_embed_url` is contributor-writable
    // and not URL-validated on write, and this value is rendered as an outbound
    // link under a "Buy ... on Bandcamp" accessible name.
    expect(isBandcampReleaseUrl('https://evil.test/album/checkout')).toBe(false)
    expect(isBandcampReleaseUrl('https://bandcamp.com.attacker.test/album/x')).toBe(false)
    expect(isBandcampReleaseUrl('https://evil.test/?next=https://x.bandcamp.com/album/y')).toBe(false)
  })
  it('rejects a Bandcamp page that is not a release', () => {
    expect(isBandcampReleaseUrl('https://kingbuffalo.bandcamp.com')).toBe(false)
    expect(isBandcampReleaseUrl('https://kingbuffalo.bandcamp.com/music')).toBe(false)
    expect(isBandcampReleaseUrl('https://kingbuffalo.bandcamp.com/merch/shirt')).toBe(false)
  })
  it('reads the path, not the whole URL', () => {
    // A host that carries "/album/" only in a query string it controls.
    expect(isBandcampReleaseUrl('https://evil.test/x?y=/album/z')).toBe(false)
    // And the real segment still wins when a query string also mentions one.
    expect(isBandcampReleaseUrl('https://x.bandcamp.com/track/t?ref=/album/a')).toBe(true)
  })
  it('rejects non-https and unparseable input', () => {
    expect(isBandcampReleaseUrl('http://x.bandcamp.com/album/x')).toBe(false)
    expect(isBandcampReleaseUrl('javascript:alert(1)//bandcamp.com/album/x')).toBe(false)
    expect(isBandcampReleaseUrl('   ')).toBe(false)
    expect(isBandcampReleaseUrl('')).toBe(false)
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

// The store-subset-of-render contract, from the READ side (PSY-1966).
//
// THE CONTRACT: anything the Go write gate (utils.IsValidBandcampEmbedURL) will
// store must be renderable here. Nothing may be storable and unrenderable, or a
// curator's save silently produces no link and nobody can see why.
//
// Both halves read ONE shared corpus, checked in beside the Go test. That file
// is the tripwire: a hand-copied list in this file could only fail for shapes
// someone had already thought of, and did in fact miss the backslash divergence
// (`/album/x\..\..\evil` — a segment delimiter to this parser, an ordinary
// byte to Go) until a review found it. Adding a case now obliges both languages
// to agree about it.
//
// The reverse direction does NOT hold and the corpus records where: the Go gate
// is deliberately the stricter side on the bandcamp.com apex and on surrounding
// whitespace, both of which this parser accepts.
describe('cross-language corpus (store is a subset of render)', () => {
  // Read at RUNTIME, not imported.
  //
  // `import ... from '../../backend/...json'` would pull a backend file into the
  // frontend TypeScript program, and `next build` typechecks that program — so a
  // Vercel project rooted at frontend/ without "include source files outside the
  // root directory" would fail the BUILD on a test fixture. The two existing
  // cross-boundary references in e2e/ are runtime path.resolve calls for exactly
  // this reason. readFileSync keeps the single shared source of truth while
  // staying invisible to tsc and to the bundle.
  const corpus = JSON.parse(
    readFileSync(
      resolve(__dirname, '../../backend/internal/utils/testdata/bandcamp_url_corpus.json'),
      'utf8'
    )
  ) as {
    storable: string[]
    rejected: { url: string; why: string; alsoRejectedByReader: boolean }[]
  }

  it('has entries on both sides', () => {
    expect(corpus.storable.length).toBeGreaterThan(0)
    expect(corpus.rejected.length).toBeGreaterThan(0)
  })

  it.each(corpus.storable)('renders anything the backend will store: %s', (url) => {
    expect(isBandcampReleaseUrl(url)).toBe(true)
  })

  const mustAlsoReject = corpus.rejected.filter((c) => c.alsoRejectedByReader)
  it.each(mustAlsoReject.map((c) => [c.url, c.why] as const))(
    'refuses %s (%s)',
    (url) => {
      expect(isBandcampReleaseUrl(url)).toBe(false)
    }
  )

  // The deltas the corpus marks as writer-only strictness. Asserting they render
  // is what keeps "the read gate is the lenient side here" honest rather than
  // aspirational — if one of these ever stopped rendering, a legacy row would
  // lose its link with nothing to say so.
  const readerAccepts = corpus.rejected.filter((c) => !c.alsoRejectedByReader)
  it.each(readerAccepts.map((c) => [c.url, c.why] as const))(
    'still renders %s, where the writer is deliberately stricter (%s)',
    (url) => {
      expect(isBandcampReleaseUrl(url)).toBe(true)
    }
  )
})
