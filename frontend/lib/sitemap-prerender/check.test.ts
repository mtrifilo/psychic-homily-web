import { describe, expect, it } from 'vitest'
import {
  ALL_SHARD_IDS,
  ENTITY_SHARD_IDS,
  RELEASE_SHARD_IDS,
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

/**
 * One entity shard to drive the single-shard cases, taken from the shared list
 * rather than named: every family is now served by ids the table owns, so a
 * literal here would go stale the next time a bucket count changes.
 */
const [SAMPLE_SHARD] = ENTITY_SHARD_IDS

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

  it('reports every entity shard when only the pages shard prerendered', () => {
    // The measured degraded build: backend unreachable, clean build cache. The
    // pages shard makes no network call, so it survives while every shard that
    // fetches a family, or one bucket of one, falls to Dynamic.
    const failures = findShardsWithoutFallback(
      manifestWith([PAGES_SHARD_ID]),
      allBodiesPresent
    )

    expect(failures.map(f => f.route)).toEqual(ENTITY_SHARD_IDS.map(shardRoutePath))
    expect(failures[0].reason).toContain('Dynamic')
  })

  it('rejects a manifest entry that is present but not STATIC', () => {
    // A manifest key alone is not a fallback: a non-STATIC mode is still
    // rendered per request, so it 500s during the same outage.
    const manifest = manifestWith(ALL_SHARD_IDS)
    manifest.routes![shardRoutePath(SAMPLE_SHARD)] = { renderingMode: 'PARTIALLY_STATIC' }

    const failures = findShardsWithoutFallback(manifest, allBodiesPresent)

    expect(failures).toEqual([
      {
        route: shardRoutePath(SAMPLE_SHARD),
        reason: 'renderingMode is "PARTIALLY_STATIC", expected "STATIC"',
      },
    ])
  })

  it('rejects a shard whose manifest entry has no rendered body on disk', () => {
    const failures = findShardsWithoutFallback(
      manifestWith(ALL_SHARD_IDS),
      bodyPath => bodyPath !== shardBodyPath(SAMPLE_SHARD)
    )

    expect(failures).toHaveLength(1)
    expect(failures[0].route).toBe(shardRoutePath(SAMPLE_SHARD))
    expect(failures[0].reason).toContain(shardBodyPath(SAMPLE_SHARD))
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
    // Existence is the assertion. fetchShard throws rather than
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
  // How ALL_SHARD_IDS is COMPOSED is asserted in app/sitemap-shards.test.ts,
  // which owns that table. What this gate owns is the mapping from an id to the
  // two build artifacts it looks up, so that is what is pinned here.
  it('maps ids onto the served route paths and build artifacts', () => {
    expect(shardRoutePath(SAMPLE_SHARD)).toBe(`/sitemap/${SAMPLE_SHARD}.xml`)
    expect(shardBodyPath(SAMPLE_SHARD)).toBe(`server/app/sitemap/${SAMPLE_SHARD}.xml.body`)
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

    expect(message).toContain(shardRoutePath(SAMPLE_SHARD))
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
    const failures = ENTITY_SHARD_IDS.map(failureFor)

    const { blocking, pending } = await partitionShardFailures(failures, async () => 'unreachable')

    expect(blocking).toHaveLength(ENTITY_SHARD_IDS.length)
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
    expect(message).toContain('SITEMAP_FAMILIES')
  })

  /**
   * The two pending cases do NOT cost the same, and saying so is the whole
   * point of the message. A new family's empty document is true; a new slug
   * range of a family the backend already serves is a live family going partly
   * unannounced. Getting these the wrong way round tells an operator to relax
   * during the one window where they should not.
   */
  it('says nothing is missing when only whole new families are pending', () => {
    const message = formatPendingShards([
      { route: shardRoutePath('venue_years'), reason: 'whatever' },
    ])

    expect(message).toContain('no known URL is missing')
    expect(message).not.toContain('ALREADY serves rows')
  })

  it('names the live family whose URLs are unannounced when a bucket is pending', () => {
    const message = formatPendingShards(
      RELEASE_SHARD_IDS.map(id => ({ route: shardRoutePath(id), reason: 'whatever' }))
    )

    expect(message).toContain('ALREADY serves rows')
    expect(message).toContain('"releases"')
    expect(message).not.toContain('no known URL is missing')
  })

  /**
   * A pending shard shipped Dynamic, so it has no prerendered body and no ISR
   * timer. Promising hourly self-healing would be wrong, and it is the kind of
   * wrong that stops someone re-running the build.
   */
  it('does not promise an ISR window for a Dynamic shard', () => {
    const message = formatPendingShards(
      RELEASE_SHARD_IDS.map(id => ({ route: shardRoutePath(id), reason: 'whatever' }))
    )

    expect(message).toContain('no ISR timer')
    expect(message).not.toMatch(/within the hour/i)
  })
})
