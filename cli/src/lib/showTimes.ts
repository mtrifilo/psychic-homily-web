/**
 * Door and music times for the ingest path: reading the wall-clock strings a
 * venue calendar states, and turning a stated pair into the UTC instants
 * `shows.doors_at` / `shows.music_at` hold.
 *
 * The parser and its refusals mirror `parseClockTime` in
 * `backend/internal/services/pipeline/discovery.go`, which writes the same two
 * columns from the discovery side. Two DELIBERATE divergences from that file's
 * `resolveShowTimes`, both stricter here:
 *
 * 1. Nothing is written when no zone is known for the venue (see
 *    `resolveShowTimes` below). The pipeline has no such gate: its
 *    `venueLocalInstant` goes through `utils.EventLocation`, which falls back to
 *    the state map and writes anyway.
 * 2. A refused pair also leaves `event_date` on the date-only convention. The
 *    pipeline's `parseEventDate` still anchors `event_date` on a stated show
 *    time that `resolveShowTimes` refused, so the two can disagree there.
 */

import {
  isShowTimezoneResolved,
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
  return (
    at.getUTCFullYear() === year &&
    at.getUTCMonth() === month - 1 &&
    at.getUTCDate() === date
  );
}

/** Whether a date string states one real calendar day and no time of day. */
export function isDateOnly(date: unknown): boolean {
  return (
    typeof date === "string" &&
    /^\d{4}-\d{2}-\d{2}$/.test(date) &&
    isRealCalendarDay(date)
  );
}

/** The calendar day a show's `event_date` names, as `YYYY-MM-DD`, or null. */
export function calendarDateOf(eventDate: unknown): string | null {
  if (typeof eventDate !== "string") return null;
  const match = /^(\d{4}-\d{2}-\d{2})(?:[T ]|$)/.exec(eventDate.trim());
  if (match === null) return null;
  return isRealCalendarDay(match[1]) ? match[1] : null;
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
  | { reason: "music-before-doors"; doors: string; music: string };

export interface ShowTimesInput {
  /** The show's `event_date` as stated, date-only or a full timestamp. */
  eventDate: unknown;
  /** The stated door time, a local wall clock ("7:00 PM"). */
  doorsAt: unknown;
  /** The stated music time, a local wall clock ("8:00 PM"). */
  musicAt: unknown;
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
  const doorsStated = isStated(input.doorsAt);
  const musicStated = isStated(input.musicAt);
  if (!doorsStated && !musicStated) return { refusals };

  if (!isShowTimezoneResolved(input.state, input.timezone)) {
    refusals.push({ reason: "no-timezone" });
    return { refusals };
  }
  const zone = resolveVenueTimezone(input.state, input.timezone);

  const date = calendarDateOf(input.eventDate);
  if (date === null) {
    refusals.push({ reason: "no-calendar-day", eventDate: String(input.eventDate) });
    return { refusals };
  }

  const music = parseClockTime(input.musicAt);
  if (music === null) {
    refusals.push(
      musicStated
        ? { reason: "unreadable-music", music: String(input.musicAt) }
        : { reason: "doors-without-music", doors: String(input.doorsAt) },
    );
    return { refusals };
  }

  const musicAt = localTimeToUTC(date, clock(music), zone);

  const doors = parseClockTime(input.doorsAt);
  if (doors === null) {
    if (doorsStated) {
      refusals.push({ reason: "unreadable-doors", doors: String(input.doorsAt) });
    }
    return { musicAt, refusals };
  }

  const doorsAt = localTimeToUTC(date, clock(doors), zone);
  if (Date.parse(musicAt) < Date.parse(doorsAt)) {
    refusals.push({
      reason: "music-before-doors",
      doors: String(input.doorsAt),
      music: String(input.musicAt),
    });
    return { refusals };
  }

  return { doorsAt, musicAt, refusals };
}

/** `HH:MM`, the shape localTimeToUTC reads. */
function clock(time: ClockTime): string {
  return `${String(time.hour).padStart(2, "0")}:${String(time.minute).padStart(2, "0")}`;
}
