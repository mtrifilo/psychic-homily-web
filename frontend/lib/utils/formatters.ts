import {
  FALLBACK_SHOW_TIMEZONE,
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
 * without one (pre-backfill rows), and finally to
 * `FALLBACK_SHOW_TIMEZONE` (`./timeUtils`) when the state is blank or non-US. A
 * malformed/unknown `timezone` string falls through to the state map rather
 * than crashing the render (`Intl` throws a RangeError on a bad zone),
 * mirroring the backend's EventLocation (PSY-996/986).
 *
 * THE LAST STEP IS NOT A DISPLAY DEFAULT. It is the read half of a round trip
 * with the submit path, and swapping it here alone would move every rendered
 * clock off its stored instant. `FALLBACK_SHOW_TIMEZONE` (`./timeUtils`) holds
 * the whole invariant; read it before touching either end.
 */
export function resolveShowTimezone(
  state?: string | null,
  timezone?: string | null
): string {
  if (timezone && isValidTimeZone(timezone)) return timezone
  // Spelled as an early return rather than folded into the map lookup so this
  // function — the one all the policy docs point readers at — actually names
  // the constant that governs it, and so a grep for FALLBACK_SHOW_TIMEZONE
  // finds the chain's last rung instead of only its definition.
  if (!state) return FALLBACK_SHOW_TIMEZONE
  return getTimezoneForState(state)
}

/**
 * Whether `resolveShowTimezone` would return a zone it actually KNOWS for this
 * show, rather than `FALLBACK_SHOW_TIMEZONE` (`./timeUtils`), which it falls
 * back to for anything outside the US state map.
 *
 * Ask this before rendering a clock time or a same-day claim ("tonight") for a
 * venue that may not have a resolved `timezone`: the default is a guess, and a
 * guess that is hours or a calendar day wrong reads as fact once it is printed
 * next to a venue name. Formatting a date is a weaker claim and can live with
 * the fallback; naming an hour cannot.
 *
 * That split is the show page's policy, not a per-surface preference. Wave 1A
 * (PSY-1684) wrote the refusing half into `showStatusStripeCopy` — no DOORS, no
 * MUSIC, no TONIGHT on a guessed zone — and Wave 1C (PSY-1686) extended it to
 * the start time and the venue module's times line. The accepting half is every
 * date render on that page, which prints the fallback's calendar day rather
 * than printing nothing. `FALLBACK_SHOW_TIMEZONE` (`./timeUtils`) carries why
 * that day is the best available answer instead of an arbitrary one, and which
 * surfaces do NOT yet ask this question.
 */
export function isShowTimezoneResolved(
  state?: string | null,
  timezone?: string | null
): boolean {
  return (!!timezone && isValidTimeZone(timezone)) || hasTimezoneForState(state)
}

/** IANA zone name -> does it exist. See {@link isValidTimeZone}. */
const timeZoneValidity = new Map<string, boolean>()

/**
 * Whether an IANA zone name exists, memoized.
 *
 * The answer is a property of the string and of the runtime's tz database, so
 * it can never change within a session, and the key domain is bounded by the
 * zones the venue table holds. The probe is not free — constructing an
 * `Intl.DateTimeFormat` costs ~20us, and this sits on the path every single
 * date and time on an entity page takes, several times per row. A dense table
 * of 50 shows asks the same question 150 times about the same venue.
 *
 * Exported because it is a genuinely different question from
 * `isShowTimezoneResolved`, which answers "can this ROW be given a zone at all"
 * and says yes on the strength of the state map alone. A caller that wants to
 * trust the venue's OWN zone string (because it is about to bucket rows by
 * calendar date in it) must ask about the string, not about the row.
 */
export function isValidTimeZone(tz: string): boolean {
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
 * Pinned for the same reason every other formatter here pins `en-US`: these
 * strings render on the server and again on the client, and a runtime that
 * abbreviated months differently would hydrate into a mismatch.
 */
const calendarMonthFormatter = new Intl.DateTimeFormat('en-US', {
  month: 'short',
  timeZone: 'UTC',
})

/**
 * The same `{ month: 'Sep', year: '2025' }` shape as {@link
 * formatShowMonthParts}, from a calendar month someone has ALREADY placed —
 * a `(year, month)` pair out of a server-side histogram rather than an instant
 * that still has to be read on some calendar.
 *
 * There is deliberately no timezone parameter. A caller here is not asking
 * "which month did this instant fall in", which is the question a zone answers;
 * it is naming a month that is already decided. Accepting a zone would invite
 * shifting an already-bucketed month by an offset and landing a whole page of
 * September rows under "Aug".
 *
 * `month` is 1-12, matching the wire format and `EXTRACT(MONTH …)`, NOT
 * JavaScript's 0-based month. Out-of-range values roll over the way `Date` does;
 * callers reading untrusted input should reject them first.
 */
export function formatCalendarMonthParts(
  year: number,
  month: number
): { month: string; year: string } {
  // The caller's year never reaches the formatter — it is returned verbatim, and
  // the Date exists only to name the MONTH. So the reference year is a fixed
  // arbitrary one, which keeps every quirk of building a Date from caller input
  // (the 0-99 remap `Date.UTC` applies, calendar-boundary drift) out of reach.
  // Mid-month and read back in UTC, so no offset or DST rule can move it either.
  const instant = new Date(Date.UTC(2000, month - 1, 15))
  return { month: calendarMonthFormatter.format(instant), year: String(year) }
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
