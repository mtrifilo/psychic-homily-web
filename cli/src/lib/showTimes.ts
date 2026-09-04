/**
 * Door and music times for the ingest path: reading the wall-clock strings a
 * venue calendar states, and turning a stated pair into the UTC instants
 * `shows.doors_at` / `shows.music_at` hold.
 *
 * The rules are the ones the discovery pipeline writes under
 * (`backend/internal/services/pipeline/discovery.go`, `parseClockTime` and
 * `resolveShowTimes`): explicit only, anchored in the venue's zone, nothing
 * written without a readable show time, and a contradictory pair written not at
 * all. Two ingest paths reach the same columns, so they answer the same
 * questions the same way or one of them publishes a time the other refuses to.
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
  const normalized = raw.replace(/\s+/gu, "").toLowerCase();

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

/** The calendar day a show's `event_date` names, as `YYYY-MM-DD`, or null. */
export function calendarDateOf(eventDate: unknown): string | null {
  if (typeof eventDate !== "string") return null;
  const match = /^(\d{4}-\d{2}-\d{2})(?:[T ]|$)/.exec(eventDate.trim());
  return match ? match[1] : null;
}

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
   * One line per stated value that is NOT being written, naming the reason.
   * Empty when the source stated nothing: silence needs no explanation.
   */
  notes: string[];
}

/** Whether a value was stated at all, for the purpose of explaining a refusal. */
function isStated(value: unknown): boolean {
  return typeof value === "string" && value.trim().length > 0;
}

/**
 * Map a show's stated door and music times onto the instants
 * `shows.doors_at` / `shows.music_at` hold, or explain why neither is written.
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
 * all. The default is America/Phoenix for a venue in Berlin as readily as for
 * one in Tucson, and every read surface refuses to print a clock on it
 * (`isShowTimezoneResolved` in `frontend/lib/utils/formatters.ts`), so writing
 * one here would store an instant nothing will ever render.
 */
export function resolveShowTimes(input: ShowTimesInput): ShowTimes {
  const notes: string[] = [];
  const doorsStated = isStated(input.doorsAt);
  const musicStated = isStated(input.musicAt);
  if (!doorsStated && !musicStated) return { notes };

  if (!isShowTimezoneResolved(input.state, input.timezone)) {
    notes.push(
      "doors/music times not stored: no timezone is known for this venue, so the clock would be anchored on the America/Phoenix default",
    );
    return { notes };
  }
  const zone = resolveVenueTimezone(input.state, input.timezone);

  const date = calendarDateOf(input.eventDate);
  if (date === null) {
    notes.push("doors/music times not stored: event_date does not state a calendar day to anchor them to");
    return { notes };
  }

  const music = parseClockTime(input.musicAt);
  if (music === null) {
    notes.push(
      musicStated
        ? `doors/music times not stored: "${String(input.musicAt)}" is not a readable music time`
        : "doors time not stored: the source states no music time, and doors alone is half a schedule",
    );
    return { notes };
  }

  const musicAt = localTimeToUTC(date, clock(music), zone);

  const doors = parseClockTime(input.doorsAt);
  if (doors === null) {
    if (doorsStated) {
      notes.push(`doors time not stored: "${String(input.doorsAt)}" is not a readable door time`);
    }
    return { musicAt, notes };
  }

  const doorsAt = localTimeToUTC(date, clock(doors), zone);
  if (Date.parse(musicAt) < Date.parse(doorsAt)) {
    notes.push(
      `doors/music times not stored: music at "${String(input.musicAt)}" is before doors at "${String(input.doorsAt)}", which states a day this listing did not`,
    );
    return { notes };
  }

  return { doorsAt, musicAt, notes };
}

/** `HH:MM`, the shape localTimeToUTC reads. */
function clock(time: ClockTime): string {
  return `${String(time.hour).padStart(2, "0")}:${String(time.minute).padStart(2, "0")}`;
}
