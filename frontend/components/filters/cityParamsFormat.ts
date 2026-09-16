import type { CityState } from './CityFilters'

/**
 * The `?cities=` WIRE FORMAT, with no runtime dependency on nuqs.
 *
 * Split out of `cityParams.ts` so a SERVER module can read the format: that
 * file declares the nuqs parser, `createParser` is a client export, and
 * importing it from `generateMetadata` fails the build with "attempted to call
 * createParser() from the server". Every helper here is pure, so both sides
 * share one definition of the format rather than keeping two that can drift.
 *
 * `cityParams.ts` re-exports all of this, so the client surface is unchanged
 * and either import path is correct.
 *
 * Wire format: `Phoenix,AZ|Mesa,AZ` — comma between city/state, pipe between
 * pairs; each segment must be exactly city,state, matching the /explore backend
 * parser in handlers/explore/explore.go.
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

/**
 * Identity of a city within a selection. Every surface that compares, dedupes
 * or keys a selection uses this one rule, so the popover, the bottom sheet and
 * `citiesEqual` cannot drift into disagreeing about what "the same city" means.
 */
export function cityKey(c: CityState): string {
  return `${c.city}|${c.state}`
}

/** A city as it reads on screen. */
export function cityLabel(c: CityState): string {
  return `${c.city}, ${c.state}`
}

/** Order-insensitive equality of two city selections. */
export function citiesEqual(a: CityState[], b: CityState[]): boolean {
  if (a.length !== b.length) return false
  const setA = new Set(a.map(cityKey))
  return b.every(c => setA.has(cityKey(c)))
}

/**
 * Explicit "All Cities" sentinel. `?cities=all` means the user deliberately
 * chose to see every city, which is DISTINCT from an absent `cities` param
 * (which means "apply my default" — the favorite city, or the anon geo
 * default, derived during render). Disambiguating those two is what lets the
 * default be derived instead of seeded into the URL by an effect.
 */
export const ALL_CITIES = 'all'
export type CitiesFilter = CityState[] | typeof ALL_CITIES
