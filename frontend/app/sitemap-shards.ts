import type { components } from '@/types/api'

type SitemapEntries = components['schemas']['SitemapEntries']

/** The entity families, minus the `$schema` key Huma adds to every response. */
export type Family = Exclude<keyof SitemapEntries, '$schema'>

/** Non-entity shard: static pages + local MDX (blog / DJ sets). */
export const PAGES_SHARD_ID = 'pages'

/**
 * Entity-family shard ids, in the same order as FAMILY_ROUTES keys would
 * enumerate. Shared by generateSitemaps() and the /sitemap.xml index route so
 * a new family cannot appear in one place without the other.
 *
 * Typed as a total tuple of Family so adding a backend field forces a compile
 * error here until the id is listed.
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
