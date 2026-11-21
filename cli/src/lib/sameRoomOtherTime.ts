import type { APIClient } from "./api";
import * as display from "./display";
import { showDedupWindow } from "./duplicates";
import { readEventDate } from "./showTimes";
import { resolveVenueTimezone } from "./timezone";
import { clean, fold, headlinerOf } from "./sameNightOtherRoom";

/**
 * Flag-only QA check for the dry run: a show whose headliner already has a show
 * at the SAME room on the same venue-local day, starting at a different time,
 * in the catalog or elsewhere in this run.
 *
 * That shape is two things that only a person can tell apart: one show stored
 * at another time, or a second show the same day (a matinee, a late set). So
 * this never refuses and never rewrites; it prints a warning naming the other
 * show and says so when a plan could not be checked.
 *
 * Both sides are compared on their start: a stored show's music time when it
 * has one, else its event_date; a listing's music time when it states one, else
 * the event_date it will be written with. So a listing identical to what is
 * stored compares equal on every later run.
 *
 * The headliner is read the way each side stores it: on a plan, the act whose
 * own signal says headliner, else the first act; on a stored show, the act
 * ranked first by role, then bill position, then id. A plan is compared at its
 * first room, the one its day and zone are read at.
 */

/** The part of a show plan this check reads. */
export interface SameRoomPlan {
  input: { event_date: string; music_at?: string; doors_at?: string };
  artists: Array<{ id?: number; name: string; is_headliner?: boolean; set_type?: string }>;
  venues: Array<{ id?: number; name: string }>;
  valid: boolean;
  duplicate?: { isDuplicate: boolean; existingShowId?: number };
}

/** One show from `GET /shows`, as far as this check reads it. */
export interface StoredShowForSameRoom {
  id: number;
  slug?: string;
  event_date: string;
  music_at?: string | null;
  venues?: Array<{ id: number; name: string; state?: string | null; timezone?: string | null }>;
  artists?: Array<{ id: number; name: string; set_type?: string | null; position?: number | null }>;
}

/** The page size and the ceiling on shows read per venue-local day. */
const PAGE = 200;
const SCAN_CAP = 600;
/** The most venue-local days read from the API at once. */
const CONCURRENT_DAY_READS = 4;

/**
 * The gap, in minutes, from which two starts read as possibly two shows: the
 * reach of one event's doors and start. The backend finder's SameShowClockSpan
 * holds the same value, applied to every stored clock rather than to starts,
 * and it accepts a spread of exactly this much where this check already reads
 * "may be a second show".
 */
export const SAME_SHOW_SPAN_MINUTES = 90;

/** The venue-local hour a show listed with a date and no start is stored at. */
const DATE_ONLY_CLOCK = "20:00";

/**
 * The stored show's headliner: role first, then bill position, then id, the
 * order the backend's showBillHeadlinerRank ranks a bill in.
 */
export function storedHeadlinerId(show: StoredShowForSameRoom): number | undefined {
  const ranked = [...(show.artists ?? [])].sort(
    (a, b) =>
      (a.set_type === "headliner" ? 0 : 1) - (b.set_type === "headliner" ? 0 : 1) ||
      (a.position ?? 0) - (b.position ?? 0) ||
      a.id - b.id,
  );
  return ranked[0]?.id;
}

/** An instant as a 24-hour venue-local clock, "20:30 PDT". */
export function localClock(instant: string, zone: string): string {
  const at = new Date(instant);
  if (Number.isNaN(at.getTime())) return clean(instant);
  return new Intl.DateTimeFormat("en-US", {
    timeZone: zone,
    hour: "2-digit",
    minute: "2-digit",
    hourCycle: "h23",
    timeZoneName: "short",
  }).format(at);
}

/** A stored show's start: its music time when it has one, else event_date. */
function storedStart(show: StoredShowForSameRoom): string {
  return show.music_at || show.event_date;
}

/**
 * The shows in `shows` at `roomId` headlined by `headlinerId`, split into those
 * starting at `start` and those starting at another readable instant. The
 * caller passes one venue-local day's shows.
 */
export function sameRoomShows(
  roomId: number,
  headlinerId: number,
  start: string,
  shows: StoredShowForSameRoom[],
): { sameInstant: StoredShowForSameRoom[]; otherTime: StoredShowForSameRoom[] } {
  const ours = Date.parse(start);
  const sameInstant: StoredShowForSameRoom[] = [];
  const otherTime: StoredShowForSameRoom[] = [];
  for (const show of shows) {
    if (!(show.venues ?? []).some((v) => v.id === roomId) || storedHeadlinerId(show) !== headlinerId) continue;
    const at = Date.parse(storedStart(show));
    if (Number.isNaN(at)) continue;
    (at === ours ? sameInstant : otherTime).push(show);
  }
  return { sameInstant, otherTime };
}

/** What a gap between two starts suggests, in the warnings' words. */
export function gapReading(minutes: number): string {
  return minutes < SAME_SHOW_SPAN_MINUTES
    ? `${minutes} minutes apart: probably the same show, stored at another time`
    : `${minutes} minutes apart: may be a second show the same day`;
}

/**
 * A listing that states a day and no start: written at the date-only hour with
 * no music time. Its stored instant is a placeholder, not a start to compare.
 */
function statesNoStart(plan: SameRoomPlan, start: string, zone: string): boolean {
  return (
    readEventDate(plan.input.event_date, zone)?.kind === "day" &&
    !plan.input.music_at &&
    localClock(start, zone).startsWith(DATE_ONLY_CLOCK)
  );
}

interface DayPage {
  shows: StoredShowForSameRoom[];
  truncated: boolean;
}

async function readDay(client: APIClient, fromDate: string, toDate: string): Promise<DayPage> {
  const shows: StoredShowForSameRoom[] = [];
  for (let offset = 0; offset < SCAN_CAP; offset = shows.length) {
    const page = await client.get<StoredShowForSameRoom[] | { shows?: StoredShowForSameRoom[]; total?: number }>(
      "/shows",
      { from_date: fromDate, to_date: toDate, limit: String(PAGE), offset: String(offset) },
    );
    const rows = Array.isArray(page) ? page : (page?.shows ?? []);
    shows.push(...rows);
    const total = Array.isArray(page) ? undefined : page?.total;
    if (rows.length === 0 || (total !== undefined ? shows.length >= total : rows.length < PAGE)) {
      return { shows, truncated: false };
    }
  }
  return { shows, truncated: true };
}

const minutesBetween = (a: string, b: string) => Math.round(Math.abs(Date.parse(a) - Date.parse(b)) / 60000);

/**
 * Prints the check. The stored-show half runs for every valid plan whose room
 * and headliner exist; the in-run half runs for every valid plan. `zoneOf` is
 * the zone the plan's clocks are read in and `startOf` is the plan's start (its
 * music time when it states one, else the event_date it will be written with).
 *
 * A plan ph already treats as a duplicate is flagged when the show it matched
 * starts at another time and no stored show of the same headliner at that room
 * starts at the plan's own start; ph skips such a listing, so a second show
 * that day cannot be added through it. A listing that states no start is
 * flagged against a stored show only when the two are SAME_SHOW_SPAN_MINUTES
 * or more apart, since its placeholder instant is not a start.
 */
export async function reportSameRoomOtherTime<P extends SameRoomPlan>(
  client: APIClient,
  plans: P[],
  zoneOf: (plan: P) => { state?: string; timezone?: string },
  startOf: (plan: P) => string,
): Promise<void> {
  const lines: string[] = [];

  interface Placed {
    plan: P;
    index: number;
    start: string;
    state?: string;
    timezone?: string;
    day: string;
    /** The listing's start as the warnings print it. */
    clock: string;
    noStart: boolean;
    roomKey: string;
    headKey: string;
    label: string;
    /** Set when the room and the headliner both exist, so stored shows can be read. */
    stored?: { roomId: number; headlinerId: number; dayKey: string };
  }
  const placed: Placed[] = [];
  const windows = new Map<string, { fromDate: string; toDate: string }>();

  for (const [index, plan] of plans.entries()) {
    if (!plan.valid) continue;
    const headliner = headlinerOf(plan);
    const room = plan.venues[0];
    if (!headliner || !room) continue;
    const { state, timezone } = zoneOf(plan);
    const zone = resolveVenueTimezone(state, timezone);
    const start = startOf(plan);
    const day = readEventDate(start, zone)?.day;
    if (!day || Number.isNaN(Date.parse(start))) continue;
    const noStart = statesNoStart(plan, start, zone);
    const p: Placed = {
      plan,
      index,
      start,
      state,
      timezone,
      day,
      noStart,
      clock: noStart ? `no start time (it is stored at ${localClock(start, zone)})` : localClock(start, zone),
      roomKey: room.id !== undefined ? `id:${room.id}` : `name:${fold(room.name)}`,
      headKey: headliner.id !== undefined ? `id:${headliner.id}` : `name:${fold(headliner.name)}`,
      label: `${clean(headliner.name)} on ${day} at ${clean(room.name)}`,
    };
    if (room.id !== undefined && headliner.id !== undefined) {
      const window = showDedupWindow(start, state, timezone);
      const dayKey = `${window.fromDate}|${window.toDate}`;
      if (!windows.has(dayKey)) windows.set(dayKey, window);
      p.stored = { roomId: room.id, headlinerId: headliner.id, dayKey };
    }
    placed.push(p);
  }

  // One read per venue-local day, at most CONCURRENT_DAY_READS at once: each is
  // started a few days ahead of the one being reported, and held settled so a
  // failed read waits for its turn to be reported.
  const dayKeys = [...windows.keys()];
  const reads = new Map<string, Promise<{ page?: DayPage; error?: unknown }>>();
  const startThrough = (last: number) => {
    for (const key of dayKeys.slice(0, last + 1)) {
      if (reads.has(key)) continue;
      const { fromDate, toDate } = windows.get(key)!;
      reads.set(
        key,
        readDay(client, fromDate, toDate).then(
          (page) => ({ page }),
          (error: unknown) => ({ error }),
        ),
      );
    }
  };

  for (const { plan, start, state, timezone, label, clock, noStart, stored } of placed) {
    if (!stored) continue;
    startThrough(dayKeys.indexOf(stored.dayKey) + CONCURRENT_DAY_READS - 1);
    const { page: found, error } = await reads.get(stored.dayKey)!;
    if (!found) {
      const message = error instanceof Error ? error.message : "Unknown error";
      lines.push(`${label}: could not read that day's shows (${clean(message)}); not checked`);
      continue;
    }
    if (found.truncated) {
      lines.push(`${label}: read the first ${SCAN_CAP} shows that day and stopped; the rest were not checked`);
    }
    const { sameInstant, otherTime } = sameRoomShows(stored.roomId, stored.headlinerId, start, found.shows);
    for (const other of otherTime) {
      const minutes = minutesBetween(start, storedStart(other));
      if (noStart && minutes < SAME_SHOW_SPAN_MINUTES) continue;
      const venue = (other.venues ?? []).find((v) => v.id === stored.roomId);
      const theirZone = resolveVenueTimezone(venue?.state ?? state, venue?.timezone ?? timezone);
      const theirs =
        other.music_at && Date.parse(other.music_at) !== Date.parse(other.event_date)
          ? `${localClock(other.music_at, theirZone)} (its music time; it is stored at ${localClock(other.event_date, theirZone)})`
          : localClock(storedStart(other), theirZone);
      const ref = other.slug ? clean(other.slug) : `show ${other.id}`;
      if (plan.duplicate?.isDuplicate) {
        if (plan.duplicate.existingShowId !== other.id || sameInstant.length > 0) continue;
        lines.push(
          `${label}: ph skips this listing as a copy of ${ref}, which starts ${theirs} where this listing states ${clock}, ` +
            `${gapReading(minutes)}. If it is a second show, ph cannot add it, and --confirm writes any doors or ` +
            `music time this listing states onto ${ref} where it has none`,
        );
      } else {
        lines.push(
          `${label}: ${ref} is already at this room that day with this headliner, starting ${theirs}; this listing ` +
            `states ${clock}, ${gapReading(minutes)}. Check before --confirm`,
        );
      }
    }
  }

  // Within the run: two listings this run would create at one room on one day
  // with one headliner, starting at different times.
  const creatable = placed.filter((p) => !p.plan.duplicate?.isDuplicate);
  for (const p of creatable) {
    const others = creatable.filter(
      (q) =>
        q.index !== p.index &&
        q.roomKey === p.roomKey &&
        q.headKey === p.headKey &&
        q.day === p.day &&
        Date.parse(q.start) !== Date.parse(p.start),
    );
    for (const q of others) {
      lines.push(
        `${p.label}: show #${q.index + 1} in this run is at this room that day with this headliner and states ` +
          `${q.clock}; this listing states ${p.clock}, ${gapReading(minutesBetween(p.start, q.start))}. ` +
          `Check before --confirm`,
      );
    }
  }

  if (lines.length === 0) return;
  display.header("Same Room, Same Day, Another Time (flag only)");
  for (const line of lines) display.warn(line);
}
