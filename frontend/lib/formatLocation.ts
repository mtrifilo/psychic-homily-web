/**
 * Format a city/state/country triple as a display location string.
 *
 * PSY-558 display rule (locked):
 *  - Falsy parts (null, undefined, empty/whitespace strings) are dropped so
 *    callers never render trailing separators like "Phoenix, ".
 *  - Country is included UNLESS state is set AND country is "USA"/"US" —
 *    local readers parse "Phoenix, AZ" as US-implicit, so adding "USA"
 *    is noise. International artists ("Melbourne, Australia",
 *    "London, England, UK", "Tokyo, Japan") always render the country.
 *  - Country comparison is case-insensitive + trimmed; either spelling
 *    ("USA" / "US") triggers the suppression.
 *  - When every part is missing, returns "Location Unknown" as a stable
 *    placeholder rather than an empty string.
 *
 * Consolidated PSY-780: previously the canonical PSY-558 implementation
 * lived only in `features/artists/types.ts getArtistLocation`. The venue
 * counterpart was a bare `${city}, ${state}` template that left a trailing
 * comma when `state` was empty. Both call sites now delegate here so the
 * rule is enforced uniformly.
 */
/**
 * The placeholder `formatLocation` returns when nothing is placeable.
 * Exported so callers that compose the result INTO a larger line (rather than
 * rendering it as a location field) can recognise it and drop the segment
 * instead of printing the placeholder mid-sentence.
 */
export const LOCATION_UNKNOWN = 'Location Unknown'

/**
 * Whether a free-text country value names the United States.
 *
 * Country columns are written from several sources and are never
 * canonicalized, so the comparison trims and folds case.
 *
 * THE SET IS THE ONE PSY-558's DISPLAY RULE RECOGNIZES, and it is narrower
 * than the backend's: `show_venue_local_sql.go` accepts "United States" as a
 * third spelling and `geo.go` carries a fuller alias map. A value outside this
 * set is not a claim that the place is outside the US, only that this rule
 * does not recognize it as inside.
 */
export function isUnitedStatesCountry(country?: string | null): boolean {
  const value = country?.trim().toUpperCase()
  return value === 'US' || value === 'USA'
}

export function formatLocation(loc: {
  city?: string | null
  state?: string | null
  country?: string | null
}): string {
  const city = nonEmpty(loc.city)
  const state = nonEmpty(loc.state)
  const country = nonEmpty(loc.country)
  const parts = [city, state].filter(Boolean) as string[]
  const countryIsUS = isUnitedStatesCountry(country)
  if (country && !(state && countryIsUS)) {
    parts.push(country)
  }
  return parts.length > 0 ? parts.join(', ') : LOCATION_UNKNOWN
}

function nonEmpty(value: string | null | undefined): string | undefined {
  if (value == null) return undefined
  const trimmed = value.trim()
  return trimmed.length > 0 ? trimmed : undefined
}
