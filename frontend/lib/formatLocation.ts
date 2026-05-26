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
export function formatLocation(loc: {
  city?: string | null
  state?: string | null
  country?: string | null
}): string {
  const city = nonEmpty(loc.city)
  const state = nonEmpty(loc.state)
  const country = nonEmpty(loc.country)
  const parts = [city, state].filter(Boolean) as string[]
  const countryIsUS =
    country?.toUpperCase() === 'USA' || country?.toUpperCase() === 'US'
  if (country && !(state && countryIsUS)) {
    parts.push(country)
  }
  return parts.length > 0 ? parts.join(', ') : 'Location Unknown'
}

function nonEmpty(value: string | null | undefined): string | undefined {
  if (value == null) return undefined
  const trimmed = value.trim()
  return trimmed.length > 0 ? trimmed : undefined
}
