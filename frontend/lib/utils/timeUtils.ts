/**
 * Time utility functions for handling date/time conversions
 */

/**
 * Map of US state codes to their IANA timezones. This is the FALLBACK used only
 * when a venue has no resolved `timezone` (e.g. rows created before PSY-985's
 * geocoding, until the PSY-987 backfill runs). New code should prefer
 * `venue.timezone`; this map only narrows the guess for US venues.
 *
 * Kept in sync with the CLI's map in cli/src/lib/timezone.ts — both must list
 * the same states. (PSY-986 fixed the prior drift: this map had only 7 states,
 * so non-listed US states like IL/WA/MN/MA silently fell back to Arizona time.)
 */
const STATE_TIMEZONES: Record<string, string> = {
  AZ: 'America/Phoenix',
  CA: 'America/Los_Angeles',
  NV: 'America/Los_Angeles',
  CO: 'America/Denver',
  NM: 'America/Denver',
  TX: 'America/Chicago',
  NY: 'America/New_York',
  // Eastern
  CT: 'America/New_York',
  DC: 'America/New_York',
  DE: 'America/New_York',
  FL: 'America/New_York',
  GA: 'America/New_York',
  MA: 'America/New_York',
  MD: 'America/New_York',
  ME: 'America/New_York',
  MI: 'America/New_York',
  NC: 'America/New_York',
  NH: 'America/New_York',
  NJ: 'America/New_York',
  OH: 'America/New_York',
  PA: 'America/New_York',
  RI: 'America/New_York',
  SC: 'America/New_York',
  VA: 'America/New_York',
  VT: 'America/New_York',
  WV: 'America/New_York',
  // Central
  AL: 'America/Chicago',
  AR: 'America/Chicago',
  IA: 'America/Chicago',
  IL: 'America/Chicago',
  IN: 'America/Indiana/Indianapolis',
  KS: 'America/Chicago',
  KY: 'America/New_York',
  LA: 'America/Chicago',
  MN: 'America/Chicago',
  MO: 'America/Chicago',
  MS: 'America/Chicago',
  ND: 'America/Chicago',
  NE: 'America/Chicago',
  OK: 'America/Chicago',
  SD: 'America/Chicago',
  TN: 'America/Chicago',
  WI: 'America/Chicago',
  // Mountain
  ID: 'America/Boise',
  MT: 'America/Denver',
  UT: 'America/Denver',
  WY: 'America/Denver',
  // Pacific
  OR: 'America/Los_Angeles',
  WA: 'America/Los_Angeles',
  // Non-contiguous
  AK: 'America/Anchorage',
  HI: 'Pacific/Honolulu',
}

/**
 * The zone a show is read on when NOTHING is known about where it happens:
 * no `venues.timezone`, and a `state` this map does not list (blank, or any
 * non-US region).
 *
 * The one place this value exists IN THE FRONTEND. It used to be spelled twice
 * even here — this literal, and again as `state || 'AZ'` inside
 * `resolveShowTimezone` — so "the default" was two literals in two files that
 * happened to agree.
 *
 * IT IS NOT THE ONLY COPY IN THE REPO, and the others are WRITERS, which is why
 * this is not a value you can change alone. The same fallback is hardcoded as
 * the `||` default of `getTimezoneForState` in `cli/src/lib/timezone.ts` and of
 * `utils.GetTimezoneForState` in `backend/internal/utils/timezone.go` — the
 * DEFAULT, not an entry in the state map, so syncing "the map" does not touch
 * either. Both compose instants: `ph submit-show` through
 * `resolveVenueTimezone`, and the backend when it anchors a date-only show via
 * `utils.EventLocation`. `catalog.backfillTimezones` uses it a third way, to
 * INFER the zone historical rows were written under. PSY-1915 tracks the
 * remaining state-map anchoring on the backend.
 *
 * (Symbols, not line numbers, on purpose: this is the stop sign in front of a
 * change that can move stored instants, and a stop sign pointing at the wrong
 * line is worse than one pointing at a name. Nothing in CI pins them.)
 *
 * `shared.showVenueLocalTimezoneSQL` is NOT a fourth copy, and the difference
 * matters: its CASE reaches this fallback only when the venue's country is
 * null, blank, or US/USA/UNITED STATES, and every other country falls to
 * `ELSE 'UTC'`. So for a Berlin venue with `country = 'DE'` and no stored
 * `timezone` — the exact population this constant exists for — the backend
 * buckets the show's venue-local day in UTC while this file renders it in
 * Arizona, up to a calendar day apart. That divergence is real and untracked;
 * do not assume a sweep of "the constant" reconciles it.
 *
 * DO NOT change it to UTC, to the reader's zone, or to anything else without
 * changing every writer in the same commit. This is not a display guess: it is
 * one half of a matched pair. `ShowForm`'s submit composes `event_date` with
 * `combineDateTimeToUTC(date, time, resolveShowTimezone(...))`, and
 * `showToFormValues` reads it back through the same resolver (PSY-1873). For a
 * show written through the app with no resolvable zone, the stored instant
 * therefore MEANS "this wall clock, read in America/Phoenix", and rendering it
 * here reproduces exactly what the submitter typed. Swapping this constant for
 * a more "honest" zone would keep every stored instant where it is and shift
 * every rendered clock off it — showing a time nobody entered. Changing it on
 * the read side while a writer still uses the old value is the corruption class
 * `backend/internal/utils/timezone.go` warns about at the top of the file.
 *
 * Arizona rather than UTC for the one case this genuinely guesses at (a US
 * show whose state never reached us): a North American evening crosses UTC
 * midnight, so reading such an instant in UTC lands it on the WRONG calendar
 * day, while a UTC-7 zone with no DST lands it on the right one.
 *
 * It is still a guess for anything outside the US, and wrong by up to a
 * calendar day on a date and by hours on a clock. `hasTimezoneForState` (and
 * `isShowTimezoneResolved`, which wraps it) is how a caller tells a known zone
 * from this one.
 *
 * WHERE THAT IS ENFORCED TODAY, stated narrowly because it is not a site-wide
 * rule and reads as one if you squint. The SHOW PAGE asks first and prints no
 * CLOCK when the answer is no: Wave 1A (PSY-1684) put the gate on the stripe,
 * which drops DOORS, MUSIC and TONIGHT; Wave 1C (PSY-1686) extended it to
 * `startTimeFactSegment` and `doorsMusicFactSegment` in
 * `features/shows/components/showStatusStripeCopy.ts`, which both return null.
 *
 * Three things on that same page are NOT gated, and each is a separate ticket
 * because each needs a different decision:
 * - The DATE renders through the guess unmarked, here and in the stripe, plus
 *   two hand-rolled copies in `app/shows/[slug]/page.tsx` (meta description)
 *   and `app/shows/[slug]/opengraph-image.tsx` (share card). Whether to mark it
 *   is PSY-1964.
 * - `getShowLifecycleState` (`./showTiming`) resolves through here with no gate,
 *   so a zone-less venue east of Phoenix keeps ON SALE and [Buy Tickets] live
 *   past its own midnight — about nine hours for Berlin. See PSY-1963.
 * - `formatShowTime` has no gate at all, so its 11 listing call sites (ShowCard,
 *   CompactShowRow, ShowSubmissionsConsole, the artist/venue show tables,
 *   library, the scene panels, sceneDay) and `MusicEvent.startDate` in
 *   `lib/seo/jsonld.ts` name an hour on this guess. Also PSY-1963.
 */
export const FALLBACK_SHOW_TIMEZONE = 'America/Phoenix'

/**
 * Get the IANA timezone for a US state. Falls back to
 * {@link FALLBACK_SHOW_TIMEZONE} for unknown/international input — callers
 * should prefer a venue's resolved `timezone` and use this only as a fallback.
 */
export function getTimezoneForState(state: string): string {
  return STATE_TIMEZONES[state.toUpperCase()] || FALLBACK_SHOW_TIMEZONE
}

/**
 * Whether the map actually knows this state, as opposed to handing back the
 * Arizona default.
 *
 * The default is silent by design: every caller needs SOME zone, and a
 * plausible one beats a crash. But a caller that is about to print a clock time
 * or the word "tonight" is making a claim, and a claim built on the default is
 * wrong by up to most of a day for a venue outside the US. This lets such a
 * caller tell the two apart and say less instead of saying it confidently.
 */
export function hasTimezoneForState(state?: string | null): boolean {
  return !!state && state.toUpperCase() in STATE_TIMEZONES
}

/**
 * Combines a date string and time string into a UTC ISO timestamp
 * Treats the input as local time in the specified timezone
 *
 * @param dateString - Date in YYYY-MM-DD format (from date input)
 * @param timeString - Time in HH:MM format (from time input)
 * @param timezone - IANA timezone (e.g., 'America/Phoenix'). Defaults to browser timezone.
 * @returns ISO 8601 timestamp in UTC
 */
export function combineDateTimeToUTC(
  dateString: string,
  timeString: string,
  timezone?: string
): string {
  // Parse date and time parts manually to avoid JS Date timezone quirks
  const [year, month, day] = dateString.split('-').map(Number)
  const [hours, minutes] = timeString.split(':').map(Number)

  if (!timezone) {
    // No timezone specified — use browser-local behavior (backward compatible)
    const date = new Date(year, month - 1, day, hours, minutes, 0, 0)
    return date.toISOString().replace(/\.\d{3}Z$/, 'Z')
  }

  // Timezone-aware: interpret the date/time as being in the target timezone.
  // 1. Create a UTC date with the desired wall-clock values
  const utcGuess = Date.UTC(year, month - 1, day, hours, minutes, 0, 0)

  // 2. Probe the target timezone's UTC offset at that instant
  const formatter = new Intl.DateTimeFormat('en-US', {
    timeZone: timezone,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  })
  const parts = formatter.formatToParts(new Date(utcGuess))
  const p = (type: string) => Number(parts.find(x => x.type === type)?.value ?? 0)
  const tzYear = p('year')
  const tzMonth = p('month')
  const tzDay = p('day')
  let tzHour = p('hour')
  if (tzHour === 24) tzHour = 0 // Intl may return 24 for midnight
  const tzMinute = p('minute')

  // 3. The offset (in ms) is how much the timezone's wall clock differs from our UTC guess
  const localAsUtc = Date.UTC(tzYear, tzMonth - 1, tzDay, tzHour, tzMinute, 0, 0)
  const offsetMs = localAsUtc - utcGuess

  // 4. Subtract the offset to get the correct UTC time
  const corrected = new Date(utcGuess - offsetMs)

  // Convert to UTC and return as RFC3339 format (without milliseconds)
  // Go's time.Time parser expects this format
  return corrected.toISOString().replace(/\.\d{3}Z$/, 'Z')
}

/**
 * Format a UTC date string for display in a specific timezone
 *
 * @param utcDateString - ISO 8601 date string in UTC
 * @param timezone - IANA timezone (e.g., 'America/Phoenix')
 * @param options - Intl.DateTimeFormat options
 */
export function formatInTimezone(
  utcDateString: string,
  timezone: string,
  options: Intl.DateTimeFormatOptions
): string {
  const date = new Date(utcDateString)
  return date.toLocaleString('en-US', { ...options, timeZone: timezone })
}

/**
 * Format date as "Mon, Dec 1" in specified timezone
 */
export function formatDateInTimezone(
  utcDateString: string,
  timezone: string
): string {
  return formatInTimezone(utcDateString, timezone, {
    weekday: 'short',
    month: 'short',
    day: 'numeric',
  })
}

/**
 * Format date as "Sat Apr 5, 2025" in specified timezone (includes year)
 */
export function formatDateWithYearInTimezone(
  utcDateString: string,
  timezone: string
): string {
  const date = new Date(utcDateString)
  const formatter = new Intl.DateTimeFormat('en-US', {
    weekday: 'short',
    month: 'short',
    day: 'numeric',
    year: 'numeric',
    timeZone: timezone,
  })

  const parts = formatter.formatToParts(date)
  const weekday = parts.find(p => p.type === 'weekday')?.value || ''
  const month = parts.find(p => p.type === 'month')?.value || ''
  const day = parts.find(p => p.type === 'day')?.value || ''
  const year = parts.find(p => p.type === 'year')?.value || ''

  return `${weekday} ${month} ${day}, ${year}`
}

/**
 * Format time as "7:30 PM" in specified timezone
 */
export function formatTimeInTimezone(
  utcDateString: string,
  timezone: string
): string {
  return formatInTimezone(utcDateString, timezone, {
    hour: 'numeric',
    minute: '2-digit',
    hour12: true,
  })
}

/**
 * Format a UTC date string as an ISO 8601 string carrying the venue's local UTC
 * offset, e.g. "2026-03-14T20:00:00-07:00" for an 8 PM Phoenix show. Used for
 * structured data (JSON-LD MusicEvent.startDate) so crawlers index the local
 * start time, not the bare UTC instant. `timezone` must be a valid IANA name. (PSY-986)
 */
export function toZonedISOString(
  utcDateString: string,
  timezone: string
): string {
  const date = new Date(utcDateString)
  const parts = new Intl.DateTimeFormat('en-US', {
    timeZone: timezone,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
    timeZoneName: 'longOffset',
  }).formatToParts(date)
  const p = (type: string) => parts.find(x => x.type === type)?.value ?? ''
  let hour = p('hour')
  if (hour === '24') hour = '00' // Intl may render midnight as 24
  // longOffset → "GMT-07:00" / "GMT+05:30" / "GMT" (UTC)
  const match = p('timeZoneName').match(/GMT([+-]\d{2}:\d{2})/)
  const offset = match ? match[1] : '+00:00'
  return `${p('year')}-${p('month')}-${p('day')}T${hour}:${p('minute')}:${p('second')}${offset}`
}

/**
 * Parse an ISO date string into separate date and time strings for form inputs
 * Returns date in YYYY-MM-DD format and time in HH:MM format
 *
 * @param isoDateString - ISO 8601 date string
 * @returns Object with date and time strings
 */
export function parseISOToDateAndTime(
  isoDateString: string,
  timezone?: string
): {
  date: string
  time: string
} {
  const dateObj = new Date(isoDateString)

  if (!timezone) {
    // No timezone — use browser-local behavior (backward compatible)
    const year = dateObj.getFullYear()
    const month = String(dateObj.getMonth() + 1).padStart(2, '0')
    const day = String(dateObj.getDate()).padStart(2, '0')
    const date = `${year}-${month}-${day}`

    const hours = String(dateObj.getHours()).padStart(2, '0')
    const minutes = String(dateObj.getMinutes()).padStart(2, '0')
    const time = `${hours}:${minutes}`

    return { date, time }
  }

  // Timezone-aware: extract date/time parts in the target timezone
  const formatter = new Intl.DateTimeFormat('en-US', {
    timeZone: timezone,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  })
  const parts = formatter.formatToParts(dateObj)
  const p = (type: string) => parts.find(x => x.type === type)?.value ?? '00'

  let hourVal = p('hour')
  if (hourVal === '24') hourVal = '00' // Intl may return 24 for midnight

  const date = `${p('year')}-${p('month')}-${p('day')}`
  const time = `${hourVal}:${p('minute')}`

  return { date, time }
}
