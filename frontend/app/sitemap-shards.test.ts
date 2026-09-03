import { describe, expect, it } from 'vitest'
import {
  ALL_SHARD_IDS,
  ARTIST_SHARD_IDS,
  ENTITY_SHARD_IDS,
  FAMILY_URL_PREFIXES,
  PAGES_SHARD_ID,
  RELEASE_SHARD_IDS,
  shardFamily,
  shardIdsFor,
  SHOW_SHARD_IDS,
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
    // Not vacuous: `SUB_SHARDS[family]?.ids ?? [family]` falls back only on
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
 * The sub-sharded families, and the id list each is served by.
 *
 * Table-driven so a newly bucketed family inherits every assertion by being
 * added here, rather than growing a fourth near-duplicate describe block.
 */
const SUB_SHARDED: ReadonlyArray<[Family, readonly string[]]> = [
  ['shows', SHOW_SHARD_IDS],
  ['artists', ARTIST_SHARD_IDS],
  ['releases', RELEASE_SHARD_IDS],
]

describe.each(SUB_SHARDED)('the %s sub-shards', (family, ids) => {
  it('are the shards that serve the family, and the only ones', () => {
    expect(ENTITY_SHARD_IDS.filter(id => shardFamily(id) === family)).toEqual([...ids])
  })

  it('are what shardIdsFor answers for the family', () => {
    // Every consumer reaches the ids through this function, so a list exported
    // but not wired into SUB_SHARDS would leave the family served by one
    // over-cap document while the list looked right.
    expect([...shardIdsFor(family)]).toEqual([...ids])
  })

  it('keep the family out of the shard list under its bare name', () => {
    // A bare `shows` shard alongside the buckets would announce every URL of
    // the family twice and re-introduce the over-cap fetch the buckets exist
    // to avoid.
    expect(ENTITY_SHARD_IDS).not.toContain(family)
  })

  /**
   * A bucket holds the rows whose primary key is its number modulo the list
   * length, so the ids must be `b0` through `b{n-1}` with nothing skipped and
   * nothing repeated. A gap here is a residue nobody fetches: those rows leave
   * the sitemap while every document still renders, which is the failure no
   * byte-size measurement can show.
   */
  it('is every residue of one modulus, in order, starting at b0', () => {
    const expected = ids.map((_, bucket) => `${family}-b${bucket}`)

    expect([...ids]).toEqual(expected)
  })

  it('leaves more than one shard, or sub-sharding bought nothing', () => {
    expect(ids.length).toBeGreaterThan(1)
  })

  it('emit URLs under the family prefix, not a per-shard one', () => {
    // Sub-sharding is a transport detail. If the bucket reached the `<loc>`
    // values it would change the site's URLs, which is the one thing it must
    // not do: a show stays at /shows/{slug} whichever bucket serves it.
    for (const id of ids) {
      expect(FAMILY_URL_PREFIXES[shardFamily(id) as Family]).toBe(
        FAMILY_URL_PREFIXES[family]
      )
    }
  })
})

describe('shardIdsFor', () => {
  it('answers a single-document family with its own id', () => {
    expect([...shardIdsFor('venues')]).toEqual(['venues'])
  })
})
