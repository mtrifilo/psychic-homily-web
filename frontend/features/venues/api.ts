/**
 * Venues API Configuration
 *
 * Co-located endpoint definitions and query keys for the venues feature.
 * Imported by venue hooks and re-exported from lib/api.ts and lib/queryClient.ts
 * for backward compatibility.
 */

import { API_BASE_URL } from '@/lib/api-base'

// ============================================================================
// Endpoints
// ============================================================================

export const venueEndpoints = {
  LIST: `${API_BASE_URL}/venues`,
  CITIES: `${API_BASE_URL}/venues/cities`,
  SEARCH: `${API_BASE_URL}/venues/search`,
  GET: (venueIdOrSlug: string | number) => `${API_BASE_URL}/venues/${venueIdOrSlug}`,
  SHOWS: (venueIdOrSlug: string | number) => `${API_BASE_URL}/venues/${venueIdOrSlug}/shows`,
  GENRES: (venueIdOrSlug: string | number) => `${API_BASE_URL}/venues/${venueIdOrSlug}/genres`,
  // PSY-365: venue-rooted co-bill graph endpoint.
  BILL_NETWORK: (venueIdOrSlug: string | number) =>
    `${API_BASE_URL}/venues/${venueIdOrSlug}/bill-network`,
  // PSY-1542: one-tap "this venue's info is still accurate". Numeric id only —
  // the backend refuses a slug here so a rate-limited write has exactly one
  // addressable identity per row.
  CONFIRM: (venueId: number) => `${API_BASE_URL}/venues/${venueId}/confirm`,
  UPDATE: (venueIdOrSlug: string | number) => `${API_BASE_URL}/venues/${venueIdOrSlug}`,
  DELETE: (venueIdOrSlug: string | number) => `${API_BASE_URL}/venues/${venueIdOrSlug}`,
} as const

// ============================================================================
// Shared venue-shows page parameters
// ============================================================================

/**
 * The page size EVERY venue-shows caller must use.
 *
 * `venueQueryKeys.shows()` keys on venue id + time filter and deliberately not
 * on limit or timezone, so two surfaces requesting the same venue's upcoming
 * shows share one cache entry. That is only safe while they request the same
 * page: a caller that quietly asked for 5 would hand the venue page a
 * five-row list, or be handed fifty rows itself, depending purely on which
 * request landed first. One constant makes the agreement structural instead
 * of a comment in two files that can drift apart.
 */
export const VENUE_SHOWS_PAGE_LIMIT = 50

/**
 * The timezone every venue-shows caller must send, for the same reason.
 *
 * It only sets the backend's "today" boundary for the upcoming/past split —
 * rendering is always done in the VENUE's zone (PSY-985/986), never this one.
 *
 * Evaluated at import, and this module reaches the server graph (via
 * `lib/queryClient.ts`), so on the server this resolves to the SERVER's zone.
 * That is inert rather than a hydration hazard: it is not part of
 * `venueQueryKeys.shows()`, and no route server-prefetches venue shows — the
 * venue page seeds only `venues.detail` — so the value that ever reaches a
 * request is always the browser's.
 */
export const VENUE_SHOWS_VIEWER_TIMEZONE =
  Intl.DateTimeFormat().resolvedOptions().timeZone

// ============================================================================
// Query Keys
// ============================================================================

export const venueQueryKeys = {
  all: ['venues'] as const,
  list: (filters?: Record<string, unknown>) =>
    ['venues', 'list', filters] as const,
  cities: ['venues', 'cities'] as const,
  detail: (idOrSlug: string | number) => ['venues', 'detail', String(idOrSlug)] as const,
  search: (query: string) =>
    ['venues', 'search', query.toLowerCase()] as const,
  // NOTE: keyed on venue + time filter ONLY — not on limit or timezone. Two
  // surfaces asking for the same venue's upcoming shows with DIFFERENT limits
  // therefore share one cache entry, and whichever request lands first answers
  // for both. Rather than widen the key (and split the cache for two surfaces
  // that want the same page), every caller passes the same page parameters —
  // see VENUE_SHOWS_PAGE_LIMIT below.
  shows: (venueIdOrSlug: string | number) => ['venues', 'shows', String(venueIdOrSlug)] as const,
  genres: (venueIdOrSlug: string | number) => ['venues', 'genres', String(venueIdOrSlug)] as const,
  // PSY-365: bill-network cache is keyed by venue + active window so
  // toggling all/12m/year cycles through cache entries instead of refetching.
  billNetwork: (
    venueIdOrSlug: string | number,
    window: string,
    year?: number,
  ) =>
    [
      'venues',
      'bill-network',
      String(venueIdOrSlug),
      window,
      year ?? null,
    ] as const,
} as const

// ============================================================================
// Server-rendered first screen (PSY-1624)
// ============================================================================

/** The page size `VenueList` requests, and pages through with `offset`. */
export const VENUE_LIST_PAGE_LIMIT = 50

/**
 * The exact request `VenueList` issues on its FIRST render of a bare
 * `/venues`, and the cache key that request lands on.
 *
 * `app/venues/page.tsx` fetches the URL server-side and seeds the key, so the
 * first page of venues is in the server HTML. The two halves are declared
 * together because they only work as a pair: seed a key the hook does not ask
 * for and the page silently reverts to its pre-SSR behaviour — the hook misses
 * the cache and renders its spinner on BOTH the server and the hydration pass,
 * so nothing looks broken and nothing is server-rendered either. That failure
 * is invisible by construction, which is why `useVenuesFirstScreen.test.tsx`
 * asserts the hook actually registers this key and requests this URL. The
 * sibling `useVenues.test.tsx` cannot: it `vi.mock`s this module, so it never
 * sees the real constants.
 *
 * A filtered `/venues?cities=…` deep link is deliberately NOT covered: the
 * hook keys on the filter, misses this entry, and both render passes agree on
 * the spinner. No SSR benefit there, and no hydration mismatch either.
 */
export const VENUE_LIST_FIRST_SCREEN_URL = `${venueEndpoints.LIST}?limit=${VENUE_LIST_PAGE_LIMIT}`

export const VENUE_LIST_FIRST_SCREEN_KEY = venueQueryKeys.list({
  state: undefined,
  city: undefined,
  cities: undefined,
  limit: VENUE_LIST_PAGE_LIMIT,
  offset: 0,
  tags: undefined,
  tagMatch: undefined,
  includeRail: undefined,
  metroRollup: undefined,
})
