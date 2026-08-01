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
  cities: (timezone?: string) => ['shows', 'cities', timezone] as const,
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
 * city filter during render from three per-visitor sources — the `?cities=`
 * param, the signed-in viewer's `favorite_cities`, and the IP-geo default —
 * and asks for the viewer's timezone. None of that is knowable by a server
 * rendering one cacheable answer for everyone, so the seeded pair is the
 * CANONICAL one: no filter, no timezone. Two of the three resolve to exactly
 * that on a first render anyway (geo arrives from a later client fetch; a bare
 * URL contributes no filter), and the timezone is held to `undefined` through
 * hydration by `useBrowserTimezone`. The signed-in-with-favorites case and the
 * filtered deep link both miss this entry — the hook and the seed disagree, so
 * both render passes agree on the skeleton and the page behaves exactly as it
 * did before this existed. That is the failure mode by design: degraded, never
 * mismatched.
 *
 * URL and key are declared together because they only work as a pair, and the
 * pairing is unenforceable at the type level — `useShows.test.tsx` asserts the
 * hooks really do register these keys against these URLs, because a drifted
 * pair produces no error anywhere, just a page that quietly stops being
 * server-rendered.
 */
export const UPCOMING_SHOWS_FIRST_SCREEN_URL = showEndpoints.UPCOMING

export const UPCOMING_SHOWS_FIRST_SCREEN_KEY = showQueryKeys.list({
  timezone: undefined,
  cursor: undefined,
  limit: undefined,
  city: undefined,
  state: undefined,
  cities: undefined,
  tags: undefined,
  tagMatch: undefined,
})

export const SHOW_CITIES_FIRST_SCREEN_URL = showEndpoints.CITIES

export const SHOW_CITIES_FIRST_SCREEN_KEY = showQueryKeys.cities(undefined)
