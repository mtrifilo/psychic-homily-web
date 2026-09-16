import { createParser } from 'nuqs'
import {
  ALL_CITIES,
  buildCitiesParam,
  citiesEqual,
  parseCitiesParam,
  type CitiesFilter,
} from './cityParamsFormat'

/**
 * Shared `?cities=` URL-param helpers, co-located with the `CityState`
 * type + `CityFilters` component they serve (PSY-840).
 *
 * Single source of truth for the `?cities=` wire format (PSY-928):
 * every surface that reads or writes the param — the shows/venues/
 * artists list components and their data hooks, HomeShowList, and
 * /explore's UpcomingShowsList — imports these, so a format change
 * lands in exactly one place. The CityState type has a single source
 * too (CityFilters.tsx).
 *
 * The FORMAT itself lives in ./cityParamsFormat, which carries no nuqs import
 * and so can be read from the server; it is re-exported here so every existing
 * import path keeps working.
 */
export {
  ALL_CITIES,
  buildCitiesParam,
  citiesEqual,
  cityKey,
  cityLabel,
  parseCitiesParam,
  type CitiesFilter,
} from './cityParamsFormat'

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
