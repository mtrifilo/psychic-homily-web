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
  // PSY-1753: venue-local calendar years that have at least one show, with
  // counts. Drives the past-shows year strip — the strip must never link a year
  // that would land on an empty page, so the years come from data, never from a
  // range the frontend invents.
  SHOW_YEARS: (venueIdOrSlug: string | number) =>
    `${API_BASE_URL}/venues/${venueIdOrSlug}/shows/years`,
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
 * The page of a venue's shows requested by surfaces that render a venue's list
 * in full without paging it: today the Atlas `VenuePanel`.
 *
 * It is NOT a rule every caller must obey. Surfaces that deliberately want a
 * different page say so and get their own entry — `VenueCard` fetches a compact
 * preview with a "view all" link out, and the collection graph's entity panel
 * only needs the next show. Before PSY-1698 the key ignored the limit, so
 * those smaller requests silently answered for the venue page (and vice
 * versa) depending purely on which one landed first.
 *
 * The venue PAGE no longer uses it: since PSY-1753 its two sections ask
 * different questions (`VENUE_UPCOMING_SHOWS_LIMIT` unpaginated, and a paged
 * past archive at `VENUE_PAST_SHOWS_PAGE_LIMIT`), so it no longer shares a
 * cache entry with the Atlas panel. That sharing was a bonus, not a
 * requirement — `showsPage()` keys on every parameter precisely so surfaces
 * asking different questions get different entries instead of one answering
 * for the other.
 */
export const VENUE_SHOWS_PAGE_LIMIT = 50

/**
 * Rows per page in the venue page's PAST shows archive (PSY-1753).
 *
 * Deliberately the same on every device: the page number is in a shareable
 * URL, so the same link has to mean the same rows on a phone and a desktop.
 */
export const VENUE_PAST_SHOWS_PAGE_LIMIT = 50

/**
 * The venue page's UPCOMING section is unpaginated, so it asks for the backend
 * maximum in one request (PSY-1753). Booking horizons bound the real number
 * far below this (~100 at the busiest venue observed); the cap exists so a
 * pathological venue degrades into a truncation note rather than an unbounded
 * page. `VenueShowsList` renders that note when `total` exceeds what came back.
 */
export const VENUE_UPCOMING_SHOWS_LIMIT = 200

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
   * Before this, the key stopped at venue + time filter while `limit` still
   * varied per caller, so a compact 20-row preview and the venue page's 50-row
   * list collided: whichever request resolved first answered for both, for the
   * whole 5-minute staleTime, and which one that was depended on the order the
   * user happened to navigate in.
   *
   * NOTHING here is per-viewer, so a key a server computes is the key the
   * browser then looks under, and server-prefetching this list is now possible.
   * No route does it today: `archiveApi.ts` hands its rows over as
   * `initialData`, which never depended on the key. A viewer timezone used to
   * sit in this list, back when the endpoint read one; it does not any more
   * (the upcoming/past split is made in each show's own venue-local day), so do
   * not put a browser-only value back in.
   *
   * ONE PRECONDITION SURVIVES for anyone taking that up: the first segment is
   * `String(venueIdOrSlug)`, and the two forms hash differently. Callers are
   * already split — the venue page, the Atlas panel and `VenueCard` pass the
   * NUMERIC id resolved from a fetched venue, while the collection entity panel
   * passes a slug — so a server seed has to key on whatever identity ITS
   * consumer passes, not on whichever one the route happens to have. Get that
   * wrong and it misses in exactly the silent way described above.
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
      /** Venue-local calendar year filter. Omit for "every year" (PSY-1753). */
      year?: number
      /** Rows skipped. Omit for the first page (PSY-1753). */
      offset?: number
    },
  ) =>
    [
      ...venueQueryKeys.shows(venueIdOrSlug),
      params.timeFilter,
      // All normalized to null so "not sent" is ONE key rather than a hole
      // that hashes differently from an explicit undefined. Callers must pass
      // what the request actually sent, not what they were handed.
      params.limit ?? null,
      params.year ?? null,
      params.offset ?? null,
    ] as const,
  /**
   * The year histogram behind the past-shows year strip (PSY-1753).
   *
   * Keyed under the same `shows()` prefix as the pages it navigates, so the
   * coarse `['venues']` invalidation and any per-venue shows invalidation
   * clear the strip and the rows together — a strip that outlived its rows
   * would link years that no longer have any.
   *
   * `'years'` cannot collide with a `showsPage()` key: the slot it occupies
   * there holds a `VenueShowsTimeFilter`, which is only ever
   * upcoming/past/all.
   */
  showYears: (
    venueIdOrSlug: string | number,
    timeFilter: VenueShowsTimeFilter,
  ) =>
    [...venueQueryKeys.shows(venueIdOrSlug), 'years', timeFilter] as const,
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

/**
 * The exact `showsPage` parameters for one page of the venue page's PAST shows
 * archive (PSY-1753).
 *
 * The archive both ISSUES a request for the page it is on and PEEKS at the
 * cache for its neighbours (to label each page button with the months it
 * covers). Those two call sites have to agree on the key down to the
 * "sent / not sent" normalization — `showsPage` distinguishes `0` from
 * "absent", so a peek that passed `offset: 0` where the request passed
 * `undefined` would silently never find page 1. Building both from this one
 * function is what makes that class of miss impossible rather than merely
 * unlikely.
 *
 * @param page 1-based page number.
 * @param year Venue-local calendar year, or null for every year.
 */
export const venuePastShowsPageParams = (
  page: number,
  year: number | null,
) => ({
  timeFilter: 'past' as const,
  limit: VENUE_PAST_SHOWS_PAGE_LIMIT,
  year: year ?? undefined,
  // `|| undefined` rather than a ternary: page 1's offset is 0, which the key
  // must record as "not sent" because the request does not send it.
  offset: (page - 1) * VENUE_PAST_SHOWS_PAGE_LIMIT || undefined,
})

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
