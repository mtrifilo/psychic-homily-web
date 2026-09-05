/**
 * Timezone utilities for converting local venue times to UTC.
 *
 * The state to timezone mapping mirrors the frontend's getTimezoneForState()
 * in frontend/lib/utils/timeUtils.ts. Keep them in sync.
 *
 * `localTimeToUTC` and the frontend's `combineDateTimeToUTC` no longer agree:
 * this one probes the zone offset twice and matches Go for a wall clock inside a
 * DST transition, the frontend's probes once and does not. Reconciling them is a
 * frontend change with its own gates, not something to do quietly from here.
 */

/** Map of US state abbreviations to IANA timezones. */
const STATE_TIMEZONES: Record<string, string> = {
  AZ: "America/Phoenix",
  CA: "America/Los_Angeles",
  NV: "America/Los_Angeles",
  CO: "America/Denver",
  NM: "America/Denver",
  TX: "America/Chicago",
  NY: "America/New_York",
  // Eastern
  CT: "America/New_York",
  DC: "America/New_York",
  DE: "America/New_York",
  FL: "America/New_York",
  GA: "America/New_York",
  MA: "America/New_York",
  MD: "America/New_York",
  ME: "America/New_York",
  MI: "America/New_York",
  NC: "America/New_York",
  NH: "America/New_York",
  NJ: "America/New_York",
  OH: "America/New_York",
  PA: "America/New_York",
  RI: "America/New_York",
  SC: "America/New_York",
  VA: "America/New_York",
  VT: "America/New_York",
  WV: "America/New_York",
  // Central
  AL: "America/Chicago",
  AR: "America/Chicago",
  IA: "America/Chicago",
  IL: "America/Chicago",
  IN: "America/Indiana/Indianapolis",
  KS: "America/Chicago",
  KY: "America/New_York",
  LA: "America/Chicago",
  MN: "America/Chicago",
  MO: "America/Chicago",
  MS: "America/Chicago",
  ND: "America/Chicago",
  NE: "America/Chicago",
  OK: "America/Chicago",
  SD: "America/Chicago",
  TN: "America/Chicago",
  WI: "America/Chicago",
  // Mountain
  ID: "America/Boise",
  MT: "America/Denver",
  UT: "America/Denver",
  WY: "America/Denver",
  // Pacific
  OR: "America/Los_Angeles",
  WA: "America/Los_Angeles",
  // Non-contiguous
  AK: "America/Anchorage",
  HI: "Pacific/Honolulu",
};

/**
 * Get IANA timezone for a US state abbreviation.
 * Defaults to America/Phoenix (Arizona, no DST) — same as frontend.
 */
export function getTimezoneForState(state: string): string {
  return STATE_TIMEZONES[state.toUpperCase()] || "America/Phoenix";
}

/**
 * Whether the state map actually covers this state, as opposed to handing back
 * its America/Phoenix default.
 *
 * The default is indistinguishable from a real answer once getTimezoneForState
 * has returned, so a caller that needs to know whether a show's zone is KNOWN
 * rather than assumed has to ask before resolving. Mirrors hasTimezoneForState
 * in frontend/lib/utils/timeUtils.ts, which the read surfaces gate their clocks
 * on.
 */
export function hasTimezoneForState(state?: string): boolean {
  return !!state && state.toUpperCase() in STATE_TIMEZONES;
}

/**
 * Whether a zone can be resolved for a show from the venue's own zone or from
 * the state map, rather than from the America/Phoenix default.
 *
 * Mirrors isShowTimezoneResolved in frontend/lib/utils/formatters.ts. The read
 * surfaces that print a clock (the status stripe's DOORS / MUSIC / start-time
 * segments, MusicEvent.startDate) all refuse when this is false, so a write path
 * that stores a time on the default stores an instant nothing renders.
 */
export function isShowTimezoneResolved(
  state?: string,
  timezone?: string,
): boolean {
  return (!!timezone && isValidTimeZone(timezone)) || hasTimezoneForState(state);
}

/**
 * Resolve the IANA zone a show's wall-clock time should be read in.
 *
 * Precedence, and the reason it is stated in one place: a venue that is already
 * in the database carries the zone the geocoder resolved for it, and that is the
 * SAME value every read surface renders the show in. The state map is a guess
 * for venues without one, and outside the US it is not even a guess: it hands
 * back America/Phoenix for "England", "QC", or "" alike.
 *
 * PSY-1873: `ph submit-show` keyed on the state map alone, so a date-only show
 * at a venue outside the US was written as 20:00 Phoenix while the show page,
 * which reads venues.timezone, rendered it in the venue's real zone. That is
 * wrong by the offset between the two, which for European venues moves the
 * show onto the following calendar day at around 4 AM. Nearly every show at a
 * non-US venue in production carried that signature.
 *
 * Mirrors the LOGIC of resolveShowTimezone in
 * frontend/lib/utils/formatters.ts, though not its memoization: that one sits
 * on a render path that asks the same question hundreds of times, while this
 * resolves once per show in a batch. An unloadable zone string falls through to
 * the state map rather than crashing, because Intl.DateTimeFormat throws a
 * RangeError on a bad zone.
 */
export function resolveVenueTimezone(
  state?: string,
  timezone?: string,
): string {
  if (timezone && isValidTimeZone(timezone)) return timezone;
  return getTimezoneForState(state || "");
}

/** Whether an IANA zone name exists in this runtime's tz database. */
function isValidTimeZone(name: string): boolean {
  try {
    new Intl.DateTimeFormat("en-US", { timeZone: name });
    return true;
  } catch {
    return false;
  }
}

/**
 * Convert a local date+time in a given timezone to a UTC ISO 8601 string.
 *
 * Offset probing, because JS has no "this wall clock, in this zone" constructor.
 * The probe runs TWICE: the first offset is read at the wall clock interpreted
 * as UTC, which is up to a day away from the answer and lands on the wrong side
 * of any DST transition in between. Re-reading the offset at the candidate
 * instant and re-deriving from it is what makes a clock inside a transition
 * window come out right, and a listing that states 12:30 AM and 1:30 AM on a
 * spring-forward night is exactly such a clock.
 *
 * A wall clock that does not exist (the hour a spring-forward skips) has no
 * correct answer, and one that happens twice (fall-back) has two. For both,
 * this returns the instant Go's `time.Date` returns for the same wall clock and
 * zone, verified against Go across every 2025-2027 transition in 30 zones. That
 * agreement is the whole specification: which side of the transition it lands
 * on varies by zone and is not a rule worth restating here. A caller that must
 * not store a clock the source could not have meant should ask
 * `localClockExists` first.
 *
 * @param dateStr  Date in YYYY-MM-DD format
 * @param timeStr  Time in HH:MM or HH:MM:SS format
 * @param timezone IANA timezone (e.g., "America/Phoenix")
 * @returns ISO 8601 UTC string like "2026-04-15T03:00:00Z"
 */
export function localTimeToUTC(
  dateStr: string,
  timeStr: string,
  timezone: string,
): string {
  const [year, month, day] = dateStr.split("-").map(Number);
  const timeParts = timeStr.split(":").map(Number);
  const hours = timeParts[0];
  const minutes = timeParts[1] || 0;

  // The requested wall clock, read as if it were UTC.
  const wanted = Date.UTC(year, month - 1, day, hours, minutes, 0, 0);

  const formatter = new Intl.DateTimeFormat("en-US", {
    timeZone: timezone,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false,
  });

  /** The wall clock this instant reads as in the zone, as a UTC-shaped number. */
  const wallClockAt = (instant: number): number => {
    const parts = formatter.formatToParts(new Date(instant));
    const p = (type: string) =>
      Number(parts.find((x) => x.type === type)?.value ?? 0);
    let hour = p("hour");
    if (hour === 24) hour = 0; // Intl may return 24 for midnight
    return Date.UTC(p("year"), p("month") - 1, p("day"), hour, p("minute"), 0, 0);
  };

  // The offset read at an instant, and the candidate that offset implies.
  const offsetAt = (instant: number): number => wallClockAt(instant) - instant;
  const first = wanted - offsetAt(wanted);
  const second = wanted - offsetAt(first);

  // `second` is the answer whenever it reads back as the clock that was asked
  // for; `first` covers the case where the re-probe overshot back across the
  // same transition. When neither reads back, the clock does not exist and
  // `second` is the post-transition instant.
  const chosen =
    wallClockAt(second) === wanted
      ? second
      : wallClockAt(first) === wanted
        ? first
        : second;

  // Return as RFC3339 without milliseconds (Go's time.Time expects this)
  return new Date(chosen).toISOString().replace(/\.\d{3}Z$/, "Z");
}

/**
 * Whether this wall clock exists on this day in this zone.
 *
 * False only inside the hour a spring-forward skips, where `localTimeToUTC` must
 * still return something and returns the instant Go does. A writer that refuses
 * to store a time the source did not state asks this first: 2:30 AM on a
 * spring-forward night is not a time that happened, and storing it as 1:30 AM
 * publishes a clock nobody printed.
 */
export function localClockExists(
  dateStr: string,
  timeStr: string,
  timezone: string,
): boolean {
  const instant = Date.parse(localTimeToUTC(dateStr, timeStr, timezone));
  const parts = new Intl.DateTimeFormat("en-GB", {
    timeZone: timezone,
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  }).formatToParts(new Date(instant));
  const p = (type: string) => parts.find((x) => x.type === type)?.value ?? "";
  const hour = p("hour") === "24" ? "00" : p("hour");
  const [wantedHour, wantedMinute] = timeStr.split(":");
  return hour === wantedHour && p("minute") === wantedMinute;
}
