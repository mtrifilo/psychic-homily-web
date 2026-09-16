/**
 * URL shape and vocabulary for the `/venues` directory's own navigation.
 *
 * Pure: no React, no router, no fetching, so the rules that decide what a page
 * link says and which orders are addressable can be tested without a rendered
 * table.
 */

/** The directory's root path. Every href here is built from it. */
export const VENUES_ROOT = '/venues'

/**
 * Rows per page on `/venues`.
 *
 * Stated rather than inherited. The backend's `default:"50"` is the same
 * number, but the pager's arithmetic (which row ordinal falls on which page)
 * has to agree with the limit the request actually carried, so the request
 * sends this explicitly.
 */
export const VENUES_PAGE_SIZE = 50

/**
 * The accepted `?sort=` values, in the order the sort control offers them.
 *
 * The same vocabulary the API accepts (`contracts.VenueListSortValues`), so a
 * value read off the URL is sent on the wire unchanged and an unrecognized one
 * is dropped here rather than turned into a 422.
 */
export const VENUE_SORTS = ['upcoming', 'name', 'next'] as const

export type VenueSort = (typeof VENUE_SORTS)[number]

/**
 * The order applied when `?sort=` is absent, and the one value the URL never
 * carries: writing it would give one row order two addresses.
 */
export const DEFAULT_VENUE_SORT: VenueSort = 'upcoming'

/** What each order is called in the sort control. */
export const VENUE_SORT_LABELS: Record<VenueSort, string> = {
  upcoming: 'Upcoming shows',
  name: 'Name',
  next: 'Next show',
}

/**
 * The href for a 1-based page of the directory, built FROM the params already
 * on screen.
 *
 * Every key but `page` is carried through untouched: the list shares its query
 * string with the city filter, the tag filter, the sort order and whatever a
 * campaign link brought along, and a pager that minted a fresh
 * `URLSearchParams` would silently drop all of it on every page click.
 *
 * Page 1 writes NO `page`, so the first page of any filter state has exactly
 * one address and the root is reachable by paging back.
 */
export function venuesPageHref(
  params: URLSearchParams | { toString: () => string },
  page: number
): string {
  const next = new URLSearchParams(params.toString())
  if (page > 1) next.set('page', String(page))
  else next.delete('page')
  const query = next.toString()
  return query ? `${VENUES_ROOT}?${query}` : VENUES_ROOT
}

/**
 * The directory scoped to one city, as a shareable address.
 *
 * The wire format is the shared `?cities=` one (`City,ST`), so a chip here and
 * a pick in the city filter address the same page.
 */
export function venuesCityHref(city: string, state: string): string {
  const params = new URLSearchParams({ cities: `${city},${state}` })
  return `${VENUES_ROOT}?${params.toString()}`
}
