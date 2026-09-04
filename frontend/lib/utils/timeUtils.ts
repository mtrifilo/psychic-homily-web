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
 * WHERE THAT IS ENFORCED, stated narrowly because it is not a site-wide rule
 * and reads as one if you squint. Three FRONTEND READ paths ask
 * `isShowTimezoneResolved` first and say less when the answer is no:
 * `formatShowTime` (`./formatters`) returns null, so a listing surface drops
 * its time segment and the separator that introduced it; `startTimeFactSegment`
 * and `doorsMusicFactSegment` in
 * `features/shows/components/showStatusStripeCopy.ts` return null, and the
 * stripe drops DOORS, MUSIC and TONIGHT with them; and `MusicEvent.startDate`
 * in `lib/seo/jsonld.ts` degrades to a bare calendar date rather than composing
 * an offset out of this zone.
 *
 * A DATE is a weaker claim and is still rendered on this constant. The show
 * page marks it `~` (`features/shows/showPageDate`, the same register as
 * `ENDS ~11PM` and `CAP ~500`); listing surfaces do not.
 *
 * WHAT STILL BUILDS A CLOCK ON THIS VALUE, none of it gated:
 * - The edit form, both legs. `show-form-utils.ts` seeds its time input from
 *   `parseISOToDateAndTime(event_date, resolveShowTimezone(...))` and
 *   `ShowForm`'s submit composes the instant back through the same resolver.
 *   That pair is the round trip described above, so the wall clock it shows is
 *   the one the submitter typed; it must not be withheld.
 * - `getShowLifecycleState` (`./showTiming`) compares venue-local calendar days
 *   through here, so a zone-less venue east of Phoenix keeps ON SALE and the
 *   ticket bracket live past its own midnight. That function carries the
 *   measured consequence and why gating it is a product decision.
 * - The `is_tonight` flag on the scene-day and also-tonight payloads, which the
 *   backend computes on its own copy of this fallback. A client that prints
 *   TONIGHT is repeating that claim.
 *
 * The Go twin lives at `backend/internal/utils/timezone.go`. Its readers ask
 * `shared.EventLocationResolved` before naming an hour, so the reminder email,
 * the ICS feeds and Discord withhold on the same rule the three frontend read
 * paths above do. The four payloads that used to send this constant's name
 * rather than the venue's nullable column (scene day, scene week, also-tonight,
 * show timeline) now send a null zone instead, so a surrendered zone is
 * distinguishable on the wire.
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
 * A wall clock resolved in a zone: the instant it names, and whether the zone
 * has such a clock on that day at all.
 */
interface ResolvedWallClock {
  /** Milliseconds since the epoch. */
  instant: number
  /**
   * Whether {@link instant} reads back in the zone as the clock that was asked
   * for. False only inside the window a spring-forward skips.
   */
  exists: boolean
}

/**
 * Resolve a wall clock in an IANA zone to the instant it names.
 *
 * JS has no "this wall clock, in this zone" constructor, so the offset is
 * probed. The probe runs TWICE, and that is the whole of what this function
 * knows that a one-line offset lookup does not: the first offset is read at the
 * wall clock interpreted as UTC, which is up to a day away from the answer and
 * so lands on the wrong side of any transition in between. Re-reading the offset
 * at the candidate instant and re-deriving from it is what makes a clock inside
 * a transition window come out right, and a listing that states 12:30 AM and
 * 1:30 AM on a spring-forward night is exactly such a clock: on one probe both
 * resolve to the same instant.
 *
 * The instant returned is the one Go's `time.Date` returns for the same wall
 * clock and zone, pinned row by row by
 * `backend/internal/utils/testdata/dst_clock_corpus.json`, which the Go suite
 * checks against `time.Date` itself. That agreement is the specification.
 *
 * A clock inside a fall-back overlap happens twice and both instants are
 * defensible. Which one comes back depends on which side of the transition the
 * probe lands on, and that varies by zone rather than following a rule: 01:30 on
 * America/Chicago's fall-back night resolves to the earlier occurrence and 02:30
 * on Europe/Berlin's resolves to the later one. Both are in the corpus.
 *
 * A clock inside a spring-forward gap never happened, so no instant is right.
 * One is still returned, because every caller needs a value, and `exists` is
 * false so a caller that must not store a clock nobody could have meant can
 * refuse instead.
 */
function resolveWallClockInZone(
  dateString: string,
  timeString: string,
  timezone: string
): ResolvedWallClock {
  const [year, month, day] = dateString.split('-').map(Number)
  const [hours, minutes] = timeString.split(':').map(Number)

  // The requested wall clock, read as if it were UTC.
  const wanted = Date.UTC(year, month - 1, day, hours, minutes, 0, 0)

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

  /** The wall clock this instant reads as in the zone, as a UTC-shaped number. */
  const wallClockAt = (instant: number): number => {
    const parts = formatter.formatToParts(new Date(instant))
    const p = (type: string) => Number(parts.find(x => x.type === type)?.value ?? 0)
    let hour = p('hour')
    if (hour === 24) hour = 0 // Intl may return 24 for midnight
    return Date.UTC(p('year'), p('month') - 1, p('day'), hour, p('minute'), 0, 0)
  }

  const offsetAt = (instant: number): number => wallClockAt(instant) - instant
  const first = wanted - offsetAt(wanted)
  const second = wanted - offsetAt(first)

  // `second` is the answer whenever it reads back as the clock that was asked
  // for; `first` covers the case where the re-probe overshot back across the
  // same transition. When neither reads back, the clock does not exist and
  // `second` is the post-transition instant.
  const instant =
    wallClockAt(second) === wanted
      ? second
      : wallClockAt(first) === wanted
        ? first
        : second

  return { instant, exists: wallClockAt(instant) === wanted }
}

/**
 * Combines a date string and time string into a UTC ISO timestamp
 * Treats the input as local time in the specified timezone
 *
 * A clock inside a spring-forward gap has no correct answer and still gets one
 * here, because every caller needs a value; a caller that must not store a time
 * nobody could have entered asks {@link localClockExists} first.
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
  if (!timezone) {
    // No timezone specified — use browser-local behavior (backward compatible).
    // The engine resolves the transition nights on its own rule here, which is
    // not necessarily the zoned branch's; nothing that composes an instant for
    // STORAGE takes this branch.
    const [year, month, day] = dateString.split('-').map(Number)
    const [hours, minutes] = timeString.split(':').map(Number)
    const date = new Date(year, month - 1, day, hours, minutes, 0, 0)
    return date.toISOString().replace(/\.\d{3}Z$/, 'Z')
  }

  const { instant } = resolveWallClockInZone(dateString, timeString, timezone)

  // RFC3339 without milliseconds, which is what Go's time.Time parser expects.
  return new Date(instant).toISOString().replace(/\.\d{3}Z$/, 'Z')
}

/**
 * Whether this wall clock exists on this day in this zone.
 *
 * False only inside the window a spring-forward skips, where
 * {@link combineDateTimeToUTC} must still return something and returns the
 * instant Go's `time.Date` does. 2:30 AM on a spring-forward night is not a time
 * that happened, and storing it resolves to a clock nobody entered: on
 * America/Chicago's 2026-03-08 it is the same instant 1:30 AM resolves to.
 *
 * A clock inside a fall-back overlap is TRUE here. It happened, twice, and the
 * instant returned is one of the two; ambiguity is not a reason to refuse a time
 * a person typed.
 *
 * The zone is required, unlike on {@link combineDateTimeToUTC}: without one
 * there is no zone whose transitions this could be a claim about.
 */
export function localClockExists(
  dateString: string,
  timeString: string,
  timezone: string
): boolean {
  return resolveWallClockInZone(dateString, timeString, timezone).exists
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
 * Format time in the COMPACT ledger register: "8PM", and "7:30PM" when the
 * minutes are not zero.
 *
 * A dense fixed-width column, not a second site-wide clock format. The full
 * register above is what a sentence or a form says; this one is for a ledger
 * row whose lead column is measured in pixels — `8PM` is about 22px in the
 * rails' `text-xs` mono against `8:00 PM`'s ~57px — and for the show page's
 * status stripe and venue facts line, which set the same register.
 *
 * The half-hour case keeps its minutes because there is no shorter true way to
 * say 19:30, and dropping them would move a door time by half an hour.
 *
 * Built from `formatToParts` rather than a format string so the hour, minute
 * and AM/PM are read from the SAME formatted instant. An unreadable instant
 * yields "" rather than a `RangeError`, which `formatToParts` throws on an
 * invalid date and `toLocaleString` does not. Epoch milliseconds are accepted
 * beside the wire string so a caller holding an instant reaches that guard
 * rather than `toISOString`, which throws on a non-finite one before any guard
 * here could run.
 */
export function formatCompactTimeInTimezone(
  /** The wire's own spelling of a time, or epoch milliseconds. */
  utcDateStringOrInstant: string | number,
  timezone: string
): string {
  const date = new Date(utcDateStringOrInstant)
  if (!Number.isFinite(date.getTime())) return ''
  const parts = new Intl.DateTimeFormat('en-US', {
    timeZone: timezone,
    hour: 'numeric',
    minute: '2-digit',
    hour12: true,
  }).formatToParts(date)
  const part = (type: string) =>
    parts.find(p => p.type === type)?.value ?? ''
  const hour = part('hour')
  const minute = part('minute')
  const dayPeriod = part('dayPeriod').toUpperCase()
  return minute === '00'
    ? `${hour}${dayPeriod}`
    : `${hour}:${minute}${dayPeriod}`
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
 * The calendar day an instant falls on in `timezone`, as a bare
 * `YYYY-MM-DD` string: schema.org `Date` rather than `DateTime`.
 *
 * The weaker sibling of {@link toZonedISOString}, for the case where the zone
 * is {@link FALLBACK_SHOW_TIMEZONE} rather than one the row supplies. An
 * offset-bearing timestamp built on that zone states a wall clock nobody
 * entered; a date states only the day, which is the strongest claim the
 * fallback supports. Both forms are valid for `MusicEvent.startDate`.
 *
 * `timezone` must be a valid IANA name.
 */
export function toZonedDateOnly(
  utcDateString: string,
  timezone: string
): string {
  // The date half of the sibling above rather than a second formatter of its
  // own: that function assembles its result as year, month, day, `T`, clock, so
  // the two cannot answer different days for one instant. Ten characters covers
  // a four-digit year, which is every year `Intl` will be handed here; a
  // one-to-three-digit year would truncate, and so would the sibling's own
  // offset-bearing form, which is where that input has to be rejected instead.
  // The direct assertions in `timeUtils.test.ts` hold the prefix in place.
  return toZonedISOString(utcDateString, timezone).slice(0, 10)
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
