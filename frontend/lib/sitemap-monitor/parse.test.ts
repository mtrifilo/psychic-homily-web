import { describe, expect, it } from 'vitest'
import { FAMILY_SHARD_IDS } from '@/app/sitemap-shards'
import {
  classifyLoc,
  detectShape,
  parseSitemapIndex,
  parseUrlset,
  showDateFromLoc,
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
  it('pairs each loc with its lastmod', () => {
    expect(parseUrlset(URLSET_XML)).toEqual([
      {
        loc: 'https://psychichomily.com/shows/2026-03-20-lonna-kelley-at-rv-phone-home',
        lastModified: '2026-03-20T02:17:34.909Z',
      },
      { loc: 'https://psychichomily.com/artists/no-lastmod-here', lastModified: undefined },
    ])
  })

  it('decodes XML entities in a loc', () => {
    const xml = `<urlset><url><loc>https://psychichomily.com/tags/rock&amp;roll</loc></url></urlset>`
    expect(parseUrlset(xml)[0].loc).toBe('https://psychichomily.com/tags/rock&roll')
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

  // The guard that matters: a family added to the sitemap must be countable
  // here too, or the single-document path would under-report it forever.
  it('classifies every family the sitemap shards by', () => {
    const samples: Record<string, string> = {
      shows: 'https://psychichomily.com/shows/2026-03-20-x',
      artists: 'https://psychichomily.com/artists/x',
      venues: 'https://psychichomily.com/venues/x',
      scenes: 'https://psychichomily.com/scenes/x',
      scene_weeks: 'https://psychichomily.com/scenes/x/2026-W28',
      labels: 'https://psychichomily.com/labels/x',
      releases: 'https://psychichomily.com/releases/x',
      festivals: 'https://psychichomily.com/festivals/x',
      tags: 'https://psychichomily.com/tags/x',
    }
    for (const family of FAMILY_SHARD_IDS) {
      expect(samples[family], `no sample URL for family "${family}"`).toBeDefined()
      expect(classifyLoc(samples[family])).toBe(family)
    }
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
