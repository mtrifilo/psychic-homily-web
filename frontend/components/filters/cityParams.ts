import type { CityState } from './CityFilters'

/**
 * Shared `?cities=` URL-param helpers, co-located with the `CityState`
 * type + `CityFilters` component they serve (PSY-840). Several surfaces
 * still carry local copies of some of these — ShowList, VenueList,
 * ArtistList, HomeShowList, and useShows (buildCitiesParam) — so the
 * pipe-delimited wire format has 6 definitions today. A follow-up
 * (filed alongside PSY-840) should migrate all of them onto these shared
 * exports so the format has a single definition.
 *
 * Wire format: `Phoenix,AZ|Mesa,AZ` — comma between city/state, pipe
 * between pairs. Mirrors the backend parser (handlers/explore,
 * handlers/catalog/show).
 */

/** Parse the `?cities=` param ("Phoenix,AZ|Mesa,AZ") into typed pairs. */
export function parseCitiesParam(param: string | null | undefined): CityState[] {
  if (!param) return []
  return param
    .split('|')
    .map(pair => {
      const [city, state] = pair.split(',')
      return city && state ? { city: city.trim(), state: state.trim() } : null
    })
    .filter((c): c is CityState => c !== null)
}

/** Serialize a city selection into the `?cities=` param. */
export function buildCitiesParam(cities: CityState[]): string {
  return cities.map(c => `${c.city},${c.state}`).join('|')
}

/** Order-insensitive equality of two city selections. */
export function citiesEqual(a: CityState[], b: CityState[]): boolean {
  if (a.length !== b.length) return false
  const setA = new Set(a.map(c => `${c.city}|${c.state}`))
  return b.every(c => setA.has(`${c.city}|${c.state}`))
}
