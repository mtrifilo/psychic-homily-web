/**
 * Door and music times for the ingest path: reading the wall-clock strings a
 * venue calendar states, and turning a stated pair into the UTC instants
 * `shows.doors_at` / `shows.music_at` hold.
 *
 * `parseClockTime` mirrors the function of the same name in
 * `backend/internal/services/pipeline/discovery.go`, refusal for refusal, and a
 * test parses that file's table to hold the two together. The surrounding rules
 * diverge from its `resolveShowTimes` in three DELIBERATE places:
 *
 * 1. STRICTER: nothing is written when no zone is known for the venue (see
 *    `resolveShowTimes` below). The pipeline has no such gate; its
 *    `venueLocalInstant` goes through `utils.EventLocation`, which falls back to
 *    the state map and writes anyway.
 * 2. STRICTER: a refused pair also leaves `event_date` on the date-only
 *    convention. The pipeline's `parseEventDate` still anchors `event_date` on a
 *    stated show time its own `resolveShowTimes` refused.
 * 3. LOOSER: `readEventDate` accepts a naive `YYYY-MM-DDTHH:MM`, which Go's
 *    `parseCalendarDate` rejects. That shape is this CLI's own long-standing
 *    `event_date` contract (see `normalizeDate`), so refusing it here would
 *    refuse times for shows the CLI still creates.
 *
 * KNOWN LIMITATION, shared with the pipeline: a listing dated the 4th that
 * states a music time of 12:30 AM is read as 00:30 ON the 4th, not as the small
 * hours of the 5th. Reading it the other way means inferring a day rollover the
 * source did not state, which is the one thing this module will not do. Unlike
 * the doors/music pair, there is nothing to check it against, so it is written
 * rather than refused, and `event_date` follows it.
 */

import {
  isShowTimezoneResolved,
  localClockExists,
  localTimeToUTC,
  resolveVenueTimezone,
} from "./timezone";

/** A wall-clock time of day, as stated. */
export interface ClockTime {
  hour: number;
  minute: number;
}

/**
 * The 12-hour wall clock venue calendars state ("7:00 PM", "6:30pm"). The
 * optional periods cover the "7:00 p.m." a raw passthrough leaks.
 */
const CLOCK_WITH_MERIDIEM = /^(\d{1,2}):(\d{2})([ap])\.?m\.?$/;

/** The 24-hour wall clock a feed can hand through unformatted ("19:00"). */
const CLOCK_24_HOUR = /^(\d{1,2}):(\d{2})$/;

/**
 * Exactly the codepoints Go's `unicode.IsSpace` matches.
 *
 * Spelled out rather than written `\s`, which differs from it in both
 * directions: `\s` does not match U+0085 and does match U+FEFF. Either
 * difference is a scraped listing one of the two parsers reads and the other
 * refuses.
 */
const UNICODE_SPACE =
  /[\t\n\v\f\r \u0085\u00a0\u1680\u2000-\u200a\u2028\u2029\u202f\u205f\u3000]+/g;

/**
 * Read a wall-clock time out of the free text a venue calendar states, or
 * report that the string is not one.
 *
 * Deliberately strict, and strict in the same places the pipeline's parser is.
 * "doors at 7" and "7ish" are not times. Neither is "19:00 pm", a 24-hour clock
 * wearing a meridiem. Neither is a bare "7:00", which names either half of the
 * day: a source that renders a 7 PM show as "7:00" is a real shape, and reading
 * it as 7 AM would publish a time nobody stated. Refusing leaves the caller with
 * "the source did not state a time", which is true.
 *
 * Whitespace is dropped Unicode-aware, not just the ASCII space: venue calendars
 * are scraped HTML and render "7:00&nbsp;PM", so the U+00A0 arrives verbatim.
 */
export function parseClockTime(raw: unknown): ClockTime | null {
  if (typeof raw !== "string") return null;
  const normalized = raw.replace(UNICODE_SPACE, "").toLowerCase();

  const meridiem = CLOCK_WITH_MERIDIEM.exec(normalized);
  if (meridiem) {
    let hour = Number(meridiem[1]);
    const minute = Number(meridiem[2]);
    if (hour < 1 || hour > 12 || minute > 59) return null;
    if (meridiem[3] === "p" && hour !== 12) hour += 12;
    if (meridiem[3] === "a" && hour === 12) hour = 0;
    return { hour, minute };
  }

  const twentyFour = CLOCK_24_HOUR.exec(normalized);
  if (twentyFour) {
    const hour = Number(twentyFour[1]);
    const minute = Number(twentyFour[2]);
    // Hours 1 through 12 with no meridiem could mean either half of the day, so
    // they are not a stated time. Hour 0 and 13 through 23 can only be a
    // 24-hour clock.
    if (hour > 23 || minute > 59 || (hour >= 1 && hour <= 12)) return null;
    return { hour, minute };
  }

  return null;
}

/**
 * Whether `YYYY-MM-DD` names a day that exists.
 *
 * The shape alone is not enough: `Date.UTC` overflow-normalizes, so month 13
 * becomes January of the next year and 2026-02-31 becomes March 4. Go's
 * `time.Parse("2006-01-02")` rejects both, and a show created on a date nobody
 * stated is the outcome this module exists to refuse, so the round trip decides.
 */
function isRealCalendarDay(day: string): boolean {
  const [year, month, date] = day.split("-").map(Number);
  const at = new Date(Date.UTC(year, month - 1, date));
  // Date.UTC reads a year under 100 as 1900 + it, which would fail the round
  // trip for every year Go's time.Parse accepts.
  if (year < 100) at.setUTCFullYear(year);
  return (
    at.getUTCFullYear() === year &&
    at.getUTCMonth() === month - 1 &&
    at.getUTCDate() === date
  );
}

/**
 * How a show's `event_date` reads, or null when it states no day at all.
 *
 * ONE reader for the whole module, because two of them disagreed: a shape check
 * that answered "date only" and a separate slicer that answered "which day"
 * differed on trailing whitespace, on a trailing `T`, and on an overflowing
 * hour, and a show could get its times anchored to a day its `event_date` was
 * about to be rejected for.
 *
 * The three readings are the three things the field can mean:
 *
 * - `day` states a calendar day and nothing else, so the writer supplies the
 *   20:00 convention.
 * - `local` states a wall clock with no zone, which by this CLI's contract is
 *   venue-local.
 * - `instant` states a zone of its own and therefore names one moment. Its
 *   `day` is that moment read in the VENUE's zone, which is the only day a
 *   doors or music clock can be anchored to: for a US evening show the UTC day
 *   is already tomorrow, so taking it would put both times a full day late.
 */
export type EventDateReading =
  | { kind: "day"; day: string }
  | { kind: "local"; day: string; time: string }
  | { kind: "instant"; day: string };

const DATE_ONLY = /^(\d{4}-\d{2}-\d{2})$/;
const DATE_AND_LOCAL_TIME = /^(\d{4}-\d{2}-\d{2})T(\d{2}):(\d{2})(?::(\d{2}))?$/;
const DATE_AND_INSTANT =
  /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}(?::\d{2})?(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/;

export function readEventDate(raw: unknown, zone: string): EventDateReading | null {
  if (typeof raw !== "string") return null;

  const dayOnly = DATE_ONLY.exec(raw);
  if (dayOnly) {
    return isRealCalendarDay(dayOnly[1]) ? { kind: "day", day: dayOnly[1] } : null;
  }

  const local = DATE_AND_LOCAL_TIME.exec(raw);
  if (local) {
    const [, day, hour, minute, second] = local;
    if (!isRealCalendarDay(day)) return null;
    if (Number(hour) > 23 || Number(minute) > 59 || Number(second ?? "0") > 59) {
      return null;
    }
    return { kind: "local", day, time: `${hour}:${minute}` };
  }

  if (DATE_AND_INSTANT.test(raw)) {
    const at = Date.parse(raw);
    if (!Number.isFinite(at)) return null;
    return { kind: "instant", day: dayInZone(at, zone) };
  }

  return null;
}

/** The calendar day an instant falls on in a zone, as `YYYY-MM-DD`. */
function dayInZone(instant: number, zone: string): string {
  const parts = new Intl.DateTimeFormat("en-CA", {
    timeZone: zone,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).formatToParts(new Date(instant));
  const p = (type: string) => parts.find((x) => x.type === type)?.value ?? "";
  return `${p("year")}-${p("month")}-${p("day")}`;
}

/**
 * Why a stated time is not being stored. The caller words these; this module
 * decides them, so a rewording cannot change what the tests are asserting.
 */
export type ShowTimesRefusal =
  | { reason: "no-timezone" }
  | { reason: "no-calendar-day"; eventDate: string }
  | { reason: "unreadable-music"; music: string }
  | { reason: "doors-without-music"; doors: string }
  | { reason: "unreadable-doors"; doors: string }
  | { reason: "clock-does-not-exist"; clock: string; day: string }
  | { reason: "music-before-doors"; doors: string; music: string };

export interface ShowTimesInput {
  /** The show's `event_date` as stated, date-only or a full timestamp. */
  eventDate: unknown;
  /**
   * The door time as STATED: a local wall clock ("7:00 PM"), not an instant.
   * Named apart from `ShowTimes.doorsAt`, which is the UTC instant this module
   * turns it into, because the two are both strings and a mix-up type-checks.
   */
  statedDoors: unknown;
  /** The music time as stated, same shape as `statedDoors`. */
  statedMusic: unknown;
  /** The matched venue's IANA zone, when it has one. */
  timezone?: string;
  /** The venue's state, the fallback the state map covers. */
  state?: string;
}

export interface ShowTimes {
  /** RFC3339 UTC instant for `shows.doors_at`, when one is written. */
  doorsAt?: string;
  /** RFC3339 UTC instant for `shows.music_at`, when one is written. */
  musicAt?: string;
  /**
   * One entry per stated value that is NOT being written. Empty when the source
   * stated nothing: silence needs no explanation.
   */
  refusals: ShowTimesRefusal[];
}

/**
 * Whether the source put anything in this field.
 *
 * Deliberately not "is a non-empty string": a batch item stating
 * `"music_at": 1930` stated something, and reporting it as unreadable is what
 * gets it in front of the person running the dry run. Only absence and the
 * empty string are silence.
 */
function isStated(value: unknown): boolean {
  if (value === undefined || value === null) return false;
  return typeof value === "string" ? value.trim().length > 0 : true;
}

/**
 * Map a show's stated door and music times onto the instants
 * `shows.doors_at` / `shows.music_at` hold, or report why neither is written.
 *
 * A readable MUSIC time is required before either column is written: the two are
 * a pair describing one evening's schedule, and with no show time to order it
 * against, a lone door time publishes half a schedule as if it were the whole
 * one.
 *
 * A stated pair whose music time lands before its door time is contradictory,
 * and the usual cause is a listing that crossed midnight and dropped the day
 * ("Doors 11:00 PM / Show 12:00 AM"). Recovering that would mean inferring a
 * rollover the source never stated, so neither time is written.
 *
 * A venue whose zone resolves only to the state map's default is not anchored at
 * all. `resolveVenueTimezone` hands back America/Phoenix for a venue in Berlin
 * as readily as for one in Tucson, and the read surfaces named in
 * `isShowTimezoneResolved`'s doc comment refuse to print a clock on it, so a
 * time written here would be an instant nothing renders.
 */
export function resolveShowTimes(input: ShowTimesInput): ShowTimes {
  const refusals: ShowTimesRefusal[] = [];
  const doorsStated = isStated(input.statedDoors);
  const musicStated = isStated(input.statedMusic);
  if (!doorsStated && !musicStated) return { refusals };

  if (!isShowTimezoneResolved(input.state, input.timezone)) {
    refusals.push({ reason: "no-timezone" });
    return { refusals };
  }
  const zone = resolveVenueTimezone(input.state, input.timezone);

  const reading = readEventDate(input.eventDate, zone);
  if (reading === null) {
    refusals.push({ reason: "no-calendar-day", eventDate: String(input.eventDate) });
    return { refusals };
  }
  const date = reading.day;

  const music = parseClockTime(input.statedMusic);
  if (music === null) {
    refusals.push(
      musicStated
        ? { reason: "unreadable-music", music: String(input.statedMusic) }
        : { reason: "doors-without-music", doors: String(input.statedDoors) },
    );
    return { refusals };
  }

  if (!localClockExists(date, clock(music), zone)) {
    refusals.push({
      reason: "clock-does-not-exist",
      clock: String(input.statedMusic),
      day: date,
    });
    return { refusals };
  }
  const musicAt = localTimeToUTC(date, clock(music), zone);

  const doors = parseClockTime(input.statedDoors);
  if (doors === null) {
    if (doorsStated) {
      refusals.push({ reason: "unreadable-doors", doors: String(input.statedDoors) });
    }
    return { musicAt, refusals };
  }

  if (!localClockExists(date, clock(doors), zone)) {
    refusals.push({
      reason: "clock-does-not-exist",
      clock: String(input.statedDoors),
      day: date,
    });
    return { musicAt, refusals };
  }
  const doorsAt = localTimeToUTC(date, clock(doors), zone);
  if (Date.parse(musicAt) < Date.parse(doorsAt)) {
    refusals.push({
      reason: "music-before-doors",
      doors: String(input.statedDoors),
      music: String(input.statedMusic),
    });
    return { refusals };
  }

  return { doorsAt, musicAt, refusals };
}

/** `HH:MM`, the shape localTimeToUTC reads. */
function clock(time: ClockTime): string {
  return `${String(time.hour).padStart(2, "0")}:${String(time.minute).padStart(2, "0")}`;
}
