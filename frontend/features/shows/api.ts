/**
 * Shows API Configuration
 *
 * Co-located endpoint definitions and query keys for the shows feature.
 * Imported by show hooks and re-exported from lib/api.ts and lib/queryClient.ts
 * for backward compatibility.
 */

import { API_BASE_URL } from '@/lib/api-base'

// ============================================================================
// Endpoints
// ============================================================================

export const showEndpoints = {
  SUBMIT: `${API_BASE_URL}/shows`,
  UPCOMING: `${API_BASE_URL}/shows/upcoming`,
  CITIES: `${API_BASE_URL}/shows/cities`,
  // PSY-372 / PSY-520: autocomplete endpoint, used by useEntitySearch.
  SEARCH: `${API_BASE_URL}/shows/search`,
  GET: (showId: string | number) => `${API_BASE_URL}/shows/${showId}`,
  UPDATE: (showId: string | number) => `${API_BASE_URL}/shows/${showId}`,
  DELETE: (showId: string | number) => `${API_BASE_URL}/shows/${showId}`,
  UNPUBLISH: (showId: string | number) =>
    `${API_BASE_URL}/shows/${showId}/unpublish`,
  MAKE_PRIVATE: (showId: string | number) =>
    `${API_BASE_URL}/shows/${showId}/make-private`,
  PUBLISH: (showId: string | number) =>
    `${API_BASE_URL}/shows/${showId}/publish`,
  SET_SOLD_OUT: (showId: string | number) =>
    `${API_BASE_URL}/shows/${showId}/sold-out`,
  SET_CANCELLED: (showId: string | number) =>
    `${API_BASE_URL}/shows/${showId}/cancelled`,
  MY_SUBMISSIONS: `${API_BASE_URL}/shows/my-submissions`,
  // Export endpoint (dev only)
  EXPORT: (showId: string | number) =>
    `${API_BASE_URL}/shows/${showId}/export`,
  // Show report endpoints
  REPORT: (showId: string | number) =>
    `${API_BASE_URL}/shows/${showId}/report`,
  MY_REPORT: (showId: string | number) =>
    `${API_BASE_URL}/shows/${showId}/my-report`,
} as const

// ============================================================================
// Query Keys
// ============================================================================

export const showQueryKeys = {
  all: ['shows'] as const,
  list: (filters?: Record<string, unknown>) =>
    ['shows', 'list', filters] as const,
  // No timezone segment: `GET /shows/cities` counts the same venue-local
  // upcoming partition for every visitor (PSY-1678), so a per-viewer key would
  // fragment the cache across entries that can only ever hold identical data.
  cities: () => ['shows', 'cities'] as const,
  detail: (id: string) => ['shows', 'detail', id] as const,
  userShows: (userId: string) => ['shows', 'user', userId] as const,
  search: (query: string) => ['shows', 'search', query.toLowerCase()] as const,
} as const

// ============================================================================
// Server-rendered first screen (PSY-1624)
// ============================================================================

/**
 * The exact requests `ShowList` issues on its FIRST render of a bare `/shows`,
 * and the cache keys those requests land on.
 *
 * `app/shows/page.tsx` fetches these URLs server-side and seeds these keys, so
 * the first screen of shows is in the server HTML rather than appearing only
 * after the browser has downloaded, parsed and run the bundle.
 *
 * "First render" is doing real work in that sentence. `ShowList` derives its
 * city filter during render from three per-visitor sources: the `?cities=`
 * param, the signed-in viewer's `favorite_cities`, and the IP-geo default. None
 * of that is knowable by a server rendering one cacheable answer for everyone,
 * so the seeded pair is the CANONICAL one: no filter at all. Two of the three
 * sources resolve to exactly that on a first render anyway, since geo arrives
 * from a later client fetch and a bare URL contributes no filter.
 *
 * A filtered deep link, and a signed-in viewer whose `favorite_cities` apply,
 * both MISS this entry. The hook and the seed disagree, so both render passes
 * agree on the skeleton and those pages behave exactly as they did before this
 * existed. That is the failure mode by design: degraded, never mismatched. The
 * favourites case is an accepted limitation rather than an oversight; giving it
 * a server-rendered first screen means resolving per-visitor state on a
 * cacheable route, which is a separate decision.
 *
 * WHAT MAKES THE SEED LAND, and the reason PSY-1678 could delete the machinery
 * PSY-1624 needed: these requests carry no PER-VIEWER input. `GET
 * /shows/upcoming` decides "upcoming" against each show's own venue timezone, so
 * one canonical answer is the correct answer for every visitor. The filterless
 * KEY below is therefore exactly what the hooks ask for on a cold anon `/shows`
 * — the seeded entry is a hit, and the hydration commit has nothing to refetch.
 * Under the old viewer-timezone contract the key varied per viewer, so it could
 * only ever be an approximation the client re-fetched and discarded.
 *
 * A bare `/shows` therefore sends no query string at all, and the seeded URL is
 * the endpoint itself.
 *
 * The URL and the key have to stay a matched pair, and that is unenforceable at
 * the type level: `useShowsFirstScreen.test.tsx` asserts the hooks really do
 * register these keys against these URLs. The sibling `useShows.test.tsx`
 * cannot, because it `vi.mock`s this module and so never sees the real
 * constants. A drifted pair produces no error anywhere, just a page that quietly
 * stops being server-rendered.
 */
export const UPCOMING_SHOWS_FIRST_SCREEN_URL = showEndpoints.UPCOMING

export const UPCOMING_SHOWS_FIRST_SCREEN_KEY = showQueryKeys.list({
  cursor: undefined,
  limit: undefined,
  city: undefined,
  state: undefined,
  cities: undefined,
  tags: undefined,
  tagMatch: undefined,
})

export const SHOW_CITIES_FIRST_SCREEN_URL = showEndpoints.CITIES

export const SHOW_CITIES_FIRST_SCREEN_KEY = showQueryKeys.cities()
