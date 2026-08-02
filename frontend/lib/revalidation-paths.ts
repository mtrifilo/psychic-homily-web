/**
 * Which ISR-cached pages a change to an entity makes stale.
 *
 * This module owns the page-path table and the cascade rules; it knows
 * nothing about HTTP. Two entry points consume it:
 *
 *   1. lib/proxy-revalidation.ts — mutations routed through the catch-all
 *      API proxy (browser traffic), keyed on HTTP method + backend path.
 *   2. app/api/internal/revalidate/route.ts — out-of-band writes that never
 *      touch the proxy (the ph ingest CLI today; background jobs later),
 *      keyed on entity type + slug (PSY-1691). Without it those writes stay
 *      invisible for up to the page's revalidate window.
 *
 * Keeping the table here means a new page that embeds entity data is added
 * in ONE place and both callers pick it up.
 *
 * Callers turn these paths into revalidations via safeRevalidatePath
 * (lib/revalidate-entity.ts), which handles the concrete-path vs
 * dynamic-route-pattern distinction.
 */

/**
 * ISR list pages by entity URL segment, for the rename/merge/delete cascade.
 *
 * /artists, /venues and /shows are the browse pages that embed entity NAMES
 * in a cached payload, so a rename has to reach them. /scenes also caches a
 * server-fetched payload since PSY-1624, but it is keyed on city rather than
 * on any renameable entity, so it is not part of THIS cascade — it is
 * invalidated by the count-changing mutations in `showPages` / the venue rules
 * instead. Every other entity type's browse page is still client-fetched.
 */
const ISR_LIST_PAGES: Readonly<Record<string, string>> = {
  artists: '/artists',
  venues: '/venues',
  shows: '/shows',
}

/**
 * Singular entity_type values (backend contracts) → URL path segments.
 *
 * Deliberately parallel to ENTITY_PLURAL in
 * features/contributions/hooks/useSuggestEdit.ts (a client module this
 * server-only lib must not import). Keep both in sync when adding entity
 * types.
 */
export const SINGULAR_TO_SEGMENT: Readonly<Record<string, string>> = {
  artist: 'artists',
  venue: 'venues',
  show: 'shows',
  release: 'releases',
  label: 'labels',
  festival: 'festivals',
  collection: 'collections',
}

// ---------------------------------------------------------------------------
// Cascade invalidation (PSY-941)
// ---------------------------------------------------------------------------

// Dynamic route patterns. safeRevalidatePath revalidates these with type
// 'page', invalidating every cached page under the route on its next visit.
export const ALL_SHOW_PAGES = '/shows/[slug]'
const ALL_RELEASE_PAGES = '/releases/[slug]'
const ALL_COLLECTION_PAGES = '/collections/[slug]'
export const ALL_SCENE_PAGES = '/scenes/[slug]'

// The scene BROWSE page. A plain path, not a route pattern — it started
// caching a server-fetched payload in PSY-1624, so it now needs invalidating
// alongside the scene pages it links to.
export const SCENE_LIST_PAGE = '/scenes'

// The show BROWSE page, for the same reason: PSY-1624 put entity names into
// its cached payload, so the rename cascade has to reach it.
const SHOW_LIST_PAGE = '/shows'

/**
 * Route patterns made stale when an entity of the given segment is renamed,
 * merged, or deleted — the pages that embed the entity's NAME in their own
 * ISR payload (verified against backend contracts):
 *
 *   - Show pages embed artist + venue names (ShowResponse.artists/venues)
 *   - Release pages embed artist + label names (ReleaseDetailResponse)
 *   - Collection pages embed item entity names of every entity type
 *     (CollectionItemResponse.entity_name)
 *   - Scene + tag pages embed only counts → no rename cascade
 *
 * Path-based rules can't enumerate the specific affected pages (that would
 * need revalidateTag with tagged fetches), so the whole route pattern is
 * invalidated. Rename-class mutations are rare admin/trusted events and
 * pages regenerate lazily on their next visit, so the over-invalidation is
 * cheap.
 */
const RENAME_CASCADES: Readonly<Record<string, readonly string[]>> = {
  // SHOW_LIST_PAGE, not just the show DETAIL pages: since PSY-1624 the /shows
  // browse page server-renders each row's artist and venue names, so a rename
  // leaves the old one in cached HTML until the 1h window turns over. It was
  // previously safe to omit because those names reached /shows only through a
  // JSON-LD block.
  artists: [SHOW_LIST_PAGE, ALL_SHOW_PAGES, ALL_RELEASE_PAGES, ALL_COLLECTION_PAGES],
  venues: [SHOW_LIST_PAGE, ALL_SHOW_PAGES, ALL_COLLECTION_PAGES],
  shows: [ALL_COLLECTION_PAGES],
  releases: [ALL_COLLECTION_PAGES],
  labels: [ALL_RELEASE_PAGES, ALL_COLLECTION_PAGES],
  festivals: [ALL_COLLECTION_PAGES],
}

/** Cascade route patterns for a segment; empty for types nothing embeds. */
export function cascadePages(segment: string): readonly string[] {
  return RENAME_CASCADES[segment] ?? []
}

/** Detail page (when the slug is known) + ISR list page (when one exists). */
export function entityPages(
  segment: string,
  slug: string | undefined
): Array<string | undefined> {
  return [slug ? `/${segment}/${slug}` : undefined, ISR_LIST_PAGES[segment]]
}

/**
 * Pages affected by a show mutation: the show itself, the upcoming-show
 * surfaces (/shows, /explore), the /artists and /venues lists (both embed
 * upcoming-show data), the /scenes list and every scene page (per-city show
 * counts in SceneStats), and each billed artist's detail page (artist pages
 * ISR-cache stats.shows_tracked).
 *
 * Venue detail pages are deliberately absent: the venue ISR payload is the
 * venue record only — VenueDetail client-fetches the show list.
 */
export function showPages(
  slug: string | undefined,
  billedArtistSlugs: readonly string[]
): Array<string | undefined> {
  return [
    slug ? `/shows/${slug}` : undefined,
    '/shows',
    '/explore',
    '/artists',
    '/venues',
    // The /scenes LIST, not only the per-scene pages below: since PSY-1624 it
    // server-renders each city's show counts, so a show mutation makes it
    // stale for exactly the same reason.
    SCENE_LIST_PAGE,
    ALL_SCENE_PAGES,
    ...billedArtistSlugs.map((artistSlug) => `/artists/${artistSlug}`),
  ]
}

/**
 * Pages affected by a release mutation: the release itself plus each
 * credited artist's detail page (artist pages ISR-cache stats.releases).
 */
export function releasePages(
  slug: string | undefined,
  creditedArtistSlugs: readonly string[]
): Array<string | undefined> {
  return [
    ...entityPages('releases', slug),
    ...creditedArtistSlugs.map((artistSlug) => `/artists/${artistSlug}`),
  ]
}

// ---------------------------------------------------------------------------
// Out-of-band writes (PSY-1691)
// ---------------------------------------------------------------------------

/** Entity types the internal revalidate endpoint accepts, → URL segment. */
export const OUT_OF_BAND_SEGMENTS: Readonly<Record<string, string>> =
  SINGULAR_TO_SEGMENT

/**
 * Pages made stale by an out-of-band write to one entity.
 *
 * Covers the entity's own pages plus the list surfaces its counts feed. The
 * rename cascade is NOT included unless the caller sets `renamed`, and that
 * split is the whole design decision here:
 *
 * The cascade invalidates entire route patterns (`/shows/[slug]` and friends),
 * which RENAME_CASCADES justifies above on the grounds that rename-class
 * mutations are rare admin events. Out-of-band writes are not rare — a single
 * `ph batch` run touches dozens of entities — so applying the cascade to all
 * of them would flush the show, release and collection route caches on every
 * ingest. Creates and field edits do not change any name that another page
 * embedded, so they genuinely do not need it.
 *
 * The cost of the split is that a caller which DOES rename must say so.
 * Today's only caller cannot rename: the `ph` CLI's update path writes a field
 * only when the stored value is empty (`new_info` in cli/src/lib/duplicates.ts),
 * and an existing entity's name is never empty.
 *
 * Related entities that a proxy rule would have read out of the mutation
 * response (a show's billed artists, a release's credited artists) are NOT
 * inferred here — the caller batches them as their own entries instead.
 */
export function outOfBandPages(
  segment: string,
  slug: string,
  { renamed = false }: { renamed?: boolean } = {}
): string[] {
  const ownPages = ((): Array<string | undefined> => {
    switch (segment) {
      case 'shows':
        return showPages(slug, [])
      case 'releases':
        return releasePages(slug, [])
      case 'venues':
        // A created or moved venue shifts a per-city venue_count on /scenes.
        return [...entityPages('venues', slug), SCENE_LIST_PAGE]
      default:
        return entityPages(segment, slug)
    }
  })()

  const paths = renamed ? [...ownPages, ...cascadePages(segment)] : ownPages
  return paths.filter((path): path is string => typeof path === 'string')
}
