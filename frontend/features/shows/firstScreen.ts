/**
 * Which cache entries a set of server-fetched payloads may seed on a shows
 * list route, or `null` when the page must render unseeded.
 *
 * The GATING RULE lives here rather than inline in a route so it can be stated
 * once and tested: `ShowList` returns its skeleton while EITHER the rows or the
 * cities are still loading, so seeding one without the other server-renders the
 * skeleton and buys nothing. The month histogram is NOT a gate, because the
 * list renders with bare page numerals without it, so a missing one is dropped
 * from the seed rather than suppressing the other two.
 *
 * Shared by the root list and the date-addressed ones, which differ only in
 * WHICH calendar entry the rows land on — hence `calendarKey`.
 */

import {
  SHOW_CITIES_FIRST_SCREEN_KEY,
  SHOWS_MONTHS_FIRST_SCREEN_KEY,
} from './api'
import type {
  ShowCitiesResponse,
  ShowMonthsResponse,
  ShowsCalendarResponse,
} from './types'

export interface ShowsFirstScreenPayloads {
  shows: ShowsCalendarResponse | null
  cities: ShowCitiesResponse | null
  months: ShowMonthsResponse | null
  /**
   * The cache entry the rows belong to. Required rather than defaulted to the
   * root's: a windowed route seeding the root's key would look like a hit for a
   * list it does not describe, and a default is exactly how that mistake gets
   * made silently.
   */
  calendarKey: readonly unknown[]
}

export function showsFirstScreenSeeds({
  shows,
  cities,
  months,
  calendarKey,
}: ShowsFirstScreenPayloads): Array<{
  queryKey: readonly unknown[]
  data: unknown
}> | null {
  if (!shows || !cities) return null

  return [
    { queryKey: calendarKey, data: shows },
    { queryKey: SHOW_CITIES_FIRST_SCREEN_KEY, data: cities },
    ...(months
      ? [{ queryKey: SHOWS_MONTHS_FIRST_SCREEN_KEY, data: months }]
      : []),
  ]
}
