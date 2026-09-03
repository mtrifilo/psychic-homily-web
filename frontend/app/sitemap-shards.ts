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
 * FAMILIES, not shard ids — the two stopped being the same thing when
 * `releases` and `shows` were sub-sharded (see RELEASE_SHARD_IDS and
 * SHOW_SHARD_IDS). Anything counting or
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
 * The UTC event months the `shows` family is served in (PSY-2018).
 *
 * WHY MONTHS, why the span runs to the end of the year after next, and what
 * extending it costs: all of that lives with the predicate that enforces it, in
 * `showShards` in backend/internal/services/catalog/sitemap.go. That is the
 * normative statement; this note carries only what is frontend-specific and
 * points there for the rest, the way RELEASE_SHARD_IDS below does.
 *
 * What a build actually wrote, read out of `.next/cache/fetch-cache` against
 * the production show catalogue — these are the entry sizes Next's own cap is
 * applied to, not the raw body scaled by the base64 ratio:
 *
 *   shows-2026-09  508,268  24.2% of the cap
 *   shows-2026-10  504,636  24.1%
 *   shows-2026-11  300,744  14.3%
 *   shows-2026-08  278,752  13.3%
 *
 * against 80.6% for the family as one document, which is what failed the
 * production build. Every other shows shard is under 4%. `artists`, at 918,744
 * raw bytes and so 58.4% of the cap, is now the largest sitemap entry — the
 * shard family to watch next, and a whole family rather than a range.
 *
 * SHARDS HERE ARE LEGITIMATELY EMPTY, which slug ranges never are. See
 * SPARSE_SUB_SHARD_FAMILIES below.
 */
export const SHOW_SHARD_IDS = [
  'shows-before-2026',
  'shows-2026-01',
  'shows-2026-02',
  'shows-2026-03',
  'shows-2026-04',
  'shows-2026-05',
  'shows-2026-06',
  'shows-2026-07',
  'shows-2026-08',
  'shows-2026-09',
  'shows-2026-10',
  'shows-2026-11',
  'shows-2026-12',
  'shows-2027-01',
  'shows-2027-02',
  'shows-2027-03',
  'shows-2027-04',
  'shows-2027-05',
  'shows-2027-06',
  'shows-2027-07',
  'shows-2027-08',
  'shows-2027-09',
  'shows-2027-10',
  'shows-2027-11',
  'shows-2027-12',
  'shows-from-2028',
] as const satisfies readonly WireFamily[]

/**
 * The slug ranges the `releases` family is served in (PSY-1763).
 *
 * WHY. Sharding per family (PSY-1622) exists to keep each fetch under Next's
 * Data Cache item cap of 2 MiB (2,097,152 bytes — MEBIbytes, per the unit note
 * on formatMiB in lib/data-cache-budget/budget.ts). Measured against production
 * on 2026-08-09, the releases family answered 1,530,206 raw bytes over 21,525
 * rows — 2,040,275 bytes once base64-encoded into a cache entry, i.e. 1.95 MiB
 * of the 2.00 MiB cap, 97.3%, with the next sizeable import crossing it. One
 * family no longer fits one entry.
 *
 * WHY A SLUG RANGE — and not a page number or a date range, what the balance
 * measures, and how to choose new cut points: all of that lives with the
 * predicate that enforces it, in `releaseShards` in
 * backend/internal/services/catalog/sitemap.go. That is the normative
 * statement; this note carries only what is frontend-specific and points there
 * for the rest, so re-tuning a cut point is one edit rather than three.
 *
 * What a build actually wrote, read out of `.next/cache/fetch-cache` against
 * the production release catalogue — these are the entry sizes Next's own cap
 * is applied to, not the raw body scaled by the base64 ratio:
 *
 *   releases-a-e  563,747  26.9% of the cap
 *   releases-f-m  562,227  26.8%
 *   releases-n-s  489,967  23.4%
 *   releases-t-z  428,751  20.4%
 *
 * against 97.3% for the family as one document. `artists`, at 40.8%, is now the
 * largest sitemap entry — so this is the shard family to watch next, and it is
 * a whole family rather than a range.
 *
 * HOW IT GROWS. Add a cut point and split ONE range. Every other range keeps
 * both its id and its exact contents, so re-tuning churns only the range being
 * split — the property page-numbering cannot offer at any shard count. When the
 * largest range approaches the warn band in
 * lib/data-cache-budget/budget.ts, split that one.
 *
 * Each id doubles as the `family` query value the backend accepts for that
 * range. Deliberately: a backend that predates a new id answers 422, which the
 * generator degrades to an empty shard for one deploy window
 * (UNKNOWN_FAMILY_STATUSES in app/sitemap.ts) and the prerender gate excuses. A
 * separate query parameter would be silently IGNORED by that same old backend,
 * which answers every sub-shard with the whole over-cap family instead.
 */
export const RELEASE_SHARD_IDS = [
  'releases-a-e',
  'releases-f-m',
  'releases-n-s',
  'releases-t-z',
] as const satisfies readonly WireFamily[]

/**
 * Compile-time guard: every value the backend accepts must be SERVED by some
 * shard here.
 *
 * The `satisfies` above and this are not the same check, and only having the
 * first is a trap. `satisfies` is element-wise: it catches an id RENAMED or
 * REMOVED on the backend, because the stale literal stops being assignable.
 * It cannot catch an id ADDED, because a shorter list is still a list of valid
 * values — which is exactly the documented growth path (split one range, add
 * one id). Without this second half, a backend range the frontend never learned
 * is simply never fetched: the build is green, `tsc` is green, the Go enum test
 * is green, and those releases leave the sitemap with the loss sitting inside
 * the monitor's per-family drift tolerance.
 *
 * `SITEMAP_FAMILIES` has carried the same pair since PSY-1622 (`MissingFamily`
 * above is its addition-guard); this is the sub-shard's.
 *
 * Written over the whole wire vocabulary rather than just `releases-*` so it
 * keeps working when a second family is sub-sharded: add the new list to the
 * Exclude and the guard covers it.
 */
type UnservedWireFamily = Exclude<
  WireFamily,
  | (typeof SITEMAP_FAMILIES)[number]
  | (typeof RELEASE_SHARD_IDS)[number]
  | (typeof SHOW_SHARD_IDS)[number]
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
 * family name, which is the case for eight of the ten.
 *
 * `ids` is `readonly WireFamily[]`, not `readonly string[]`: an id written here
 * that the backend does not accept would be fetched, 422'd, and degraded to an
 * empty document — a shard silently serving nothing. Typing it against the
 * generated enum makes that a compile error instead.
 *
 * `partitionKey` is REQUIRED so that declaring a family sub-sharded forces
 * saying how, in the same edit. It is what the monitor's empty-shard rule turns
 * on, and defaulting it would default a new calendar-keyed family into alarming
 * on nearly every run — see subShardsCanBeEmpty and its call site in
 * lib/sitemap-monitor/fetch.ts, which carry the reasoning and the residual gap.
 */
interface SubShards {
  ids: readonly WireFamily[]
  /** What the ranges are cut on. See subShardsCanBeEmpty. */
  partitionKey: 'slug' | 'calendar'
}
const SUB_SHARDS: Partial<Record<Family, SubShards>> = {
  shows: { ids: SHOW_SHARD_IDS, partitionKey: 'calendar' },
  releases: { ids: RELEASE_SHARD_IDS, partitionKey: 'slug' },
}

/** Whether a family's sub-shards can be empty without anything being wrong. */
export function subShardsCanBeEmpty(family: Family): boolean {
  return SUB_SHARDS[family]?.partitionKey === 'calendar'
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
  for (const id of SUB_SHARDS[family]?.ids ?? [family]) {
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
