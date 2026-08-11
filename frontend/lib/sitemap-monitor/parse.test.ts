import { describe, expect, it } from 'vitest'
import { SITEMAP_FAMILIES, FAMILY_URL_PREFIXES } from '@/app/sitemap-shards'
import {
  classifyLoc,
  detectShape,
  parseSitemapIndex,
  parseUrlset,
  showDateFromLoc,
  SHARED_CLAIMANTS,
} from './parse'

const INDEX_XML = `<?xml version="1.0" encoding="UTF-8"?>
<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <sitemap>
    <loc>https://psychichomily.com/sitemap/pages.xml</loc>
  </sitemap>
  <sitemap>
    <loc>https://psychichomily.com/sitemap/shows.xml</loc>
  </sitemap>
</sitemapindex>`

const URLSET_XML = `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
<url>
<loc>https://psychichomily.com/shows/2026-03-20-lonna-kelley-at-rv-phone-home</loc>
<lastmod>2026-03-20T02:17:34.909Z</lastmod>
<changefreq>weekly</changefreq>
<priority>0.8</priority>
</url>
<url>
<loc>https://psychichomily.com/artists/no-lastmod-here</loc>
</url>
</urlset>`

describe('detectShape', () => {
  it('identifies an index', () => {
    expect(detectShape(INDEX_XML)).toBe('index')
  })

  it('identifies a urlset', () => {
    expect(detectShape(URLSET_XML)).toBe('urlset')
  })

  // A served HTML error page must not read as an empty sitemap: counting zero
  // URLs from it looks identical to a catastrophically broken sitemap.
  it('throws on a document that is neither shape', () => {
    expect(() => detectShape('<!DOCTYPE html><html><body>404</body></html>')).toThrow(
      /neither a <sitemapindex> nor a <urlset>/
    )
  })
})

describe('parseSitemapIndex', () => {
  it('returns every child document loc', () => {
    expect(parseSitemapIndex(INDEX_XML)).toEqual([
      'https://psychichomily.com/sitemap/pages.xml',
      'https://psychichomily.com/sitemap/shows.xml',
    ])
  })

  // `<sitemap\b` must not match the `<sitemapindex>` root element.
  it('does not treat the root element as a child', () => {
    expect(parseSitemapIndex(INDEX_XML)).toHaveLength(2)
  })

  it('returns nothing for a urlset', () => {
    expect(parseSitemapIndex(URLSET_XML)).toEqual([])
  })
})

describe('parseUrlset', () => {
  it('returns every loc in document order', () => {
    expect(parseUrlset(URLSET_XML)).toEqual([
      'https://psychichomily.com/shows/2026-03-20-lonna-kelley-at-rv-phone-home',
      'https://psychichomily.com/artists/no-lastmod-here',
    ])
  })

  it('decodes XML entities in a loc', () => {
    const xml = `<urlset><url><loc>https://psychichomily.com/tags/rock&amp;roll</loc></url></urlset>`
    expect(parseUrlset(xml)[0]).toBe('https://psychichomily.com/tags/rock&roll')
  })

  it('leaves an entity-free loc untouched by the decode fast path', () => {
    const xml = `<urlset><url><loc>https://psychichomily.com/tags/punk</loc></url></urlset>`
    expect(parseUrlset(xml)[0]).toBe('https://psychichomily.com/tags/punk')
  })

  it('skips a url block with no loc', () => {
    const xml = `<urlset><url><lastmod>2026-01-01</lastmod></url></urlset>`
    expect(parseUrlset(xml)).toEqual([])
  })
})

describe('classifyLoc', () => {
  it.each([
    ['https://psychichomily.com/shows/2026-03-20-a-show', 'shows'],
    ['https://psychichomily.com/artists/some-band', 'artists'],
    ['https://psychichomily.com/venues/some-venue', 'venues'],
    ['https://psychichomily.com/labels/some-label', 'labels'],
    ['https://psychichomily.com/releases/some-release', 'releases'],
    ['https://psychichomily.com/festivals/some-fest', 'festivals'],
    ['https://psychichomily.com/tags/punk', 'tags'],
  ])('%s → %s', (loc, expected) => {
    expect(classifyLoc(loc)).toBe(expected)
  })

  // The two /scenes families are split by segment count, not prefix.
  it('separates a scene from a scene week', () => {
    expect(classifyLoc('https://psychichomily.com/scenes/austin-tx')).toBe('scenes')
    expect(classifyLoc('https://psychichomily.com/scenes/austin-tx/2026-W28')).toBe(
      'scene_weeks'
    )
  })

  // The two /venues families split the same way (PSY-1756), but the archive
  // shape is matched exactly rather than by segment count alone: /venues has
  // room for other child routes, and one of them must not be counted as an
  // archive the generator never emitted.
  it('separates a venue from a venue year archive', () => {
    expect(classifyLoc('https://psychichomily.com/venues/the-van-buren')).toBe(
      'venues'
    )
    expect(
      classifyLoc('https://psychichomily.com/venues/the-van-buren/shows/2025')
    ).toBe('venue_years')
  })

  it.each([
    // Right depth, wrong middle segment.
    'https://psychichomily.com/venues/the-van-buren/events/2025',
    // Right shape, tail is not a year.
    'https://psychichomily.com/venues/the-van-buren/shows/last-year',
    // The archive index, which the generator does not emit.
    'https://psychichomily.com/venues/the-van-buren/shows',
  ])('%s → other', loc => {
    expect(classifyLoc(loc)).toBe('other')
  })

  it.each([
    'https://psychichomily.com',
    'https://psychichomily.com/',
    'https://psychichomily.com/shows',
    'https://psychichomily.com/blog/a-post',
    'https://psychichomily.com/dj-sets/a-mix',
  ])('%s → pages', loc => {
    expect(classifyLoc(loc)).toBe('pages')
  })

  it('buckets an unrecognised URL as other rather than silently as pages', () => {
    expect(classifyLoc('https://psychichomily.com/mystery/thing')).toBe('other')
  })

  it('does not throw on a malformed URL', () => {
    expect(classifyLoc('not a url')).toBe('other')
  })

  /**
   * The guard that matters, and the reason the prefix table is shared rather
   * than restated here: the sample URLs are BUILT from FAMILY_URL_PREFIXES, so
   * renaming a prefix in sitemap-shards.ts moves the generator and this
   * classifier together. A hand-written sample map would keep passing while
   * `classifyLoc` silently bucketed the renamed family as `other`.
   */
  it('classifies every family from the shared prefix table', () => {
    // The composite-slug families, whose slug is itself a path tail.
    const compositeSlugs: Partial<Record<(typeof SITEMAP_FAMILIES)[number], string>> = {
      scene_weeks: 'austin-tx/2026-W28',
      venue_years: 'the-van-buren/shows/2025',
    }
    for (const family of SITEMAP_FAMILIES) {
      const slug = compositeSlugs[family] ?? 'a-slug'
      const loc = `https://psychichomily.com${FAMILY_URL_PREFIXES[family]}/${slug}`
      expect(classifyLoc(loc), `misclassified ${family} (${loc})`).toBe(family)
    }
  })

  /**
   * `classifyLoc` can only split families that share a prefix if it has an
   * explicit rule for each CLAIMANT of that prefix. Asserting the claimants
   * rather than the prefix list is what keeps this a live guard: once `/venues`
   * is shared, a prefix-only assertion stops changing when a THIRD family joins
   * it, and that family would classify as `other` with every test still green.
   */
  it('has a disambiguation rule for every family under a shared prefix', () => {
    expect(SHARED_CLAIMANTS).toEqual({
      venues: ['venue_years', 'venues'],
      scenes: ['scene_weeks', 'scenes'],
    })
  })
})

describe('showDateFromLoc', () => {
  it('reads the event date out of the slug', () => {
    expect(
      showDateFromLoc('https://psychichomily.com/shows/2026-03-20-lonna-kelley-at-rv-phone-home')
    ).toBe('2026-03-20')
  })

  // Measured: 10 of 1458 stage show slugs carry no date prefix.
  it('returns undefined for a show slug with no date prefix', () => {
    expect(showDateFromLoc('https://psychichomily.com/shows/some-untimed-show')).toBeUndefined()
  })

  it('ignores dates in non-show URLs', () => {
    expect(showDateFromLoc('https://psychichomily.com/artists/2026-03-20-band')).toBeUndefined()
  })
})
