/**
 * URL shape and vocabulary for the `/venues` directory's own navigation.
 *
 * Pure: no React, no router, no fetching, so the rules that decide what a page
 * link says and which orders are addressable can be tested without a rendered
 * table.
 */

import { buildCitiesParam, cityKey } from '@/components/filters/cityParamsFormat'
import type { CityState, CityWithCount } from '@/components/filters'
import { formatCount, listPageHref } from '@/components/shared/paginationChrome'
import { VENUE_LIST_PAGE_LIMIT } from './api'

/** The directory's root path. Every href here is built from it. */
export const VENUES_ROOT = '/venues'

/**
 * Rows per page on `/venues`, re-exported from the module that builds the query
 * key: the number that sizes a page and the number the key records are one
 * value, so they are declared once.
 */
export const VENUES_PAGE_SIZE = VENUE_LIST_PAGE_LIMIT

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

/** The href for a 1-based page of the directory, from the params on screen. */
export function venuesPageHref(
  params: { toString: () => string },
  page: number
): string {
  return listPageHref(params, page, VENUES_ROOT)
}

/**
 * The directory scoped to one city, as a shareable address, built FROM the
 * params already on screen.
 *
 * Carrying them is the same rule the pager follows: a chosen order or tag
 * filter is part of the question the reader is asking, and only the city is
 * being answered here. `page` goes, because a different city is answered from
 * its first page.
 *
 * The city itself goes through `buildCitiesParam`, the single source of truth
 * for the `?cities=` wire format, so a chip here and a pick in the city filter
 * address the same page.
 */
export function venuesCityHref(
  params: { toString: () => string },
  city: string,
  state: string
): string {
  const next = new URLSearchParams(params.toString())
  next.delete('page')
  next.delete('city')
  next.delete('state')
  next.set('cities', buildCitiesParam([{ city, state }]))
  return `${VENUES_ROOT}?${next.toString()}`
}

/** How many other cities a city with no rooms offers as somewhere to go. */
export const NEARBY_CITY_COUNT = 5

/**
 * The cities a reader is sent to when the one they asked for has no rooms.
 *
 * "NEAREST" IS DEFINED AS SAME STATE FIRST, THEN BUSIEST, and that is a
 * consequence of the data rather than a preference: `/venues/cities` serves a
 * city name, a state and a count, and no coordinates, so no distance can be
 * computed here. Sharing a state is the only proximity signal the payload
 * carries. Ties break on the city name so the list is stable between renders.
 *
 * The subject city is excluded: it is the one the reader has already been told
 * is empty.
 */
export function nearbyCitiesWithRooms(
  cities: CityWithCount[],
  subject: CityState,
  limit = NEARBY_CITY_COUNT
): CityWithCount[] {
  const excluded = cityKey(subject).toLowerCase()
  return cities
    .filter(c => cityKey(c).toLowerCase() !== excluded)
    .sort((a, b) => {
      const aHome = a.state === subject.state ? 0 : 1
      const bHome = b.state === subject.state ? 0 : 1
      return (
        aHome - bHome || b.count - a.count || a.city.localeCompare(b.city)
      )
    })
    .slice(0, limit)
}

/**
 * A count beside the noun it counts, grouped for reading, so no bare number is
 * left for the reader to name. The plural is the singular plus "s" unless the
 * caller says otherwise.
 */
export function countLabel(
  count: number,
  singular: string,
  plural = `${singular}s`
): string {
  return `${formatCount(count)} ${count === 1 ? singular : plural}`
}
