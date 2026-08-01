import { describe, expect, it } from 'vitest'
import { ALL_SHARD_IDS, FAMILY_SHARD_IDS, PAGES_SHARD_ID } from '@/app/sitemap-shards'
import {
  findShardsWithoutFallback,
  formatShardFailures,
  looksLikeManifestShapeChange,
  shardBodyPath,
  shardRoutePath,
  type PrerenderManifestLike,
} from './check'

/** A manifest in which every listed shard prerendered as STATIC. */
function manifestWith(ids: readonly string[]): PrerenderManifestLike {
  return {
    routes: Object.fromEntries(
      ids.map(id => [shardRoutePath(id), { renderingMode: 'STATIC' }])
    ),
  }
}

const allBodiesPresent = () => true

describe('findShardsWithoutFallback', () => {
  it('passes a build where every shard prerendered with a body', () => {
    expect(
      findShardsWithoutFallback(manifestWith(ALL_SHARD_IDS), allBodiesPresent)
    ).toEqual([])
  })

  it('reports every entity family when only the pages shard prerendered', () => {
    // The measured degraded build: backend unreachable, clean build cache. The
    // pages shard makes no network call, so it survives while all nine entity
    // families fall to Dynamic.
    const failures = findShardsWithoutFallback(
      manifestWith([PAGES_SHARD_ID]),
      allBodiesPresent
    )

    expect(failures.map(f => f.route)).toEqual(FAMILY_SHARD_IDS.map(shardRoutePath))
    expect(failures[0].reason).toContain('Dynamic')
  })

  it('rejects a manifest entry that is present but not STATIC', () => {
    // A manifest key alone is not a fallback: a non-STATIC mode is still
    // rendered per request, so it 500s during the same outage.
    const manifest = manifestWith(ALL_SHARD_IDS)
    manifest.routes![shardRoutePath('artists')] = { renderingMode: 'PARTIALLY_STATIC' }

    const failures = findShardsWithoutFallback(manifest, allBodiesPresent)

    expect(failures).toEqual([
      {
        route: shardRoutePath('artists'),
        reason: 'renderingMode is "PARTIALLY_STATIC", expected "STATIC"',
      },
    ])
  })

  it('rejects a shard whose manifest entry has no rendered body on disk', () => {
    const failures = findShardsWithoutFallback(
      manifestWith(ALL_SHARD_IDS),
      bodyPath => bodyPath !== shardBodyPath('artists')
    )

    expect(failures).toHaveLength(1)
    expect(failures[0].route).toBe(shardRoutePath('artists'))
    expect(failures[0].reason).toContain(shardBodyPath('artists'))
  })

  it('treats an empty file as no body — a zero-byte document is not a fallback', () => {
    const failures = findShardsWithoutFallback(manifestWith(ALL_SHARD_IDS), () => false)
    expect(failures).toHaveLength(ALL_SHARD_IDS.length)
  })

  it('treats a manifest with no routes at all as every shard failing', () => {
    expect(findShardsWithoutFallback({}, allBodiesPresent)).toHaveLength(
      ALL_SHARD_IDS.length
    )
    expect(findShardsWithoutFallback({ routes: null }, allBodiesPresent)).toHaveLength(
      ALL_SHARD_IDS.length
    )
  })

  it('does not require a URL count — an empty family is a legitimate shard', () => {
    // Existence is the assertion. fetchSitemapFamily throws rather than
    // emitting a partial document, so a body at all proves the fetch succeeded,
    // while a threshold would fail the build on a real empty catalogue.
    expect(
      findShardsWithoutFallback(manifestWith(ALL_SHARD_IDS), allBodiesPresent)
    ).toEqual([])
  })
})

describe('looksLikeManifestShapeChange', () => {
  it('is true when other routes prerendered but no shard route did', () => {
    const manifest: PrerenderManifestLike = {
      routes: { '/robots.txt': { renderingMode: 'STATIC' } },
    }
    const failures = findShardsWithoutFallback(manifest, allBodiesPresent)

    expect(looksLikeManifestShapeChange(manifest, failures)).toBe(true)
  })

  it('is false for the degraded build — the pages shard survives an outage', () => {
    const manifest = manifestWith([PAGES_SHARD_ID])
    const failures = findShardsWithoutFallback(manifest, allBodiesPresent)

    expect(looksLikeManifestShapeChange(manifest, failures)).toBe(false)
  })

  it('is false for an empty manifest — that is a broken build, not a rename', () => {
    expect(looksLikeManifestShapeChange({ routes: {} }, [])).toBe(false)
  })
})

describe('ALL_SHARD_IDS', () => {
  it('covers the pages shard plus every family generateSitemaps() emits', () => {
    // Derived from sitemap-shards.ts on purpose: a family added there must be
    // covered by this gate without anyone remembering to update it.
    expect(ALL_SHARD_IDS).toEqual([PAGES_SHARD_ID, ...FAMILY_SHARD_IDS])
  })

  it('maps ids onto the served route paths and build artifacts', () => {
    expect(shardRoutePath('artists')).toBe('/sitemap/artists.xml')
    expect(shardBodyPath('artists')).toBe('server/app/sitemap/artists.xml.body')
  })
})

describe('formatShardFailures', () => {
  it('is empty when nothing failed, so the caller can use it as the gate', () => {
    expect(formatShardFailures([], manifestWith(ALL_SHARD_IDS))).toBe('')
  })

  it('names each failing route and how to build without a backend on purpose', () => {
    const manifest = manifestWith([PAGES_SHARD_ID])
    const message = formatShardFailures(
      findShardsWithoutFallback(manifest, allBodiesPresent),
      manifest
    )

    expect(message).toContain(shardRoutePath('artists'))
    expect(message).toContain('GET /sitemap/entries')
    expect(message).toContain('node_modules/.bin/next build')
  })

  it('blames the manifest shape, not the backend, when no shard route matched', () => {
    const manifest: PrerenderManifestLike = {
      routes: { '/robots.txt': { renderingMode: 'STATIC' } },
    }
    const message = formatShardFailures(
      findShardsWithoutFallback(manifest, allBodiesPresent),
      manifest
    )

    expect(message).toContain('Next.js upgrade')
    expect(message).not.toContain('GET /sitemap/entries')
  })
})
