import type { components, operations } from '@/types/api'

type SitemapEntries = components['schemas']['SitemapEntries']

/** The entity families, minus the `$schema` key Huma adds to every response. */
export type Family = Exclude<keyof SitemapEntries, '$schema'>

/**
 * Every value the backend accepts as `?family=` on GET /sitemap/entries.
 *
 * A superset of `Family`: it also carries the sub-shard ids, which address a
 * SLICE of a family and so are not keys of the response schema. Taken from the
 * generated operation type rather than restated, which is what lets the shard
 * ids below be checked at compile time the way the family names already are.
 */
type WireFamily = NonNullable<
  NonNullable<operations['get-sitemap-entries']['parameters']['query']>['family']
>

/** Non-entity shard: static pages + local MDX (blog / DJ sets). */
export const PAGES_SHARD_ID = 'pages'

/**
 * The entity families the sitemap covers.
 *
 * FAMILIES, not shard ids. The two are not the same thing, because `shows`,
 * `artists` and `releases` are each served by eight documents (SHOW_SHARD_IDS,
 * ARTIST_SHARD_IDS and RELEASE_SHARD_IDS). Anything counting or
 * classifying URLs wants this list; anything enumerating DOCUMENTS wants
 * ENTITY_SHARD_IDS below. Keep in sync with the Huma `family` enum on
 * GET /sitemap/entries and with `sitemapFamilies` in
 * backend/internal/services/catalog/sitemap.go.
 *
 * There is deliberately no `scene_days` family. Day permalinks stay reachable
 * through the prev/next chips on each day page; they are simply not announced
 * here, and there is one for every date the day service will answer.
 * `buildSceneDayMetadata` leans on that absence to canonicalize the rolling
 * /tonight page at the week permalink instead of a day, so adding a day family
 * here obliges revisiting that canonical. This is the normative statement of
 * that decision; the other sites referring to it point back at this note.
 */
export const SITEMAP_FAMILIES = [
  'shows',
  'artists',
  'venues',
  'venue_years',
  'scenes',
  'scene_weeks',
  'labels',
  'releases',
  'festivals',
  'tags',
] as const satisfies readonly Family[]

/** Compile-time guard: every schema family must appear in SITEMAP_FAMILIES. */
type MissingFamily = Exclude<Family, (typeof SITEMAP_FAMILIES)[number]>
type AssertNoMissingFamily = [MissingFamily] extends [never]
  ? true
  : { missing: MissingFamily }
const _assertNoMissingFamily: AssertNoMissingFamily = true
void _assertNoMissingFamily

/**
 * The buckets each over-cap family is served in.
 *
 * WHY BUCKETS, what the modulus is chosen against, and what re-tuning one
 * costs: all of that lives with the predicate that enforces it, in
 * `sitemapShard` and `sitemapShardsPerFamily` in
 * backend/internal/services/catalog/sitemap.go. That is the normative
 * statement; these notes carry only what is frontend-specific and point there
 * for the rest, so re-tuning is one edit rather than three.
 *
 * A bucket holds the rows whose primary key is its number modulo eight, so the
 * lists are contiguous from `b0` and their length IS the modulus. That is
 * asserted against the backend in two directions: `satisfies readonly
 * WireFamily[]` rejects an id the backend does not accept, and
 * AssertEveryWireValueServed below rejects an id the backend accepts that no
 * list here fetches.
 *
 * What a build actually wrote, read out of `.next/cache/fetch-cache` against a
 * database holding the production catalogue. These are the entry sizes Next's
 * own cap is applied to, not the raw body scaled by the base64 ratio:
 *
 *   releases-b0 .. b7   358,467 to 369,887   17.1% to 17.6% of the cap
 *   shows-b0 .. b7      216,704 to 234,652   10.3% to 11.2%
 *   artists-b0 .. b7    162,454 to 169,714    7.8% to  8.1%
 *
 * against 129.8% of the cap for `releases` as one document, 80.6% for `shows`
 * and 58.4% for `artists`. The spread inside a family is under half a point,
 * which is the balance the key buys: bucket membership is independent of what
 * a row contains, so the shares hold as the catalogue grows.
 *
 * Sub-shards here are never legitimately empty while their family holds rows:
 * residues of a serial key are equidistributed, so an empty bucket beside
 * populated siblings is a lost document. The monitor asserts that per shard
 * against the API's own per-shard counts rather than inferring it from the
 * siblings.
 */
export const SHOW_SHARD_IDS = [
  'shows-b0',
  'shows-b1',
  'shows-b2',
  'shows-b3',
  'shows-b4',
  'shows-b5',
  'shows-b6',
  'shows-b7',
] as const satisfies readonly WireFamily[]

/** The buckets the `artists` family is served in. See SHOW_SHARD_IDS. */
export const ARTIST_SHARD_IDS = [
  'artists-b0',
  'artists-b1',
  'artists-b2',
  'artists-b3',
  'artists-b4',
  'artists-b5',
  'artists-b6',
  'artists-b7',
] as const satisfies readonly WireFamily[]

/** The buckets the `releases` family is served in. See SHOW_SHARD_IDS. */
export const RELEASE_SHARD_IDS = [
  'releases-b0',
  'releases-b1',
  'releases-b2',
  'releases-b3',
  'releases-b4',
  'releases-b5',
  'releases-b6',
  'releases-b7',
] as const satisfies readonly WireFamily[]

/**
 * Compile-time guard: every value the backend accepts must be SERVED by some
 * shard here.
 *
 * The `satisfies` above and this are not the same check, and only having the
 * first is a trap. `satisfies` is element-wise: it catches an id RENAMED or
 * REMOVED on the backend, because the stale literal stops being assignable.
 * It cannot catch an id ADDED, because a shorter list is still a list of valid
 * values, which is exactly what doubling a family's bucket count produces.
 * Without this second half, a backend bucket the frontend never learned is
 * simply never fetched: the build is green, `tsc` is green, the Go enum test is
 * green, and those rows leave the sitemap with the loss sitting inside the
 * monitor's per-family drift tolerance.
 *
 * `SITEMAP_FAMILIES` has carried the same pair since PSY-1622 (`MissingFamily`
 * above is its addition-guard); this is the sub-shard's.
 *
 * Written over the whole wire vocabulary rather than one family's prefix, so a
 * newly bucketed family is covered by adding its list to the Exclude.
 */
type UnservedWireFamily = Exclude<
  WireFamily,
  | (typeof SITEMAP_FAMILIES)[number]
  | (typeof SHOW_SHARD_IDS)[number]
  | (typeof ARTIST_SHARD_IDS)[number]
  | (typeof RELEASE_SHARD_IDS)[number]
>
type AssertEveryWireValueServed = [UnservedWireFamily] extends [never]
  ? true
  : { unserved: UnservedWireFamily }
const _assertEveryWireValueServed: AssertEveryWireValueServed = true
void _assertEveryWireValueServed

/**
 * Families served by more than one document, and the ids those documents use.
 *
 * A family absent from this table is served by a single shard whose id IS the
 * family name, which is the case for seven of the ten.
 *
 * Typed `readonly WireFamily[]`, not `readonly string[]`: an id written here
 * that the backend does not accept would be fetched, 422'd, and degraded to an
 * empty document, which is a shard silently serving nothing. Typing it against
 * the generated enum makes that a compile error instead.
 */
const SUB_SHARDS: Partial<Record<Family, readonly WireFamily[]>> = {
  shows: SHOW_SHARD_IDS,
  artists: ARTIST_SHARD_IDS,
  releases: RELEASE_SHARD_IDS,
}

/** The documents a family is served by, which is its own id when unsharded. */
export function shardIdsFor(family: Family): readonly string[] {
  return SUB_SHARDS[family] ?? [family]
}

/**
 * Which family each entity shard id carries rows for.
 *
 * Built by walking SITEMAP_FAMILIES, so ENTITY_SHARD_IDS below is literally the
 * key set of this map: a shard cannot be enumerated without a family, and a
 * family cannot be sub-sharded without its ids being enumerated. That is the
 * single source of truth the index route, the generator, the freshness monitor
 * and the post-build prerender gate all read.
 */
const FAMILY_BY_SHARD_ID = new Map<string, Family>()
for (const family of SITEMAP_FAMILIES) {
  for (const id of shardIdsFor(family)) {
    // THROW rather than overwrite. `Map.set` would silently keep the last
    // writer, so two families claiming one id would yield a table that looks
    // consistent, an ENTITY_SHARD_IDS one entry short, and a family whose URLs
    // are attributed to the other one. A test cannot catch it after the fact —
    // by then the collision has already collapsed — so it is enforced here,
    // where every consumer of this module (the generator, the index route, the
    // monitor, the build gate) trips over it at import.
    const claimed = FAMILY_BY_SHARD_ID.get(id)
    if (claimed) {
      throw new Error(
        `sitemap shard id "${id}" is claimed by both "${claimed}" and "${family}"`
      )
    }
    FAMILY_BY_SHARD_ID.set(id, family)
  }
}

/** The family a shard id carries, or undefined if it names no known shard. */
export function shardFamily(id: string): Family | undefined {
  return FAMILY_BY_SHARD_ID.get(id)
}

/** Every entity shard, in family order. */
export const ENTITY_SHARD_IDS: readonly string[] = [...FAMILY_BY_SHARD_ID.keys()]

/**
 * Every shard `generateSitemaps()` emits, in route-table order.
 *
 * Named here rather than composed inline at each site: four modules need the
 * same list — the generator, the index route, the freshness monitor, and the
 * post-build prerender gate — and an inline `[PAGES_SHARD_ID, ...]` in each is
 * a fork waiting to happen the day a non-family shard is added.
 */
export const ALL_SHARD_IDS: readonly string[] = [
  PAGES_SHARD_ID,
  ...ENTITY_SHARD_IDS,
]

/**
 * The path Next serves shard `id` at — and, on a prerendered build, its key in
 * `.next/prerender-manifest.json`.
 *
 * Owned here for the same reason as FAMILY_URL_PREFIXES below: the compile-time
 * guards catch a shard being ADDED, but only a shared table catches the served
 * path SHAPE being changed. The index route builds these URLs, the monitor
 * parses them back apart, and the build gate looks them up in the manifest.
 */
export function shardRoutePath(id: string): string {
  return `/sitemap/${id}.xml`
}

/**
 * The URL path prefix each family's entries live under.
 *
 * Owned here, next to the family ids, because two independent consumers need
 * the same fact: app/sitemap.ts builds `<loc>` values FROM it, and
 * lib/sitemap-monitor classifies served URLs BACK INTO families with it. When
 * each kept its own copy, renaming a prefix in one still compiled and still
 * passed every test, while the monitor silently bucketed the whole family as
 * unrecognised and alarmed forever after. The compile-time `Record<Family, …>`
 * guards catch a family being ADDED; only a shared table catches a prefix
 * being CHANGED.
 *
 * Keyed by FAMILY, not by shard id: every sub-shard of a family emits URLs
 * under that family's one prefix, which is what keeps sub-sharding invisible to
 * the URLs themselves.
 *
 * TWO prefixes are shared, and both for the same reason — a family addressing a
 * SLICE of an entity lives under that entity's prefix:
 *
 *   - `scenes` / `scene_weeks` share `/scenes`: `/scenes/{city}` vs
 *     `/scenes/{city}/{iso-week}`.
 *   - `venues` / `venue_years` share `/venues`: `/venues/{slug}` vs
 *     `/venues/{slug}/shows/{year}` (PSY-1756).
 *
 * Anything mapping a URL back to a family has to disambiguate each pair by
 * segment count, not by prefix — see `classifyLoc` in lib/sitemap-monitor/parse,
 * whose SHARED_CLAIMANTS guard fails the day any family joins a shared prefix
 * without a rule.
 */
export const FAMILY_URL_PREFIXES = {
  shows: '/shows',
  artists: '/artists',
  venues: '/venues',
  venue_years: '/venues',
  scenes: '/scenes',
  scene_weeks: '/scenes',
  labels: '/labels',
  releases: '/releases',
  festivals: '/festivals',
  tags: '/tags',
} as const satisfies Record<Family, string>
