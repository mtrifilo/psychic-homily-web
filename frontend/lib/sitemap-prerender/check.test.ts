import { describe, expect, it } from 'vitest'
import {
  ALL_SHARD_IDS,
  FAMILY_SHARD_IDS,
  PAGES_SHARD_ID,
  shardRoutePath,
} from '@/app/sitemap-shards'
import {
  findShardsWithoutFallback,
  formatPendingShards,
  formatShardFailures,
  looksLikeManifestShapeChange,
  partitionShardFailures,
  shardBodyPath,
  shardIdFromRoute,
  type FamilyVerdict,
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

/**
 * The deploy-order excuse (PSY-1756).
 *
 * A family lands in the frontend and the backend in one PR but deploys on two
 * pipelines, so for one window Vercel builds against an API that has never
 * heard of it. Measured on this ticket's own preview builds: `1 of 11 shards`
 * failed while the other ten prerendered from the same healthy backend.
 *
 * The excuse must be narrow enough that a real outage still fails the build,
 * which is what most of these cases pin.
 */
describe('partitionShardFailures', () => {
  const failureFor = (id: string) => ({
    route: shardRoutePath(id),
    reason: 'no prerender-manifest entry — the route fell back to Dynamic',
  })

  const probeReturning =
    (verdicts: Record<string, FamilyVerdict>) =>
    async (shardId: string): Promise<FamilyVerdict> =>
      verdicts[shardId] ?? 'served'

  it('excuses a family the backend says it does not serve', async () => {
    const { blocking, pending } = await partitionShardFailures(
      [failureFor('venue_years')],
      probeReturning({ venue_years: 'unknown' })
    )

    expect(pending.map(f => f.route)).toEqual([shardRoutePath('venue_years')])
    expect(blocking).toEqual([])
  })

  /**
   * The case the gate exists for. A backend that is down cannot say "I do not
   * serve shows", so every family it fails to answer stays blocking.
   */
  it('blocks every shard when the backend is unreachable', async () => {
    const failures = FAMILY_SHARD_IDS.map(failureFor)

    const { blocking, pending } = await partitionShardFailures(failures, async () => 'unreachable')

    expect(blocking).toHaveLength(FAMILY_SHARD_IDS.length)
    expect(pending).toEqual([])
  })

  it('blocks a family the backend DOES serve — that is a real prerender failure', async () => {
    const { blocking, pending } = await partitionShardFailures(
      [failureFor('shows')],
      probeReturning({ shows: 'served' })
    )

    expect(blocking.map(f => f.route)).toEqual([shardRoutePath('shows')])
    expect(pending).toEqual([])
  })

  /** The pages shard makes no network call, so the backend cannot explain it. */
  it('never excuses the pages shard, whatever the probe says', async () => {
    const { blocking, pending } = await partitionShardFailures(
      [failureFor(PAGES_SHARD_ID)],
      async () => 'unknown'
    )

    expect(blocking.map(f => f.route)).toEqual([shardRoutePath(PAGES_SHARD_ID)])
    expect(pending).toEqual([])
    })

  it('never excuses a route it cannot map back to a shard', async () => {
    const { blocking, pending } = await partitionShardFailures(
      [{ route: '/sitemap/mystery.xml', reason: 'whatever' }],
      async () => 'unknown'
    )

    expect(blocking).toHaveLength(1)
    expect(pending).toEqual([])
  })

  it('splits a mixed build into both buckets', async () => {
    const { blocking, pending } = await partitionShardFailures(
      [failureFor('venue_years'), failureFor('shows')],
      probeReturning({ venue_years: 'unknown', shows: 'unreachable' })
    )

    expect(pending.map(f => f.route)).toEqual([shardRoutePath('venue_years')])
    expect(blocking.map(f => f.route)).toEqual([shardRoutePath('shows')])
  })

  it('passes a build with no failures without probing at all', async () => {
    let probed = 0
    const { blocking, pending } = await partitionShardFailures([], async () => {
      probed += 1
      return 'unknown'
    })

    expect(blocking).toEqual([])
    expect(pending).toEqual([])
    expect(probed).toBe(0)
  })
})

describe('shardIdFromRoute', () => {
  it.each(ALL_SHARD_IDS)('maps %s back from its route path', id => {
    expect(shardIdFromRoute(shardRoutePath(id))).toBe(id)
  })

  it('returns null for a path that is not a shard route', () => {
    expect(shardIdFromRoute('/sitemap/mystery.xml')).toBeNull()
    expect(shardIdFromRoute('/venues/a-venue')).toBeNull()
  })
})

describe('formatPendingShards', () => {
  it('says nothing when nothing is pending', () => {
    expect(formatPendingShards([])).toBe('')
  })

  it('names the shard and what to check if it never clears', () => {
    const message = formatPendingShards([
      { route: shardRoutePath('venue_years'), reason: 'whatever' },
    ])

    expect(message).toContain('/sitemap/venue_years.xml')
    expect(message).toContain('backend does not serve that family yet')
    // The reader has to be told this is temporary AND how to tell when it is not.
    expect(message).toContain('FAMILY_SHARD_IDS')
  })
})
