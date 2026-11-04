const LEADING_CALENDAR_DATE = /^(\d{4}-\d{2}-\d{2})(?:T|$)/

/**
 * The `YYYY-MM-DD` an episode URL (`/radio/{station}/{show}/{date}`) is keyed
 * on, from a date-only string or an RFC 3339 timestamp; null when the value is
 * absent or in neither form.
 *
 * Returns the date as written and never re-zones it through `Date`: a station
 * air date sent as a midnight UTC timestamp would otherwise land on the
 * previous day in any zone west of UTC.
 */
export function toEpisodeDate(value: string | null | undefined): string | null {
  return LEADING_CALENDAR_DATE.exec(value ?? '')?.[1] ?? null
}
