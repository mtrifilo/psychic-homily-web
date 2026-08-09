import {
  getTimezoneForState,
  hasTimezoneForState,
  formatDateInTimezone,
  formatDateWithYearInTimezone,
  formatTimeInTimezone,
  formatInTimezone,
} from './timeUtils'

/**
 * Resolve the IANA timezone for rendering a show time. Prefers the venue's
 * resolved `timezone` (PSY-985); falls back to the US state→tz map for venues
 * without one (pre-backfill rows). A malformed/unknown `timezone` string falls
 * through to the state map rather than crashing the render (`Intl` throws a
 * RangeError on a bad zone), mirroring the backend's EventLocation (PSY-996/986).
 */
export function resolveShowTimezone(
  state?: string | null,
  timezone?: string | null
): string {
  if (timezone && isValidTimeZone(timezone)) return timezone
  return getTimezoneForState(state || 'AZ')
}

/**
 * Whether `resolveShowTimezone` would return a zone it actually KNOWS for this
 * show, rather than the America/Phoenix default it falls back to for anything
 * outside the US state map.
 *
 * Ask this before rendering a clock time or a same-day claim ("tonight") for a
 * venue that may not have a resolved `timezone`: the default is a guess, and a
 * guess that is hours or a calendar day wrong reads as fact once it is printed
 * next to a venue name. Formatting a date is a weaker claim and can live with
 * the fallback; naming an hour cannot.
 */
export function isShowTimezoneResolved(
  state?: string | null,
  timezone?: string | null
): boolean {
  return (!!timezone && isValidTimeZone(timezone)) || hasTimezoneForState(state)
}

/**
 * Whether an IANA zone name exists, memoized.
 *
 * The answer is a property of the string and of the runtime's tz database, so
 * it can never change within a session. The probe is not free — constructing an
 * `Intl.DateTimeFormat` costs ~20µs, and this sits on the path every single
 * date and time on an entity page takes, several times per row. A dense table
 * of 50 shows asks the same question 150 times about the same venue.
 */
const timeZoneValidity = new Map<string, boolean>()

function isValidTimeZone(tz: string): boolean {
  const known = timeZoneValidity.get(tz)
  if (known !== undefined) return known
  let valid: boolean
  try {
    // Throws RangeError for an unknown/malformed IANA name.
    new Intl.DateTimeFormat('en-US', { timeZone: tz })
    valid = true
  } catch {
    valid = false
  }
  timeZoneValidity.set(tz, valid)
  return valid
}

/**
 * Format a show date in the venue's timezone: "Mon, Dec 1" or "Mon Dec 1, 2026".
 * Pass the venue's `timezone` when available; `state` is the fallback.
 */
export function formatShowDate(
  dateString: string,
  state?: string | null,
  includeYear = false,
  timezone?: string | null
): string {
  const tz = resolveShowTimezone(state, timezone)
  return includeYear
    ? formatDateWithYearInTimezone(dateString, tz)
    : formatDateInTimezone(dateString, tz)
}

/** Format only the venue-local short weekday, e.g. "Fri". */
export function formatShowWeekday(
  dateString: string,
  state?: string | null,
  timezone?: string | null,
): string {
  return formatInTimezone(dateString, resolveShowTimezone(state, timezone), {
    weekday: 'short',
  })
}

/**
 * The venue-local month and year of a show, kept apart: `{ month: 'Sep', year:
 * '2025' }`.
 *
 * Callers that need to compare or recombine the halves take them from here
 * rather than splitting {@link formatShowMonth}'s output, so no caller has to
 * assume where the year sits inside a formatted string.
 */
export function formatShowMonthParts(
  dateString: string,
  state?: string | null,
  timezone?: string | null
): { month: string; year: string } {
  const tz = resolveShowTimezone(state, timezone)
  return {
    month: formatInTimezone(dateString, tz, { month: 'short' }),
    year: formatInTimezone(dateString, tz, { year: 'numeric' }),
  }
}

/**
 * Format the venue-local month and year of a show: "Sep 2025".
 *
 * Doubles as the grouping KEY for month-grouped show lists, which is why it is
 * one function rather than a formatter plus a separate key builder: two months
 * share a heading exactly when they share this string, with no chance of the
 * label and the key disagreeing about which timezone decided the boundary.
 */
export function formatShowMonth(
  dateString: string,
  state?: string | null,
  timezone?: string | null
): string {
  return formatInTimezone(dateString, resolveShowTimezone(state, timezone), {
    month: 'short',
    year: 'numeric',
  })
}

/**
 * Format a show time in the venue's timezone: "7:30 PM".
 * Pass the venue's `timezone` when available; `state` is the fallback.
 */
export function formatShowTime(
  dateString: string,
  state?: string | null,
  timezone?: string | null
): string {
  return formatTimeInTimezone(dateString, resolveShowTimezone(state, timezone))
}

/**
 * Format price for display. Shows "Free" for $0, otherwise "$XX.XX".
 */
export function formatPrice(price: number): string {
  if (price === 0) return 'Free'
  return `$${price.toFixed(2)}`
}

/**
 * Format a content date: "January 15, 2026"
 */
export function formatContentDate(dateString: string): string {
  const date = new Date(dateString)
  return date.toLocaleDateString('en-US', {
    year: 'numeric',
    month: 'long',
    day: 'numeric',
  })
}

/**
 * Format an admin date with weekday: "Sat, Jan 15, 2026"
 */
export function formatAdminDate(dateString: string): string {
  const date = new Date(dateString)
  return date.toLocaleDateString('en-US', {
    weekday: 'short',
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  })
}

/**
 * Format time in browser local timezone: "7:30 PM"
 */
export function formatAdminTime(dateString: string): string {
  const date = new Date(dateString)
  return date.toLocaleTimeString('en-US', {
    hour: 'numeric',
    minute: '2-digit',
    hour12: true,
  })
}

/**
 * Format a short date without weekday: "Jan 15, 2026"
 */
export function formatShortDate(dateString: string): string {
  const date = new Date(dateString)
  return date.toLocaleDateString('en-US', {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  })
}

/**
 * Format a timestamp with date and time: "Jan 15, 2026, 7:30 PM"
 */
export function formatTimestamp(dateString: string): string {
  const date = new Date(dateString)
  return date.toLocaleDateString('en-US', {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
  })
}
