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

/**
 * The city a selection is ABOUT, as the FACET spells it, or null.
 *
 * ONE definition, read by both sides of the page: `VenueList` names the `<h1>`
 * and the breadcrumb with it, and `generateMetadata` decides the `<title>`, the
 * canonical and the `robots` off the same answer. Two implementations would let
 * the heading and the title disagree about which city the page is, with only
 * one of them guarded.
 *
 * Canonical rather than as-typed, and that is the guard: `?cities=`, `?city=`
 * and `?state=` are free text off the URL and they reach all of the above.
 * Taking the matched row's spelling means a hand-crafted link cannot put
 * arbitrary text into any of them; an unmatched value still filters the list,
 * because the wire contract is shared with every other surface, but the page
 * falls back to naming no city.
 *
 * Matched case-insensitively, so `?cities=phoenix,az` resolves to the facet's
 * spelling rather than falling back. More than one selected city has no single
 * answer, so it is null as well.
 */
export function facetCityFor(
  selected: readonly CityState[],
  facet: readonly { city: string; state: string }[]
): CityState | null {
  if (selected.length !== 1) return null
  const wanted = cityKey(selected[0]).toLowerCase()
  const match = facet.find(c => cityKey(c).toLowerCase() === wanted)
  return match ? { city: match.city, state: match.state } : null
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
 * NOT `suggestAlternativeCities` (features/shows/suggestCities), which answers
 * the same question for `/shows`. That one ranks by haversine when the payload
 * carries centroids and falls back to busiest-only when it does not, and its
 * fallback order is pinned by a test. The venue facet has no centroids, so the
 * distance branch could never run here, and the same-state tier is not a
 * behaviour `/shows` has. Once the venue facet serves centroids the two want to
 * become one function, which is a change to both surfaces rather than to this
 * one.
 *
 * The subject city is excluded: it is the one the reader has already been told
 * is empty. A city with no rooms is excluded too, so every link offered lands
 * somewhere with something on it.
 */
export function nearbyCitiesWithRooms(
  cities: CityWithCount[],
  subject: CityState,
  limit = NEARBY_CITY_COUNT
): CityWithCount[] {
  const excluded = cityKey(subject).toLowerCase()
  return cities
    .filter(c => c.count > 0 && cityKey(c).toLowerCase() !== excluded)
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
