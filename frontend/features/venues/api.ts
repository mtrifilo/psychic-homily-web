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

/** Which side of "today" a venue-shows request asks for. */
export type VenueShowsTimeFilter = 'upcoming' | 'past' | 'all'

/**
 * The full page of a venue's shows, as requested by the surfaces that render
 * the whole list: the venue page's `VenueShowsList` and the Atlas
 * `VenuePanel`. Sharing one constant is what lets those two share a cache
 * entry, because `venueQueryKeys.showsPage()` keys on the limit.
 *
 * It is NOT a rule every caller must obey. Surfaces that deliberately want a
 * shorter page say so and get their own entry — `VenueCard` fetches a compact
 * preview with a "view all" link out, and the collection graph's entity panel
 * only needs the next show. Before PSY-1698 the key ignored the limit, so
 * those smaller requests silently answered for the venue page (and vice
 * versa) depending purely on which one landed first.
 */
export const VENUE_SHOWS_PAGE_LIMIT = 50

/**
 * The timezone every venue-shows caller should send.
 *
 * It only sets the backend's "today" boundary for the upcoming/past split —
 * rendering is always done in the VENUE's zone (PSY-985/986), never this one.
 * A caller that omits it gets the backend's UTC default, which puts the
 * boundary in the wrong place for most of the Americas.
 *
 * Evaluated at import, and this module reaches the server graph (via
 * `lib/queryClient.ts`), so on the server this resolves to the SERVER's zone.
 * Since PSY-1698 it is part of `venueQueryKeys.showsPage()`, which makes one
 * fact load-bearing: no route may server-prefetch venue shows. Seeding the
 * cache from a server component would compute this key under the server's
 * zone and the browser would then look under its own, missing the seed
 * silently. Today no route does — the venue page seeds only `venues.detail`
 * (see `app/venues/[slug]/page.tsx`) — so every key that reaches a request is
 * built from the browser's zone.
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
  /**
   * The PREFIX every venue-shows entry lives under — an invalidation handle,
   * not a cache key. `showsPage()` builds the key an actual request lands on.
   */
  shows: (venueIdOrSlug: string | number) => ['venues', 'shows', String(venueIdOrSlug)] as const,
  /**
   * The cache key for one venue-shows REQUEST (PSY-1698).
   *
   * Every parameter that changes the response body is in the key, so two
   * surfaces share an entry exactly when they are asking the same question.
   * Before this, the key stopped at venue + time filter while `limit` and
   * `timezone` still varied per caller, so a compact 20-row preview and the
   * venue page's 50-row list collided: whichever request resolved first
   * answered for both, for the whole 5-minute staleTime, and which one that
   * was depended on the order the user happened to navigate in.
   *
   * Extends `shows()` rather than replacing it so the coarse `['venues']`
   * invalidation in `createInvalidateQueries` keeps prefix-matching every page.
   */
  showsPage: (
    venueIdOrSlug: string | number,
    params: {
      timeFilter: VenueShowsTimeFilter
      /** Omit for "not sent" — the backend's own default then applies. */
      limit?: number
      /** Omit for "not sent" — the backend defaults the boundary to UTC. */
      timezone?: string
    },
  ) =>
    [
      ...venueQueryKeys.shows(venueIdOrSlug),
      params.timeFilter,
      // Both normalized to null so "not sent" is ONE key rather than a hole
      // that hashes differently from an explicit undefined. Callers must pass
      // what the request actually sent, not what they were handed.
      params.limit ?? null,
      params.timezone ?? null,
    ] as const,
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
