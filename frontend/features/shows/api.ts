/**
 * Shows API Configuration
 *
 * Co-located endpoint definitions and query keys for the shows feature.
 * Imported by show hooks and re-exported from lib/api.ts and lib/queryClient.ts
 * for backward compatibility.
 */

import { API_BASE_URL } from '@/lib/api-base'
import { SHOWS_PAGE_SIZE } from './showsListNavigation'

// ============================================================================
// Endpoints
// ============================================================================

export const showEndpoints = {
  SUBMIT: `${API_BASE_URL}/shows`,
  UPCOMING: `${API_BASE_URL}/shows/upcoming`,
  // The OFFSET reader of the same venue-local upcoming partition `UPCOMING`
  // serves by cursor. `/shows` pages by number, which a cursor cannot address;
  // home and explore keep the cursor feed.
  CALENDAR: `${API_BASE_URL}/shows/calendar`,
  // Upcoming shows per venue-local month, under the same filters as the list.
  MONTHS: `${API_BASE_URL}/shows/months`,
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
  // The show page's also-tonight rail (PSY-1683 / PSY-1689): other shows in
  // this show's metro on this show's own date. A sub-route of /shows, but NOT
  // a frontend ROUTE — it is read by the existing `/shows/[slug]` page, so
  // `proxy.ts` needs no branch for it (see ShowDiscoveryRails).
  ALSO_TONIGHT: (showId: string | number) =>
    `${API_BASE_URL}/shows/${showId}/also-tonight`,
  // Export endpoint (dev only)
  EXPORT: (showId: string | number) =>
    `${API_BASE_URL}/shows/${showId}/export`,
  // The gig timeline: the headliner's adjacent dates plus per-act recurrence
  // in this show's place. Takes the same id-or-slug address as GET above.
  TIMELINE: (showId: string | number) =>
    `${API_BASE_URL}/shows/${showId}/timeline`,
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
  // Separate from `list` because the two read different endpoints over the same
  // partition: one keyed by cursor, one by offset. Sharing a key namespace would
  // let a cursor page and an offset page occupy the same entry.
  calendar: (filters?: Record<string, unknown>) =>
    ['shows', 'calendar', filters] as const,
  // The month histogram is a function of the FILTERS alone, never of the page,
  // so paging never re-requests it.
  months: (filters?: Record<string, unknown>) =>
    ['shows', 'months', filters] as const,
  // No timezone segment: `GET /shows/cities` counts the same venue-local
  // upcoming partition for every visitor (PSY-1678), so a per-viewer key would
  // fragment the cache across entries that can only ever hold identical data.
  cities: () => ['shows', 'cities'] as const,
  detail: (id: string) => ['shows', 'detail', id] as const,
  // No timezone or viewer segment: the rail is about the SHOW's own night, read
  // on the venue's clock, so every viewer gets the same answer for the same
  // show (the same contract `cities` states above).
  alsoTonight: (id: string) => ['shows', 'also-tonight', id] as const,
  // Keyed on the NUMERIC show id, unlike `detail` above, which is keyed on
  // whatever the route addressed the show by. The endpoint takes either, so two
  // spellings of one show would otherwise occupy two entries holding identical
  // data, and the route's server seed would miss whichever one the hook asked
  // for. The id is the spelling every caller can reach: a component holding a
  // ShowResponse has it, a route holding only a slug does not.
  timeline: (showId: number) => ['shows', 'timeline', showId] as const,
  // Every cached timeline, for invalidation. A show's neighbours are cached
  // under THEIR ids, and editing one show's date reorders their spines too, so
  // a mutation cannot name the set of keys it invalidated.
  timelineAll: () => ['shows', 'timeline'] as const,
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
 * WHAT MAKES THE SEED LAND: on the public path these requests carry no
 * PER-VIEWER input. `GET /shows/calendar` decides "upcoming" against each show's
 * own venue timezone, so one canonical answer is the correct answer for every
 * visitor. The filterless KEY below is therefore exactly what the hooks ask for
 * on a cold anon `/shows`. The seeded entry is a hit, and the hydration commit
 * has nothing to refetch.
 *
 * The handlers do consult `upcomingListIncludesNonApproved`, but neither route
 * is registered under the optional-auth group, so no viewer ever reaches that
 * branch and the keys below carry no viewer segment. Moving either route under
 * optional auth would break that pairing, and the keys would have to move with
 * it.
 *
 * The seed is PAGE 1 only, and `app/shows/page.tsx` never reads `searchParams`,
 * so every `?page=N` document ships page 1's rows and swaps them client-side.
 * The pager renders real `<a href>`s into that HTML, so a crawler DOES reach
 * the deep pages; they carry the route's static canonical back to `/shows`,
 * which is the site's canonicalize-to-root pagination policy. Seeding per page
 * would mean reading `searchParams` in the route, which costs it the
 * prerendered shell.
 *
 * The `limit` is in the URL rather than left to the endpoint's own default,
 * because the pager's arithmetic depends on the page size the request actually
 * carried. `offset` is omitted at page 1, where it would be zero.
 *
 * The URL and the key have to stay a matched pair, and that is unenforceable at
 * the type level: `useShowsFirstScreen.test.tsx` asserts the hooks really do
 * register these keys against these URLs. The sibling `useShows.test.tsx`
 * cannot, because it `vi.mock`s this module and so never sees the real
 * constants. A drifted pair produces no error anywhere, just a page that quietly
 * stops being server-rendered.
 */
export const SHOWS_CALENDAR_FIRST_SCREEN_URL = `${showEndpoints.CALENDAR}?limit=${SHOWS_PAGE_SIZE}`

export const SHOWS_CALENDAR_FIRST_SCREEN_KEY = showQueryKeys.calendar({
  limit: SHOWS_PAGE_SIZE,
  offset: undefined,
  city: undefined,
  state: undefined,
  cities: undefined,
  tags: undefined,
  tagMatch: undefined,
})

export const SHOW_CITIES_FIRST_SCREEN_URL = showEndpoints.CITIES

export const SHOW_CITIES_FIRST_SCREEN_KEY = showQueryKeys.cities()

/**
 * The month histogram behind the pager's page labels.
 *
 * Seeded for the same reason the rows are, and with more at stake than the
 * labels: it is a third call on the site's busiest public read, it is
 * `private, max-age=60` so no shared cache absorbs a repeat, and the anonymous
 * per-IP budget is shared by everyone behind one address. Unseeded, a cold
 * `/shows` view costs three calls instead of two against that budget, which is
 * the shape that surfaces as intermittent "Failed to load" and never reaches
 * Sentry.
 *
 * Filterless, like its siblings, so it is the entry a cold anon visitor asks
 * for. A filtered deep link misses it and fetches for itself, which costs that
 * visitor the labels for a beat and nothing else.
 */
export const SHOWS_MONTHS_FIRST_SCREEN_URL = showEndpoints.MONTHS

export const SHOWS_MONTHS_FIRST_SCREEN_KEY = showQueryKeys.months({
  city: undefined,
  state: undefined,
  cities: undefined,
  tags: undefined,
  tagMatch: undefined,
})
