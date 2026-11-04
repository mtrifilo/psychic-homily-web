const LEADING_CALENDAR_DATE = /^(\d{4}-\d{2}-\d{2})(?:T|$)/

/**
 * The `YYYY-MM-DD` an episode URL (`/radio/{station}/{show}/{date}`) is keyed
 * on, from a date-only string or an RFC 3339 timestamp; null when the value is
 * absent or in neither form.
 *
 * Reads the leading date text and never parses through `Date`: the API sends a
 * Postgres `date` as UTC midnight (`2025-01-15T00:00:00Z`), which any zone west
 * of UTC would turn into the previous day.
 */
export function toEpisodeDate(value: string | null | undefined): string | null {
  const match = value ? LEADING_CALENDAR_DATE.exec(value) : null
  return match ? match[1] : null
}
