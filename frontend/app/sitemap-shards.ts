import type { components } from '@/types/api'

type SitemapEntries = components['schemas']['SitemapEntries']

/** The entity families, minus the `$schema` key Huma adds to every response. */
export type Family = Exclude<keyof SitemapEntries, '$schema'>

/** Non-entity shard: static pages + local MDX (blog / DJ sets). */
export const PAGES_SHARD_ID = 'pages'

/**
 * Entity-family shard ids. Shared by generateSitemaps() and the
 * /sitemap-index route so a new family cannot appear in one place without
 * the other. Keep in sync with the Huma `family` query enum on
 * GET /sitemap/entries and the known-family map in SitemapService.Entries.
 *
 * There is deliberately no `scene_days` family. `buildSceneDayMetadata` depends
 * on that absence: it canonicalizes the rolling /tonight page to the WEEK
 * permalink precisely because day permalinks are announced nowhere. Adding a
 * day family here obliges revisiting that canonical.
 */
export const FAMILY_SHARD_IDS = [
  'shows',
  'artists',
  'venues',
  'scenes',
  'scene_weeks',
  'labels',
  'releases',
  'festivals',
  'tags',
] as const satisfies readonly Family[]

/** Compile-time guard: every schema family must appear in FAMILY_SHARD_IDS. */
type MissingFamily = Exclude<Family, (typeof FAMILY_SHARD_IDS)[number]>
type AssertNoMissingFamily = [MissingFamily] extends [never]
  ? true
  : { missing: MissingFamily }
const _assertNoMissingFamily: AssertNoMissingFamily = true
void _assertNoMissingFamily

/**
 * Every shard `generateSitemaps()` emits, in route-table order.
 *
 * Named here rather than composed inline at each site: four modules need the
 * same list — the generator, the index route, the freshness monitor, and the
 * post-build prerender gate — and an inline `[PAGES_SHARD_ID, ...FAMILY_SHARD_IDS]`
 * in each is a fork waiting to happen the day a non-family shard is added.
 */
export const ALL_SHARD_IDS = [PAGES_SHARD_ID, ...FAMILY_SHARD_IDS] as const

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
 * `scenes` and `scene_weeks` deliberately share `/scenes`: a scene is
 * `/scenes/{city}` and a scene week is `/scenes/{city}/{iso-week}`. Anything
 * mapping a URL back to a family has to disambiguate those two by segment
 * count, not by prefix.
 */
export const FAMILY_URL_PREFIXES = {
  shows: '/shows',
  artists: '/artists',
  venues: '/venues',
  scenes: '/scenes',
  scene_weeks: '/scenes',
  labels: '/labels',
  releases: '/releases',
  festivals: '/festivals',
  tags: '/tags',
} as const satisfies Record<Family, string>
