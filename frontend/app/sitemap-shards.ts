import type { components } from '@/types/api'

type SitemapEntries = components['schemas']['SitemapEntries']

/** The entity families, minus the `$schema` key Huma adds to every response. */
export type Family = Exclude<keyof SitemapEntries, '$schema'>

/** Non-entity shard: static pages + local MDX (blog / DJ sets). */
export const PAGES_SHARD_ID = 'pages'

/**
 * The entity families the sitemap covers.
 *
 * FAMILIES, not shard ids — the two stopped being the same thing when
 * `releases` was sub-sharded (see RELEASE_SHARD_IDS). Anything counting or
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
 * The slug ranges the `releases` family is served in (PSY-1763).
 *
 * WHY. Sharding per family (PSY-1622) exists to keep each fetch under Next's
 * 2 MB Data Cache item cap. Measured against production on 2026-08-09, the
 * releases family answered 1,530,206 raw bytes over 21,525 rows — 2.04 MB once
 * base64-encoded into a cache entry, i.e. 97% of the cap, with the next
 * sizeable import crossing it. One family no longer fits one entry.
 *
 * WHY A SLUG RANGE. The partition key has to be STABLE, because a release that
 * changes shard churns what crawlers refetch for no new information. A page
 * number (OFFSET over the slug order) is perfectly balanced and maximally
 * unstable: one insert near the front shifts every later row across every
 * boundary, on every import. A release-year range is stable but does not stay
 * balanced — the catalogue is heavily weighted to recent decades and new rows
 * land almost only at the recent end, so the hot bucket needs re-cutting
 * forever while the cold ones stay thin. A slug range is stable by
 * construction: the backend regenerates a release's slug only when its title
 * changes, which changes the URL itself, so a row cannot move between shards
 * while keeping a URL a crawler already holds.
 *
 * The cut points, the bounds they stand for, and the two readings that say the
 * balance is a property of naming rather than of one import all live with the
 * predicate that enforces them, in `releaseShards` in
 * backend/internal/services/catalog/sitemap.go.
 *
 * What a build actually wrote, read out of `.next/cache/fetch-cache` against
 * the production release catalogue — these are the entry sizes Next's own 2 MB
 * check is applied to, not the raw body scaled by the base64 ratio:
 *
 *   releases-a-e  563,747  26.9% of the cap
 *   releases-f-m  562,227  26.8%
 *   releases-n-s  489,967  23.4%
 *   releases-t-z  428,751  20.4%
 *
 * against 97% for the family as one document. `artists`, at 40.8%, is now the
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
] as const

/**
 * Families served by more than one document, and the ids those documents use.
 *
 * A family absent from this table is served by a single shard whose id IS the
 * family name, which is the case for nine of the ten.
 */
const SUB_SHARD_IDS: Partial<Record<Family, readonly string[]>> = {
  releases: RELEASE_SHARD_IDS,
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
  for (const id of SUB_SHARD_IDS[family] ?? [family]) {
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
