import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { describe, it, expect } from 'vitest'

import {
  ANCHORED_SOCIAL_LINK_PLATFORM_KEYS,
  SOCIAL_LINK_PLATFORMS,
  SOCIAL_LINK_PLATFORM_KEYS,
  hasRenderableSocialLink,
  renderableSocialLinks,
  socialLinkHref,
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

describe('cross-language corpus (the write anchor and this gate are one rule)', () => {
  it('has entries on every side', () => {
    expect(Object.keys(corpus.platforms).length).toBeGreaterThan(0)
    expect(corpus.storable.length).toBeGreaterThan(0)
    expect(corpus.refusedByWriter.length).toBeGreaterThan(0)
  })

  // The drift tripwire: the Go table asserts the same object.
  it('the host table matches the backend table', () => {
    const mine = Object.fromEntries(
      ANCHORED_SOCIAL_LINK_PLATFORM_KEYS.map(key => [
        key,
        [...(SOCIAL_LINK_PLATFORMS[key].hosts ?? [])],
      ])
    )
    expect(mine).toEqual(corpus.platforms)
  })

  it('the unanchored fields match the backend', () => {
    const mine = SOCIAL_LINK_PLATFORM_KEYS.filter(
      key => SOCIAL_LINK_PLATFORMS[key].hosts === null
    )
    expect(mine).toEqual(corpus.unanchored)
  })

  it.each(corpus.storable.map(c => [c.field, c.value, c.why] as const))(
    'renders anything the backend will store: %s %s',
    (field, value) => {
      expect(socialLinkHref(field, value)).not.toBeNull()
    }
  )

  // The two shapes where Go is the lenient parser and the browser refuses the
  // value outright. Asserting them keeps the divergence pinned: closing it on
  // the write side would make these rows fail here and force the corpus to
  // move them, rather than leaving the contract quietly untrue.
  it.each(
    corpus.storableButUnrenderable.map(c => [c.field, c.value, c.why] as const)
  )('renders nothing for the storable-but-unparseable %s %s (%s)', (field, value) => {
    expect(socialLinkHref(field, value)).toBeNull()
  })

  const mustRefuse = corpus.refusedByWriter.filter(c => !c.rendersAnyway)
  it.each(mustRefuse.map(c => [c.field, c.value, c.why] as const))(
    'refuses %s %s (%s)',
    (field, value) => {
      expect(socialLinkHref(field, value)).toBeNull()
    }
  )

  // The legacy tolerance, asserted rather than assumed: these are the shapes
  // the write boundary refuses and the reader deliberately keeps, because the
  // value still lands on the platform. Without these rows a later tightening
  // would silently take away links that work.
  const stillRenders = corpus.refusedByWriter.filter(c => c.rendersAnyway)
  it.each(stillRenders.map(c => [c.field, c.value, c.why] as const))(
    'still renders the legacy shape %s %s (%s)',
    (field, value) => {
      expect(socialLinkHref(field, value)).not.toBeNull()
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
    expect(socialLinkHref('myspace', 'https://myspace.com/band')).toBeNull()
  })

  it('does not resolve inherited Object properties as platforms', () => {
    expect(socialLinkHref('constructor', 'https://instagram.com/x')).toBeNull()
    expect(socialLinkHref('toString', 'https://instagram.com/x')).toBeNull()
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
