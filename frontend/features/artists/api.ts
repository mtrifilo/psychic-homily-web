/**
 * Artists API Configuration
 *
 * Co-located endpoint definitions and query keys for the artists feature.
 * Imported by artist hooks and re-exported from lib/api.ts and lib/queryClient.ts
 * for backward compatibility.
 */

import { API_BASE_URL } from '@/lib/api-base'

// ============================================================================
// Endpoints
// ============================================================================

export const artistEndpoints = {
  LIST: `${API_BASE_URL}/artists`,
  CITIES: `${API_BASE_URL}/artists/cities`,
  SEARCH: `${API_BASE_URL}/artists/search`,
  GET: (artistIdOrSlug: string | number) => `${API_BASE_URL}/artists/${artistIdOrSlug}`,
  SHOWS: (artistIdOrSlug: string | number) => `${API_BASE_URL}/artists/${artistIdOrSlug}/shows`,
  // PSY-1754: venue-local calendar years in which this artist played at least
  // one show, with counts. Drives the past-shows year strip — the strip must
  // never link a year that would land on an empty page, so the years come from
  // data, never from a range the frontend invents.
  SHOW_YEARS: (artistIdOrSlug: string | number) =>
    `${API_BASE_URL}/artists/${artistIdOrSlug}/shows/years`,
  LABELS: (artistIdOrSlug: string | number) => `${API_BASE_URL}/artists/${artistIdOrSlug}/labels`,
  ALIASES: (artistId: string | number) => `${API_BASE_URL}/artists/${artistId}/aliases`,
  GRAPH: (artistId: string | number) => `${API_BASE_URL}/artists/${artistId}/graph`,
  GRAPH_CARD: (artistId: string | number) => `${API_BASE_URL}/artists/${artistId}/graph-card`,
  BILL_COMPOSITION: (artistId: string | number) => `${API_BASE_URL}/artists/${artistId}/bill-composition`,
  RELATED: (artistId: string | number) => `${API_BASE_URL}/artists/${artistId}/related`,
  RELATIONSHIPS: {
    CREATE: `${API_BASE_URL}/artists/relationships`,
    VOTE: (sourceId: number, targetId: number) =>
      `${API_BASE_URL}/artists/relationships/${sourceId}/${targetId}/vote`,
  },
  DELETE: (artistId: string | number) => `${API_BASE_URL}/artists/${artistId}`,
  /**
   * The caller's own pending report for an artist. Submitting one goes through
   * `useReportEntity` (POST /artists/{id}/report) rather than an artist-specific
   * endpoint — PSY-1633 consolidated artist reports onto the entity pipeline, so
   * there is no artist-only REPORT endpoint here to drift out of sync with it.
   */
  MY_REPORT: (artistId: string | number) =>
    `${API_BASE_URL}/artists/${artistId}/my-report`,
} as const

// ============================================================================
// Shared artist-shows page parameters
// ============================================================================

/** Which side of "today" an artist-shows request asks for. */
export type ArtistShowsTimeFilter = 'upcoming' | 'past' | 'all'

/**
 * The page of an artist's shows requested by surfaces that render the list
 * without paging it: today the Atlas `ArtistPanel`.
 *
 * It is NOT a rule every caller must obey. Surfaces that deliberately want a
 * different page say so and get their own entry, because `showsPage()` keys on
 * every parameter. Before PSY-1754 the key ignored the limit, so a smaller
 * request could silently answer for a larger one (and vice versa) depending
 * purely on which one landed first — the artist twin of the venue bug PSY-1698
 * fixed.
 *
 * The artist PAGE no longer uses it: since PSY-1754 its two sections ask
 * different questions (`ARTIST_UPCOMING_SHOWS_LIMIT` unpaginated, and a paged
 * past archive at `ARTIST_PAST_SHOWS_PAGE_LIMIT`).
 */
export const ARTIST_SHOWS_PAGE_LIMIT = 50

/**
 * Rows per page in the artist page's PAST shows archive (PSY-1754).
 *
 * Deliberately the same on every device: the page number is in a shareable
 * URL, so the same link has to mean the same rows on a phone and a desktop.
 */
export const ARTIST_PAST_SHOWS_PAGE_LIMIT = 50

/**
 * The artist page's UPCOMING section is unpaginated, so it asks for the backend
 * maximum in one request (PSY-1754). Touring schedules bound the real number
 * far below this; the cap exists so a pathological artist degrades into a
 * truncation note rather than an unbounded page. `ArtistShowsList` renders that
 * note when `total` exceeds what came back.
 */
export const ARTIST_UPCOMING_SHOWS_LIMIT = 200

// ============================================================================
// Query Keys
// ============================================================================

export const artistQueryKeys = {
  all: ['artists'] as const,
  list: (filters?: Record<string, unknown>) =>
    ['artists', 'list', filters] as const,
  cities: ['artists', 'cities'] as const,
  search: (query: string) =>
    ['artists', 'search', query.toLowerCase()] as const,
  detail: (idOrSlug: string | number) => ['artists', 'detail', String(idOrSlug)] as const,
  /**
   * The PREFIX every artist-shows entry lives under — an invalidation handle,
   * not a cache key. `showsPage()` builds the key an actual request lands on.
   */
  shows: (artistIdOrSlug: string | number) => ['artists', 'shows', String(artistIdOrSlug)] as const,
  /**
   * The cache key for one artist-shows REQUEST (PSY-1754).
   *
   * Every parameter that changes the response body is in the key, so two
   * surfaces share an entry exactly when they are asking the same question.
   * Before this, the key stopped at artist + time filter while `limit` still
   * varied per caller, so a compact preview and the artist page's list
   * collided: whichever request resolved first answered for both, for the whole
   * 5-minute staleTime, and which one that was depended on the order the user
   * happened to navigate in. The venue side fixed exactly this in PSY-1698; the
   * artist side never got the fix.
   *
   * NOTHING here is per-viewer, and that is what makes a server-rendered first
   * screen possible: a key a server computes is the key the browser then looks
   * under. A viewer timezone used to sit in this list, back when the endpoint
   * read one; it does not any more (the upcoming/past split is made in each
   * show's own venue-local day), so do not put a browser-only value back in.
   *
   * Extends `shows()` rather than replacing it so the coarse `['artists']`
   * invalidation in `createInvalidateQueries` keeps prefix-matching every page.
   */
  showsPage: (
    artistIdOrSlug: string | number,
    params: {
      timeFilter: ArtistShowsTimeFilter
      /** Omit for "not sent" — the backend's own default then applies. */
      limit?: number
      /** Venue-local calendar year filter. Omit for "every year". */
      year?: number
      /** Rows skipped. Omit for the first page. */
      offset?: number
    },
  ) =>
    [
      ...artistQueryKeys.shows(artistIdOrSlug),
      params.timeFilter,
      // All normalized to null so "not sent" is ONE key rather than a hole
      // that hashes differently from an explicit undefined. Callers must pass
      // what the request actually sent, not what they were handed.
      params.limit ?? null,
      params.year ?? null,
      params.offset ?? null,
    ] as const,
  /**
   * The year histogram behind the past-shows year strip (PSY-1754).
   *
   * Keyed under the same `shows()` prefix as the pages it navigates, so the
   * coarse `['artists']` invalidation and any per-artist shows invalidation
   * clear the strip and the rows together — a strip that outlived its rows
   * would link years that no longer have any.
   *
   * `'years'` cannot collide with a `showsPage()` key: the slot it occupies
   * there holds an `ArtistShowsTimeFilter`, which is only ever
   * upcoming/past/all.
   */
  showYears: (
    artistIdOrSlug: string | number,
    timeFilter: ArtistShowsTimeFilter,
  ) =>
    [...artistQueryKeys.shows(artistIdOrSlug), 'years', timeFilter] as const,
  labels: (artistIdOrSlug: string | number) => ['artists', 'labels', String(artistIdOrSlug)] as const,
  aliases: (artistId: number) => ['artists', 'aliases', artistId] as const,
  graph: (idOrSlug: string | number, types?: string[]) =>
    ['artists', 'graph', String(idOrSlug), types] as const,
  graphCard: (idOrSlug: string | number) =>
    ['artists', 'graphCard', String(idOrSlug)] as const,
  billComposition: (idOrSlug: string | number, months: number) =>
    ['artists', 'billComposition', String(idOrSlug), months] as const,
} as const

// ============================================================================
// Server-rendered first screen (PSY-1774)
// ============================================================================

/** The page size `ArtistList` requests, and pages through with `offset`. */
export const ARTIST_LIST_PAGE_LIMIT = 50

/**
 * The exact request `ArtistList` issues on its FIRST render of a bare
 * `/artists`, and the cache key that request lands on.
 *
 * `app/artists/page.tsx` fetches the URL server-side and seeds the key, so the
 * first page of artists is in the server HTML. The two halves are declared
 * together because they only work as a pair: seed a key the hook does not ask
 * for and the page silently reverts to its pre-SSR behaviour — the hook misses
 * the cache and renders its spinner on BOTH the server and the hydration pass,
 * so nothing looks broken and nothing is server-rendered either. That failure
 * is invisible by construction, which is why `useArtistsFirstScreen.test.tsx`
 * asserts the hook actually registers this key and requests this URL. The
 * sibling `useArtists.test.tsx` cannot: it `vi.mock`s this module, so it never
 * sees the real constants.
 *
 * `offset` is in the KEY but not the URL, because `useArtists` omits a zero
 * offset from the request and keys on it unconditionally. Both halves are
 * built from that one hook, so this pair records what it actually does rather
 * than what would be tidier.
 *
 * A filtered `/artists?cities=…` or a deep page `?page=2` is deliberately NOT
 * covered: the hook keys on the filter and the offset, misses this entry, and
 * both render passes agree on the spinner. No SSR benefit there, and no
 * hydration mismatch either.
 */
export const ARTIST_LIST_FIRST_SCREEN_URL = `${artistEndpoints.LIST}?limit=${ARTIST_LIST_PAGE_LIMIT}`

export const ARTIST_LIST_FIRST_SCREEN_KEY = artistQueryKeys.list({
  cities: undefined,
  tags: undefined,
  tagMatch: undefined,
  limit: ARTIST_LIST_PAGE_LIMIT,
  offset: 0,
})

/**
 * The exact `showsPage` parameters for one page of the artist page's PAST shows
 * archive (PSY-1754).
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
export const artistPastShowsPageParams = (
  page: number,
  year: number | null,
) => ({
  timeFilter: 'past' as const,
  limit: ARTIST_PAST_SHOWS_PAGE_LIMIT,
  year: year ?? undefined,
  // `|| undefined` rather than a ternary: page 1's offset is 0, which the key
  // must record as "not sent" because the request does not send it.
  offset: (page - 1) * ARTIST_PAST_SHOWS_PAGE_LIMIT || undefined,
})
