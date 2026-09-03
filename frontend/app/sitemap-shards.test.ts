import { describe, expect, it } from 'vitest'
import {
  ALL_SHARD_IDS,
  ENTITY_SHARD_IDS,
  FAMILY_URL_PREFIXES,
  PAGES_SHARD_ID,
  RELEASE_SHARD_IDS,
  shardFamily,
  SHOW_SHARD_IDS,
  SPARSE_SUB_SHARD_FAMILIES,
  shardRoutePath,
  SITEMAP_FAMILIES,
  type Family,
} from './sitemap-shards'

/**
 * The shard table is what four independent modules read to agree on which
 * documents exist: the generator, the /sitemap-index route, the freshness
 * monitor, and the post-build prerender gate. TypeScript catches a family
 * missing from SITEMAP_FAMILIES and a sub-shard id that is not a wire enum
 * value, and the table itself throws on two families claiming one id. What is
 * left for a test is the composition: a family enumerated and then served by no
 * shard, and the pages shard colliding with an entity one.
 */
describe('the shard table', () => {
  it('serves every family with at least one shard', () => {
    // Not vacuous: `SUB_SHARD_IDS[family] ?? [family]` falls back only on
    // undefined, so mapping a family to an EMPTY array would enumerate it here
    // and serve it nowhere — a whole family silently absent from the sitemap.
    const served = new Set(ENTITY_SHARD_IDS.map(id => shardFamily(id)))
    const unserved = SITEMAP_FAMILIES.filter(family => !served.has(family))

    expect(unserved).toEqual([])
  })

  it('has no duplicate ids once the pages shard is included', () => {
    // The table's own collision check covers two FAMILIES claiming one id. It
    // cannot cover the pages shard, which is not in that map — a sub-shard
    // named `pages` would appear twice here and be fetched twice.
    expect(new Set(ALL_SHARD_IDS).size).toBe(ALL_SHARD_IDS.length)
  })

  it('does not claim the pages shard for a family', () => {
    // The pages shard makes no backend call; a family mapping for it would send
    // the generator fetching a family that does not exist.
    expect(shardFamily(PAGES_SHARD_ID)).toBeUndefined()
    expect(ENTITY_SHARD_IDS).not.toContain(PAGES_SHARD_ID)
  })

  it('claims nothing it does not serve', () => {
    expect(shardFamily('releases-nope')).toBeUndefined()
    expect(shardFamily('')).toBeUndefined()
  })

  it('puts the pages shard first and every entity shard after it', () => {
    expect(ALL_SHARD_IDS).toEqual([PAGES_SHARD_ID, ...ENTITY_SHARD_IDS])
  })

  /**
   * Ids become path segments in `/sitemap/{id}.xml` and query values in
   * `?family={id}`. An id needing escaping in either would make the served path
   * and the requested family disagree with the strings this table hands out.
   */
  it('uses ids that survive a URL round trip unescaped', () => {
    for (const id of ALL_SHARD_IDS) {
      expect(encodeURIComponent(id)).toBe(id)
      expect(shardRoutePath(id)).toBe(`/sitemap/${id}.xml`)
    }
  })
})

/**
 * The shows family is sub-sharded by UTC event month (PSY-2018). These pin the
 * relationship rather than the specific span, so extending the enumerated years
 * is a table edit — while REMOVING the sub-sharding, or letting a month escape
 * into another family, still fails.
 */
describe('the shows sub-shards', () => {
  it('are the shards that serve the shows family, and the only ones', () => {
    const showShards = ENTITY_SHARD_IDS.filter(id => shardFamily(id) === 'shows')

    expect(showShards).toEqual([...SHOW_SHARD_IDS])
  })

  it('keep the family out of the shard list under its bare name', () => {
    // A bare `shows` shard alongside the months would announce every show URL
    // twice and re-introduce the over-cap fetch the months exist to avoid.
    expect(ENTITY_SHARD_IDS).not.toContain('shows')
  })

  it('emit URLs under the family prefix, not a per-shard one', () => {
    // Sub-sharding is a transport detail. If the event month reached the `<loc>`
    // values it would change the site's URLs, which is the one thing it must
    // not do — show URLs stay /shows/{slug} whichever month serves them.
    for (const id of SHOW_SHARD_IDS) {
      const family = shardFamily(id) as Family
      expect(FAMILY_URL_PREFIXES[family]).toBe('/shows')
    }
  })

  /**
   * The head and tail ids are what make the partition total: everything before
   * the enumerated span and everything after it has somewhere to go. A span
   * enumerated without them would drop every show outside it from the sitemap
   * with each remaining shard still rendering a valid document.
   */
  it('bracket the enumerated months with an open head and an open tail', () => {
    const months = SHOW_SHARD_IDS.filter(id => /^shows-\d{4}-\d{2}$/.test(id))
    expect(months.length).toBeGreaterThan(0)

    const open = SHOW_SHARD_IDS.filter(id => !months.includes(id))
    expect(open).toHaveLength(2)
    expect(open[0]).toMatch(/^shows-before-\d{4}$/)
    expect(open[1]).toMatch(/^shows-from-\d{4}$/)
    expect(SHOW_SHARD_IDS[0]).toBe(open[0])
    expect(SHOW_SHARD_IDS[SHOW_SHARD_IDS.length - 1]).toBe(open[1])
  })

  /**
   * A skipped month is the failure this list cannot show by reading: every id
   * would still look well formed, and the shows dated inside the gap would
   * belong to no shard at all. The backend asserts the same property against
   * its own table (TestShowShardYearsAreContiguous); this is the frontend half,
   * which is the side that decides which documents get fetched.
   */
  it('enumerates every month of every year in the span, in order', () => {
    const months = SHOW_SHARD_IDS.filter(id => /^shows-\d{4}-\d{2}$/.test(id))

    expect(months.length % 12).toBe(0)
    const expected = months.map((_, i) => {
      const first = Number(months[0].slice('shows-'.length, 'shows-'.length + 4))
      return `shows-${first + Math.floor(i / 12)}-${String((i % 12) + 1).padStart(2, '0')}`
    })
    expect(months).toEqual(expected)
  })

  /**
   * The open shards must meet the enumerated span exactly. `shows-before-2026`
   * next to a span starting in 2027 is a whole year served by nothing, and
   * `shows-from-2027` next to a span ending in 2027 is a year served twice.
   */
  it('anchors the open shards to the ends of the span', () => {
    const months = SHOW_SHARD_IDS.filter(id => /^shows-\d{4}-\d{2}$/.test(id))
    const firstYear = months[0].slice('shows-'.length, 'shows-'.length + 4)
    const lastYear = months[months.length - 1].slice('shows-'.length, 'shows-'.length + 4)

    expect(SHOW_SHARD_IDS[0]).toBe(`shows-before-${firstYear}`)
    expect(SHOW_SHARD_IDS[SHOW_SHARD_IDS.length - 1]).toBe(`shows-from-${Number(lastYear) + 1}`)
  })
})

/**
 * A family listed here is exempt from the monitor's empty-sibling check, so the
 * list has to stay a deliberate statement about how a family is keyed rather
 * than a place to silence an alarm.
 */
describe('the sparse sub-shard families', () => {
  it('names only families that are actually sub-sharded', () => {
    for (const family of SPARSE_SUB_SHARD_FAMILIES) {
      const shards = ENTITY_SHARD_IDS.filter(id => shardFamily(id) === family)
      expect(shards).not.toEqual([family])
      expect(shards.length).toBeGreaterThan(1)
    }
  })

  it('does not exempt the slug-keyed releases ranges', () => {
    // Every slug range of a non-empty catalogue holds rows, so an empty one
    // there IS a lost document and must keep alarming.
    expect(SPARSE_SUB_SHARD_FAMILIES.has('releases')).toBe(false)
  })
})

/**
 * The releases family is sub-sharded by slug range (PSY-1763). These pin the
 * relationship rather than the specific cut points, so re-tuning the ranges is
 * a one-line edit and adding a range is free — while REMOVING the sub-sharding,
 * or letting a range escape into another family, still fails.
 */
describe('the releases sub-shards', () => {
  it('are the shards that serve the releases family, and the only ones', () => {
    const releaseShards = ENTITY_SHARD_IDS.filter(
      id => shardFamily(id) === 'releases'
    )

    expect(releaseShards).toEqual([...RELEASE_SHARD_IDS])
  })

  it('keep the family out of the shard list under its bare name', () => {
    // A bare `releases` shard alongside the ranges would announce every release
    // URL twice and re-introduce the over-cap fetch the ranges exist to avoid.
    expect(ENTITY_SHARD_IDS).not.toContain('releases')
  })

  it('leaves more than one shard, or sub-sharding bought nothing', () => {
    expect(RELEASE_SHARD_IDS.length).toBeGreaterThan(1)
  })

  it('emit URLs under the family prefix, not a per-shard one', () => {
    // Sub-sharding is a transport detail. If it reached the `<loc>` values it
    // would change the site's URLs, which is the one thing it must not do.
    for (const id of RELEASE_SHARD_IDS) {
      const family = shardFamily(id) as Family
      expect(FAMILY_URL_PREFIXES[family]).toBe('/releases')
    }
  })
})
