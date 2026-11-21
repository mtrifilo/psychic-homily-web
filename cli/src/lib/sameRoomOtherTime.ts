import type { APIClient } from "./api";
import * as display from "./display";
import { showDedupWindow } from "./duplicates";
import { readEventDate } from "./showTimes";
import { resolveVenueTimezone } from "./timezone";
import { headlinerOf } from "./sameNightOtherRoom";

/**
 * Flag-only QA check for the dry run: a show whose headliner already has a show
 * at the SAME room on the same venue-local day, at a different time, in the
 * catalog or elsewhere in this run.
 *
 * That shape is two things that only a person can tell apart: a matinee and an
 * evening show, which are two shows, or one show listed again at a new time,
 * which is a duplicate. So this never refuses and never rewrites; it prints a
 * warning naming the other show and says so when a plan could not be checked.
 *
 * The headliner is read the way each side stores it: on a plan, the act whose
 * own signal says headliner, else the first act; on a stored show, the act
 * ranked first by role, then bill position, then id.
 */

/** The part of a show plan this check reads. */
export interface SameRoomPlan {
  input: { event_date: string };
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
  venues?: Array<{ id: number; name: string; state?: string | null; timezone?: string | null }>;
  artists?: Array<{ id: number; name: string; set_type?: string | null; position?: number | null }>;
}

/** The page size and the ceiling on shows read per venue-local day. */
const PAGE = 200;
const SCAN_CAP = 600;

/**
 * Replaces control and format characters (bidirectional overrides among them)
 * and the Unicode line separators, so a stored name cannot rewrite or reorder
 * the terminal line the operator reads.
 */
const clean = (s: string) => s.replace(/[\p{Cc}\p{Cf}\u2028\u2029]/gu, "?");

/** The stored show's headliner: role first, then bill position, then id. */
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
  if (Number.isNaN(at.getTime())) return instant;
  return new Intl.DateTimeFormat("en-US", {
    timeZone: zone,
    hour: "2-digit",
    minute: "2-digit",
    hourCycle: "h23",
    timeZoneName: "short",
  }).format(at);
}

/**
 * The shows in `shows` at `roomId`, headlined by `headlinerId`, stored at an
 * instant other than `instant`. The caller passes one venue-local day's shows.
 */
export function sameRoomOtherTimeShows(
  roomId: number,
  headlinerId: number,
  instant: string,
  shows: StoredShowForSameRoom[],
): StoredShowForSameRoom[] {
  const ours = Date.parse(instant);
  return shows.filter(
    (show) =>
      (show.venues ?? []).some((v) => v.id === roomId) &&
      storedHeadlinerId(show) === headlinerId &&
      Date.parse(show.event_date) !== ours,
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

/**
 * Prints the check for every valid plan whose room and headliner exist.
 * `zoneOf` is the zone the plan's clocks are read in and `instantOf` is the
 * event_date the plan will store, so the day and the time compared are the
 * ones the show would be written with.
 *
 * A plan ph already treats as a duplicate is flagged only when the show it
 * matched is at a different time: ph skips such a listing, so a second show
 * that day cannot be added through it.
 */
export async function reportSameRoomOtherTime<P extends SameRoomPlan>(
  client: APIClient,
  plans: P[],
  zoneOf: (plan: P) => { state?: string; timezone?: string },
  instantOf: (plan: P) => string,
): Promise<void> {
  const lines: string[] = [];
  const days = new Map<string, Promise<DayPage>>();

  interface Placed {
    index: number;
    instant: string;
    zone: string;
    day: string;
    roomKey: string;
    headKey: string;
    label: string;
  }
  const placed: Placed[] = [];

  for (const [index, plan] of plans.entries()) {
    if (!plan.valid) continue;
    const headliner = headlinerOf(plan);
    const room = plan.venues[0];
    if (!headliner || !room) continue;
    const { state, timezone } = zoneOf(plan);
    const zone = resolveVenueTimezone(state, timezone);
    const instant = instantOf(plan);
    const day = readEventDate(instant, zone)?.day;
    if (!day || Number.isNaN(Date.parse(instant))) continue;
    const label = `${clean(headliner.name)} on ${day} at ${clean(room.name)}`;
    placed.push({
      index,
      instant,
      zone,
      day,
      roomKey: room.id !== undefined ? `id:${room.id}` : `name:${room.name.trim().toLowerCase()}`,
      headKey: headliner.id !== undefined ? `id:${headliner.id}` : `name:${headliner.name.trim().toLowerCase()}`,
      label,
    });

    // The catalog half needs both ids: a room or headliner this run would
    // create has no stored shows.
    if (room.id === undefined || headliner.id === undefined) continue;
    const { fromDate, toDate } = showDedupWindow(instant, state, timezone);
    const key = `${fromDate}|${toDate}`;
    let page = days.get(key);
    if (!page) {
      page = readDay(client, fromDate, toDate);
      days.set(key, page);
    }
    let found: DayPage;
    try {
      found = await page;
    } catch (err) {
      const message = err instanceof Error ? err.message : "Unknown error";
      lines.push(`${label}: could not read that day's shows (${clean(message)}); not checked`);
      continue;
    }
    if (found.truncated) {
      lines.push(`${label}: read the first ${SCAN_CAP} shows that day and stopped; the rest were not checked`);
    }
    const ours = localClock(instant, zone);
    for (const other of sameRoomOtherTimeShows(room.id, headliner.id, instant, found.shows)) {
      const venue = (other.venues ?? []).find((v) => v.id === room.id);
      const theirs = localClock(other.event_date, resolveVenueTimezone(venue?.state ?? state, venue?.timezone ?? timezone));
      const ref = other.slug ? clean(other.slug) : `show ${other.id}`;
      if (plan.duplicate?.isDuplicate) {
        if (plan.duplicate.existingShowId !== other.id) continue;
        lines.push(
          `${label}: ph skips this listing as a copy of ${ref}, which is at ${theirs} where this listing says ${ours}. ` +
            `If they are two shows that day, ph cannot add this one, and --confirm writes any doors or music time ` +
            `this listing states onto ${ref} where it has none`,
        );
      } else {
        lines.push(
          `${label}: ${ref} is already at this room that day with this headliner, at ${theirs}; this listing is at ${ours}. ` +
            `Two shows that day are real, one show listed again at a new time is a duplicate: check before --confirm`,
        );
      }
    }
  }

  // Within the run: two listings this run would create at one room on one day
  // with one headliner, at different times.
  const creatable = placed.filter((p) => !plans[p.index].duplicate?.isDuplicate);
  for (const p of creatable) {
    const others = creatable.filter(
      (q) =>
        q.index !== p.index &&
        q.roomKey === p.roomKey &&
        q.headKey === p.headKey &&
        q.day === p.day &&
        Date.parse(q.instant) !== Date.parse(p.instant),
    );
    for (const q of others) {
      lines.push(
        `${p.label}: show #${q.index + 1} in this run is at this room that day with this headliner, ` +
          `at ${localClock(q.instant, q.zone)}; this listing is at ${localClock(p.instant, p.zone)}. Check before --confirm`,
      );
    }
  }

  if (lines.length === 0) return;
  display.header("Same Room, Same Day, Another Time (flag only)");
  for (const line of lines) display.warn(line);
}
