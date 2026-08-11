import { describe, expect, it } from 'vitest'
import {
  ALL_SHARD_IDS,
  ENTITY_SHARD_IDS,
  FAMILY_URL_PREFIXES,
  PAGES_SHARD_ID,
  RELEASE_SHARD_IDS,
  shardFamily,
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
 * The releases family is the only sub-sharded one (PSY-1763). These pin the
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
