/**
 * Timezone utilities for converting local venue times to UTC.
 *
 * The state→timezone mapping mirrors the frontend's getTimezoneForState()
 * in frontend/lib/utils/timeUtils.ts. Keep them in sync.
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
 * Uses the same Intl.DateTimeFormat offset-probing approach as the frontend's
 * combineDateTimeToUTC() in frontend/lib/utils/timeUtils.ts.
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

  // 1. Create a UTC date with the desired wall-clock values
  const utcGuess = Date.UTC(year, month - 1, day, hours, minutes, 0, 0);

  // 2. Probe the target timezone's UTC offset at that instant
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
  const parts = formatter.formatToParts(new Date(utcGuess));
  const p = (type: string) =>
    Number(parts.find((x) => x.type === type)?.value ?? 0);
  const tzYear = p("year");
  const tzMonth = p("month");
  const tzDay = p("day");
  let tzHour = p("hour");
  if (tzHour === 24) tzHour = 0; // Intl may return 24 for midnight
  const tzMinute = p("minute");

  // 3. The offset (in ms) is how much the timezone's wall clock differs from our UTC guess
  const localAsUtc = Date.UTC(
    tzYear,
    tzMonth - 1,
    tzDay,
    tzHour,
    tzMinute,
    0,
    0,
  );
  const offsetMs = localAsUtc - utcGuess;

  // 4. Subtract the offset to get the correct UTC time
  const corrected = new Date(utcGuess - offsetMs);

  // Return as RFC3339 without milliseconds (Go's time.Time expects this)
  return corrected.toISOString().replace(/\.\d{3}Z$/, "Z");
}
