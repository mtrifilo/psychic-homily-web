import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { describe, it, expect } from 'vitest'

import {
  SOCIAL_LINK_PLATFORMS,
  SOCIAL_LINK_PLATFORM_KEYS,
  hasRenderableSocialLink,
  renderableSocialLinks,
  socialLinkHref,
  type SocialLinkPlatform,
} from './socialLinks'

// Read at RUNTIME, not imported.
//
// `import ... from '../../backend/...json'` would pull a backend file into the
// frontend TypeScript program, and `next build` typechecks that program, so a
// Vercel project rooted at frontend/ without "include source files outside the
// root directory" would fail the BUILD on a test fixture. readFileSync keeps the
// single shared source of truth while staying invisible to tsc and to the bundle.
const corpus = JSON.parse(
  readFileSync(
    resolve(__dirname, '../../backend/internal/utils/testdata/social_link_corpus.json'),
    'utf8'
  )
) as {
  platforms: Record<string, string[]>
  unanchored: string[]
  storable: { field: string; value: string; why: string }[]
  storableButUnrenderable: { field: string; value: string; why: string }[]
  refusedByWriter: {
    field: string
    value: string
    why: string
    rendersAnyway: boolean
  }[]
}

/**
 * The gate's parameter is typed, so a production typo fails the build. The
 * corpus rows and the unknown-key cases below deliberately feed it names that
 * are not platform keys, which is what its runtime own-property guard exists
 * for, so they pass through this one cast rather than widening the signature.
 */
function hrefFor(field: string, value: string | null | undefined): string | null {
  return socialLinkHref(field as SocialLinkPlatform, value)
}

/** The corpus rows as `it.each` tuples. */
function rows<T extends { field: string; value: string; why: string }>(cases: T[]) {
  return cases.map(c => [c.field, c.value, c.why] as const)
}

describe('cross-language corpus (the write anchor and this gate are one rule)', () => {
  it('has entries on every side', () => {
    expect(Object.keys(corpus.platforms).length).toBeGreaterThan(0)
    expect(corpus.storable.length).toBeGreaterThan(0)
    // Named explicitly: an empty array turns its `it.each` into zero tests, so
    // the pinned divergence would stop being asserted with nothing failing.
    expect(corpus.storableButUnrenderable.length).toBeGreaterThan(0)
    expect(corpus.refusedByWriter.length).toBeGreaterThan(0)
  })

  // The drift tripwire: the Go table asserts the same object.
  it('the host table matches the backend table', () => {
    const mine = Object.fromEntries(
      SOCIAL_LINK_PLATFORM_KEYS.filter(
        key => SOCIAL_LINK_PLATFORMS[key].hosts !== null
      ).map(key => [key, [...(SOCIAL_LINK_PLATFORMS[key].hosts ?? [])]])
    )
    expect(mine).toEqual(corpus.platforms)
  })

  it('the unanchored fields match the backend', () => {
    const mine = SOCIAL_LINK_PLATFORM_KEYS.filter(
      key => SOCIAL_LINK_PLATFORMS[key].hosts === null
    )
    expect(mine).toEqual(corpus.unanchored)
  })

  it.each(rows(corpus.storable))(
    'renders anything the backend will store: %s %s',
    (field, value) => {
      expect(hrefFor(field, value)).not.toBeNull()
    }
  )

  // The two shapes where Go is the lenient parser and the browser refuses the
  // value outright. Asserting them keeps the divergence pinned: closing it on
  // the write side would make these rows fail here and force the corpus to
  // move them, rather than leaving the contract quietly untrue.
  it.each(rows(corpus.storableButUnrenderable))(
    'renders nothing for the storable-but-unparseable %s %s (%s)',
    (field, value) => {
      expect(hrefFor(field, value)).toBeNull()
    }
  )

  const mustRefuse = corpus.refusedByWriter.filter(c => !c.rendersAnyway)
  it.each(rows(mustRefuse))('refuses %s %s (%s)', (field, value) => {
    expect(hrefFor(field, value)).toBeNull()
  })

  // The legacy tolerance, asserted rather than assumed: these are the shapes
  // the write boundary refuses and the reader deliberately keeps, because the
  // value still lands on the platform. Without these rows a later tightening
  // would silently take away links that work.
  const stillRenders = corpus.refusedByWriter.filter(c => c.rendersAnyway)
  it.each(rows(stillRenders))(
    'still renders the legacy shape %s %s (%s)',
    (field, value) => {
      expect(hrefFor(field, value)).not.toBeNull()
    }
  )

  it('exercises every field the component renders', () => {
    const exercised = new Set([
      ...corpus.storable.map(c => c.field),
      ...corpus.refusedByWriter.map(c => c.field),
    ])
    for (const key of SOCIAL_LINK_PLATFORM_KEYS) {
      expect(exercised.has(key)).toBe(true)
    }
  })
})

describe('socialLinkHref', () => {
  it('returns null for an unknown platform key', () => {
    expect(hrefFor('myspace', 'https://myspace.com/band')).toBeNull()
  })

  it('does not resolve inherited Object properties as platforms', () => {
    expect(hrefFor('constructor', 'https://instagram.com/x')).toBeNull()
    expect(hrefFor('toString', 'https://instagram.com/x')).toBeNull()
  })

  it('returns null for a missing, blank or whitespace-only value', () => {
    expect(socialLinkHref('instagram', null)).toBeNull()
    expect(socialLinkHref('instagram', undefined)).toBeNull()
    expect(socialLinkHref('instagram', '')).toBeNull()
    expect(socialLinkHref('instagram', '   ')).toBeNull()
  })

  it('returns the exact string it parsed, so the href and the anchor agree', () => {
    expect(socialLinkHref('instagram', 'https://instagram.com/calexico')).toBe(
      'https://instagram.com/calexico'
    )
    expect(socialLinkHref('instagram', '  https://instagram.com/calexico  ')).toBe(
      'https://instagram.com/calexico'
    )
  })

  it('resolves a bare handle onto the platform, @ prefix and all', () => {
    expect(socialLinkHref('instagram', 'calexico')).toBe('https://instagram.com/calexico')
    expect(socialLinkHref('instagram', '@calexico')).toBe('https://instagram.com/calexico')
    expect(socialLinkHref('soundcloud', 'calexico')).toBe('https://soundcloud.com/calexico')
  })

  // `handleBase` is the half of the registry the shared corpus cannot pin: it
  // is a render concern with no backend counterpart, and a typo in it still
  // produces an ANCHORED url, so every corpus assertion would still pass while
  // every legacy handle resolved to the wrong page. One exact case per field
  // that has one.
  it('pins where each platform resolves a handle', () => {
    expect(socialLinkHref('instagram', 'calexico')).toBe('https://instagram.com/calexico')
    expect(socialLinkHref('facebook', 'calexico')).toBe('https://facebook.com/calexico')
    expect(socialLinkHref('twitter', 'calexico')).toBe('https://twitter.com/calexico')
    expect(socialLinkHref('youtube', 'calexico')).toBe('https://youtube.com/calexico')
    expect(socialLinkHref('spotify', 'artist/abc')).toBe(
      'https://open.spotify.com/artist/abc'
    )
    expect(socialLinkHref('soundcloud', 'calexico')).toBe('https://soundcloud.com/calexico')
  })

  // The two fields with no handle base: their account URL is a subdomain or an
  // arbitrary host, so there is nothing a handle could be appended to.
  it('renders nothing for a bare handle on a field with no handle base', () => {
    expect(socialLinkHref('bandcamp', 'calexico')).toBeNull()
    expect(socialLinkHref('website', 'calexico')).toBeNull()
  })

  it('refuses a value that is neither a URL nor a handle', () => {
    expect(socialLinkHref('website', '123')).toBeNull()
    expect(socialLinkHref('website', 'not a url')).toBeNull()
  })

  it('takes the same branch for a value however it is cased', () => {
    expect(socialLinkHref('instagram', 'evil.com')).toBeNull()
    expect(socialLinkHref('instagram', 'EVIL.COM')).toBeNull()
  })

  it('refuses userinfo, which is text a browser discards', () => {
    expect(
      socialLinkHref('spotify', 'https://evil.test@open.spotify.com/artist/abc')
    ).toBeNull()
    expect(
      socialLinkHref('spotify', 'https://open.spotify.com/artist/abc')
    ).toBe('https://open.spotify.com/artist/abc')
  })

  it('never produces a non-http href from a hostile scheme', () => {
    for (const key of SOCIAL_LINK_PLATFORM_KEYS) {
      expect(socialLinkHref(key, 'javascript:alert(1)')).toBeNull()
      expect(socialLinkHref(key, 'data:text/html,x')).toBeNull()
      expect(socialLinkHref(key, 'vbscript:msgbox(1)')).toBeNull()
    }
  })

  it('anchors every platform on its own hosts, not a sibling platform', () => {
    expect(socialLinkHref('spotify', 'https://soundcloud.com/calexico')).toBeNull()
    expect(socialLinkHref('bandcamp', 'https://instagram.com/calexico')).toBeNull()
    expect(socialLinkHref('twitter', 'https://facebook.com/calexico')).toBeNull()
  })

  it('accepts any host on the unanchored website field', () => {
    expect(socialLinkHref('website', 'https://anything.example.test/tour')).toBe(
      'https://anything.example.test/tour'
    )
  })
})

describe('renderableSocialLinks', () => {
  it('returns nothing for null, undefined or an all-empty object', () => {
    expect(renderableSocialLinks(null)).toEqual([])
    expect(renderableSocialLinks(undefined)).toEqual([])
    expect(renderableSocialLinks({ instagram: null, spotify: '' })).toEqual([])
  })

  it('keeps registry order regardless of key order in the row', () => {
    const links = renderableSocialLinks({
      soundcloud: 'calexico',
      website: 'https://calexico.example.com',
      instagram: 'calexico',
    })
    expect(links.map(l => l.platform)).toEqual(['website', 'instagram', 'soundcloud'])
  })

  it('drops the refused value and keeps the conforming ones beside it', () => {
    const links = renderableSocialLinks({
      instagram: 'https://instagram.com/calexico',
      spotify: 'https://spotify-account-verify.evil.test/',
      bandcamp: 'https://calexico.bandcamp.com',
    })
    expect(links.map(l => l.platform)).toEqual(['instagram', 'bandcamp'])
  })

  it('carries the label the surfaces print', () => {
    const links = renderableSocialLinks({ twitter: 'calexico' })
    expect(links[0].label).toBe('Twitter/X')
  })
})

describe('hasRenderableSocialLink', () => {
  it('is false when every stored value is refused', () => {
    expect(
      hasRenderableSocialLink({
        spotify: 'https://spotify-account-verify.evil.test/',
        instagram: 'https://instagram.com.evil.test/x',
      })
    ).toBe(false)
  })

  it('is true when one value survives', () => {
    expect(
      hasRenderableSocialLink({
        spotify: 'https://spotify-account-verify.evil.test/',
        instagram: 'https://instagram.com/calexico',
      })
    ).toBe(true)
  })
})
