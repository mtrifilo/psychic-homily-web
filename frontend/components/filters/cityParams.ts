import type { CityState } from './CityFilters'

/**
 * Shared `?cities=` URL-param helpers, co-located with the `CityState`
 * type + `CityFilters` component they serve (PSY-840).
 *
 * Wire format: `Phoenix,AZ|Mesa,AZ` — comma between city/state, pipe
 * between pairs; each segment must be exactly city,state, matching the
 * /explore backend parser in handlers/explore/explore.go.
 *
 * Duplication: ShowList, VenueList, and ArtistList keep local
 * parse+build copies; useShows keeps a local buildCitiesParam;
 * HomeShowList keeps a local citiesEqual (it holds selection in
 * component state, not the URL). PSY-928 tracks migrating all of them
 * onto these shared exports so the wire format has one definition.
 */

/** Parse the `?cities=` param ("Phoenix,AZ|Mesa,AZ") into typed pairs.
 * Each segment must be exactly city,state — segments with extra commas
 * or a blank half are dropped, matching the /explore backend parser. */
export function parseCitiesParam(param: string | null | undefined): CityState[] {
  if (!param) return []
  return param
    .split('|')
    .map(pair => {
      const parts = pair.split(',')
      if (parts.length !== 2) return null
      const city = parts[0].trim()
      const state = parts[1].trim()
      return city && state ? { city, state } : null
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
