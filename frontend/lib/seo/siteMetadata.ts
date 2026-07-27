/**
 * Canonical, site-wide description string.
 *
 * Consumed by the root layout metadata (the fallback for every page that sets
 * no description of its own), the homepage metadata (plain + Open Graph), and
 * the Organization JSON-LD that search engines and AI assistants read.
 *
 * These four copies are centralized here because they had already drifted: the
 * layout fallback and the Organization schema still described the site as
 * Arizona-scoped long after the homepage copy had been rewritten, so a share
 * card, a search result, and the page itself could each claim a different
 * scope. One constant makes that class of drift impossible rather than merely
 * unlikely.
 */
export const SITE_DESCRIPTION =
  'Show lists for your city, real-time freeform radio playlists, and your music knowledge graph for artists, labels, venues, and releases around the world.'
