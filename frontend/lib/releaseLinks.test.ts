import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { describe, it, expect } from 'vitest'

import {
  OFFERED_RELEASE_LINK_PLATFORM_KEYS,
  MAX_RELEASE_LINK_URL_LENGTH,
  RELEASE_LINK_PLATFORMS,
  RELEASE_LINK_PLATFORM_KEYS,
  findBandcampEmbedUrl,
  findSpotifyEmbedUrl,
  isRenderableReleaseLink,
  releaseLinkPlatformLabel,
  releaseLinkRefusal,
  renderableReleaseLinks,
} from './releaseLinks'

// Read at RUNTIME, not imported.
//
// `import ... from '../../backend/...json'` would pull a backend file into the
// frontend TypeScript program, and `next build` typechecks that program, so a
// Vercel project rooted at frontend/ without "include source files outside the
// root directory" would fail the BUILD on a test fixture. readFileSync keeps the
// single shared source of truth while staying invisible to tsc and to the bundle.
const corpus = JSON.parse(
  readFileSync(
    resolve(__dirname, '../../backend/internal/utils/testdata/release_link_corpus.json'),
    'utf8'
  )
) as {
  maxUrlLength: number
  platforms: Record<string, string[]>
  renderable: { platform: string; url: string }[]
  refused: {
    platform: string
    url: string
    why: string
    alsoRefusedByReader: boolean
  }[]
}

describe('cross-language corpus (the write gate and this gate are one rule)', () => {
  it('has entries on every side', () => {
    expect(Object.keys(corpus.platforms).length).toBeGreaterThan(0)
    expect(corpus.renderable.length).toBeGreaterThan(0)
    expect(corpus.refused.length).toBeGreaterThan(0)
  })

  // The drift tripwire: the Go registry asserts the same object and the same cap.
  it('the platform table matches the backend registry', () => {
    const mine = Object.fromEntries(
      RELEASE_LINK_PLATFORM_KEYS.map((key) => [key, [...RELEASE_LINK_PLATFORMS[key].hosts]])
    )
    expect(mine).toEqual(corpus.platforms)
  })

  it('the URL cap matches the backend cap', () => {
    expect(MAX_RELEASE_LINK_URL_LENGTH).toBe(corpus.maxUrlLength)
  })

  it.each(corpus.renderable.map((c) => [c.platform, c.url] as const))(
    'renders anything the backend will store: %s %s',
    (platform, url) => {
      expect(isRenderableReleaseLink({ platform, url })).toBe(true)
    }
  )

  // The third edge of the mirror, and the one that was missing: the write-side
  // hint has to refuse EVERYTHING the write gate refuses, or it green-lights a
  // value the server 422s. Driving only the render gate through the corpus is
  // how four UTS-46 rows sat approved by the hint with both suites green.
  it.each(corpus.refused.map((c) => [c.platform, c.url, c.why] as const))(
    'the write-side hint refuses %s %s (%s)',
    (platform, url) => {
      expect(releaseLinkRefusal({ platform, url })).not.toBeNull()
    }
  )

  it.each(corpus.renderable.map((c) => [c.platform, c.url] as const))(
    'the write-side hint accepts anything the backend will store: %s %s',
    (platform, url) => {
      expect(releaseLinkRefusal({ platform, url })).toBeNull()
    }
  )

  const mustAlsoRefuse = corpus.refused.filter((c) => c.alsoRefusedByReader)
  it.each(mustAlsoRefuse.map((c) => [c.platform, c.url, c.why] as const))(
    'refuses %s %s (%s)',
    (platform, url) => {
      expect(isRenderableReleaseLink({ platform, url })).toBe(false)
    }
  )

  // The deltas the corpus marks as writer-only strictness: hosts the browser's
  // UTS-46 mapping resolves back ONTO the platform. Asserting they render is
  // what keeps "the reader is the lenient side here" honest rather than
  // aspirational, so a legacy row of that shape does not silently lose a link
  // that works.
  const readerAccepts = corpus.refused.filter((c) => !c.alsoRefusedByReader)
  it.each(readerAccepts.map((c) => [c.platform, c.url, c.why] as const))(
    'still renders %s %s, where the writer is deliberately stricter (%s)',
    (platform, url) => {
      expect(isRenderableReleaseLink({ platform, url })).toBe(true)
    }
  )
})

// The two functions answer different questions and are allowed to disagree in
// exactly one direction. These pin that direction, since a client hint that
// green-lit a value the server refuses is worse than no hint.
describe('the write-side hint and the render gate', () => {
  const normalizesOntoPlatform = 'https://ünicode.bandcamp.com/album/x'

  it('refuses a host the server refuses, even though a browser normalizes it', () => {
    expect(
      releaseLinkRefusal({ platform: 'bandcamp', url: normalizesOntoPlatform })
    ).toContain('must be an http or https URL on bandcamp.com')
  })

  it('still renders that same value for a row already stored', () => {
    expect(
      isRenderableReleaseLink({ platform: 'bandcamp', url: normalizesOntoPlatform })
    ).toBe(true)
  })

  it('refuses an untrimmed value up front, and renders a stored one', () => {
    const padded = 'https://kingbuffalo.bandcamp.com/album/x '
    expect(releaseLinkRefusal({ platform: 'bandcamp', url: padded })).not.toBeNull()
    expect(isRenderableReleaseLink({ platform: 'bandcamp', url: padded })).toBe(true)
  })
})

describe('isRenderableReleaseLink', () => {
  // String.trim() strips these; the URL parser does not. Leading, that makes the
  // whole value unparseable, so certifying it would render an href that lands
  // same-origin on a 404. Trailing is conservative: one rule for both.
  it.each([
    ['non-breaking space', '\u00A0'],
    ['byte order mark', '\uFEFF'],
  ])('refuses a URL padded with a %s, which the URL parser does not strip', (_name, pad) => {
    expect(
      isRenderableReleaseLink({
        platform: 'bandcamp',
        url: `${pad}https://kingbuffalo.bandcamp.com/album/x`,
      })
    ).toBe(false)
  })

  it('refuses a URL longer than the shared cap', () => {
    const long = `https://kingbuffalo.bandcamp.com/album/${'a'.repeat(MAX_RELEASE_LINK_URL_LENGTH)}`
    expect(isRenderableReleaseLink({ platform: 'bandcamp', url: long })).toBe(false)
  })

  // The cap is a BYTE count, as the column and the Go gate measure it. Counting
  // UTF-16 units instead let a multibyte URL pass here and 422 at the server.
  it('counts the cap in bytes, not UTF-16 units', () => {
    const multibyte = `https://kingbuffalo.bandcamp.com/album/${'é'.repeat(1200)}`
    expect(multibyte.length).toBeLessThan(MAX_RELEASE_LINK_URL_LENGTH)
    expect(isRenderableReleaseLink({ platform: 'bandcamp', url: multibyte })).toBe(false)
    expect(releaseLinkRefusal({ platform: 'bandcamp', url: multibyte })).toContain(
      'characters or fewer'
    )
  })

  it('refuses an empty URL', () => {
    expect(isRenderableReleaseLink({ platform: 'bandcamp', url: '' })).toBe(false)
  })

  it('matches the platform case-insensitively', () => {
    for (const platform of ['bandcamp', 'Bandcamp', 'BANDCAMP']) {
      expect(
        isRenderableReleaseLink({
          platform,
          url: 'https://kingbuffalo.bandcamp.com/album/regenerator',
        })
      ).toBe(true)
    }
  })
})

describe('renderableReleaseLinks', () => {
  it('keeps conforming rows in stored order and drops the rest', () => {
    const links = [
      { id: 1, platform: 'bandcamp', url: 'https://evil.test/album/x' },
      { id: 2, platform: 'spotify', url: 'https://open.spotify.com/album/x' },
      { id: 3, platform: 'napster', url: 'https://us.napster.com/album/x' },
      { id: 4, platform: 'bandcamp', url: 'https://kingbuffalo.bandcamp.com/album/y' },
    ]
    expect(renderableReleaseLinks(links).map((l) => l.id)).toEqual([2, 4])
  })

  it('treats null and undefined as no links', () => {
    expect(renderableReleaseLinks(null)).toEqual([])
    expect(renderableReleaseLinks(undefined)).toEqual([])
  })
})

// These two are the shared selectors behind both the release page and the
// collection graph panel, which previously kept private copies and could feed
// MusicEmbed a link the page would not have shown.
describe('findBandcampEmbedUrl', () => {
  it('skips a bandcamp row the gate refuses', () => {
    expect(
      findBandcampEmbedUrl([
        { platform: 'bandcamp', url: 'https://evil.test/album/x' },
        { platform: 'bandcamp', url: 'https://kingbuffalo.bandcamp.com/album/ok' },
      ])
    ).toBe('https://kingbuffalo.bandcamp.com/album/ok')
  })

  it('skips a bare profile root, which resolves to no player', () => {
    expect(
      findBandcampEmbedUrl([
        { platform: 'bandcamp', url: 'https://kingbuffalo.bandcamp.com' },
      ])
    ).toBeNull()
  })

  // The old whole-URL substring test selected this; the path test does not.
  it('ignores a release path that appears only in the query', () => {
    expect(
      findBandcampEmbedUrl([
        { platform: 'bandcamp', url: 'https://kingbuffalo.bandcamp.com/merch?x=/album/y' },
      ])
    ).toBeNull()
  })
})

describe('findSpotifyEmbedUrl', () => {
  it('skips a spotify row the gate refuses', () => {
    expect(
      findSpotifyEmbedUrl([
        { platform: 'spotify', url: 'https://spotify-verify.evil.test/album/x' },
      ])
    ).toBeNull()
  })

  it('returns the first renderable spotify row', () => {
    expect(
      findSpotifyEmbedUrl([
        { platform: 'bandcamp', url: 'https://kingbuffalo.bandcamp.com/album/ok' },
        { platform: 'spotify', url: 'https://open.spotify.com/album/ok' },
      ])
    ).toBe('https://open.spotify.com/album/ok')
  })
})

describe('releaseLinkRefusal', () => {
  it('says nothing about an empty value', () => {
    expect(releaseLinkRefusal({ platform: 'bandcamp', url: '' })).toBeNull()
  })

  it('names the accepted hosts', () => {
    expect(
      releaseLinkRefusal({ platform: 'spotify', url: 'https://evil.test/album/x' })
    ).toContain('spotify.com')
  })

  // Both languages answer an unknown platform by listing the ones that exist,
  // which is the only form a submitter can act on.
  it('lists the platforms that exist for an unknown one', () => {
    const message = releaseLinkRefusal({
      platform: 'napster',
      url: 'https://us.napster.com/album/x',
    })
    expect(message).toContain('Platform must be one of')
    for (const key of RELEASE_LINK_PLATFORM_KEYS) {
      expect(message).toContain(key)
    }
  })

  it('is silent for a pair the gate accepts', () => {
    expect(
      releaseLinkRefusal({
        platform: 'bandcamp',
        url: 'https://kingbuffalo.bandcamp.com/album/ok',
      })
    ).toBeNull()
  })
})

// Which platforms the pickers offer is a product decision, not a consequence of
// the registry, so it is pinned by name. The picker tests derive their
// expectation FROM this flag, so without this nothing would notice it moving.
describe('the offered subset', () => {
  it('is the seven the picker carried before this registry existed', () => {
    expect(OFFERED_RELEASE_LINK_PLATFORM_KEYS).toEqual([
      'bandcamp',
      'spotify',
      'apple_music',
      'youtube',
      'discogs',
      'tidal',
      'soundcloud',
    ])
  })
})

describe('releaseLinkPlatformLabel', () => {
  it('names every registered platform', () => {
    expect(releaseLinkPlatformLabel('apple_music')).toBe('Apple Music')
    expect(releaseLinkPlatformLabel('youtube_music')).toBe('YouTube Music')
  })

  // An admin surface still has to name a legacy row it refuses to link, or the
  // row cannot be identified for removal.
  it('title-cases an unknown platform rather than dropping it', () => {
    expect(releaseLinkPlatformLabel('some_store')).toBe('Some Store')
  })
})
