import { createParser } from 'nuqs'
import type { CityState } from './CityFilters'

/**
 * Shared `?cities=` URL-param helpers, co-located with the `CityState`
 * type + `CityFilters` component they serve (PSY-840).
 *
 * Wire format: `Phoenix,AZ|Mesa,AZ` — comma between city/state, pipe
 * between pairs; each segment must be exactly city,state, matching the
 * /explore backend parser in handlers/explore/explore.go.
 *
 * Single source of truth for the `?cities=` wire format (PSY-928):
 * every surface that reads or writes the param — the shows/venues/
 * artists list components and their data hooks, HomeShowList, and
 * /explore's UpcomingShowsList — imports these, so a format change
 * lands in exactly one place. The CityState type has a single source
 * too (CityFilters.tsx).
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

/**
 * Explicit "All Cities" sentinel. `?cities=all` means the user deliberately
 * chose to see every city, which is DISTINCT from an absent `cities` param
 * (which means "apply my default" — the favorite city, or the anon geo
 * default, derived during render). Disambiguating those two is what lets the
 * default be derived instead of seeded into the URL by an effect.
 */
export const ALL_CITIES = 'all'
export type CitiesFilter = CityState[] | typeof ALL_CITIES

/**
 * nuqs parser for the `?cities=` param, wrapping the wire-format helpers above
 * so the `Phoenix,AZ|Mesa,AZ` format stays the single source of truth.
 *
 * State model (mirrors the three cases the surfaces derive from):
 *   - `null`          → param absent → caller applies its default (favorites/geo)
 *   - `ALL_CITIES`    → `?cities=all` → explicit "all cities"
 *   - `CityState[]`   → `?cities=Phoenix,AZ|…` → explicit selection
 *
 * A present-but-unparseable value (empty or all-malformed segments) resolves to
 * `null` so it falls back to the default rather than rendering an empty filter.
 */
export const citiesParser = createParser<CitiesFilter>({
  parse(value) {
    if (value === ALL_CITIES) return ALL_CITIES
    const cities = parseCitiesParam(value)
    return cities.length > 0 ? cities : null
  },
  serialize(value) {
    if (value === ALL_CITIES) return ALL_CITIES
    return buildCitiesParam(value)
  },
  eq(a, b) {
    if (a === ALL_CITIES || b === ALL_CITIES) return a === b
    return citiesEqual(a, b)
  },
})
