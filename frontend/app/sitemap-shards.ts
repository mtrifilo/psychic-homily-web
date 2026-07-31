import type { components } from '@/types/api'

type SitemapEntries = components['schemas']['SitemapEntries']

/** The entity families, minus the `$schema` key Huma adds to every response. */
export type Family = Exclude<keyof SitemapEntries, '$schema'>

/** Non-entity shard: static pages + local MDX (blog / DJ sets). */
export const PAGES_SHARD_ID = 'pages'

/**
 * How long a shard's entry set stays fresh before Next tries to re-render it.
 *
 * Lives here, next to the shard ids, because two routes have to agree on it:
 * it is the `revalidate` half of the `cacheLife()` window on the shards
 * themselves (app/sitemap.ts) AND the `s-maxage` the index advertises for them
 * (app/sitemap-index/route.ts). Split across the two files, retuning one would
 * silently desync the index from the children it indexes.
 */
export const ENTRY_REVALIDATE_SECONDS = 3600

/**
 * Entity-family shard ids. Shared by generateSitemaps() and the
 * /sitemap-index route so a new family cannot appear in one place without
 * the other. Keep in sync with the Huma `family` query enum on
 * GET /sitemap/entries and the known-family map in SitemapService.Entries.
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
