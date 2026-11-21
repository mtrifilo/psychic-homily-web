import type { APIClient } from "./api";
import * as display from "./display";
import { readEventDate } from "./showTimes";
import { resolveVenueTimezone } from "./timezone";
import { statedSlot } from "./setType";

/**
 * Flag-only QA check for the dry run: a show whose headliner already has a show
 * the same venue-local night at ANOTHER room in the same city.
 *
 * That shape is how a calendar that serves several rooms gets ingested into one
 * of them: the copy at the other room is usually the real one. It is also
 * legitimate (an in-store and a gig, two sets), so this never refuses and never
 * rewrites. It prints a warning naming the other show and leaves the call to
 * the reviewer, and it says so when a plan could not be checked.
 */

/** The part of a show plan this check reads. */
export interface SameNightPlan {
  input: { event_date: string; city?: string; state?: string };
  artists: Array<{ id?: number; name: string; is_headliner?: boolean; set_type?: string }>;
  venues: Array<{ id?: number; name: string; city?: string; state?: string }>;
  valid: boolean;
  duplicate?: { isDuplicate: boolean };
}

/** One show from `GET /artists/{id}/shows`, as far as this check reads it. */
export interface ArtistShowForSameNight {
  id: number;
  slug?: string;
  event_date: string;
  venue?: { id: number; name: string; city?: string; state?: string; timezone?: string | null } | null;
}

/** The city a plan's room is in, as the comparison reads it. */
export interface RoomCity {
  city: string;
  state: string;
}

/** The most shows `GET /artists/{id}/shows` returns in one page. */
const ARTIST_SHOWS_PAGE = 200;

/**
 * The act the show is billed under: the act whose own signal says headliner
 * (read the way the payload is built), else the first act.
 */
export function headlinerOf(plan: Pick<SameNightPlan, "artists">) {
  return plan.artists.find((a) => statedSlot(a) === "headliner") ?? plan.artists[0];
}

interface ArtistYearPage {
  shows: ArtistShowForSameNight[];
  total: number;
}

export const fold = (s: string | undefined | null) => (s ?? "").trim().toLowerCase();

/**
 * Replaces control and format characters (bidirectional overrides among them)
 * and the Unicode line separators, so a stored name cannot rewrite or reorder
 * the terminal line the operator reads.
 */
export const clean = (s: string) => s.replace(/[\p{Cc}\p{Cf}\u2028\u2029]/gu, "?");

/**
 * The shows in `shows` that sit at another room of `roomCity` on the plan's
 * venue-local `day`. Each existing show's day is read in its own room's zone,
 * which is how the site dates it.
 */
export function sameNightOtherRoomShows(
  plan: SameNightPlan,
  roomCity: RoomCity,
  day: string,
  shows: ArtistShowForSameNight[],
): ArtistShowForSameNight[] {
  const room = plan.venues[0];
  if (!room) return [];
  return shows.filter((show) => {
    const venue = show.venue;
    if (!venue) return false;
    if (room.id !== undefined ? venue.id === room.id : fold(venue.name) === fold(room.name)) return false;
    if (fold(venue.city) !== fold(roomCity.city) || fold(venue.state) !== fold(roomCity.state)) return false;
    const zone = resolveVenueTimezone(venue.state, venue.timezone ?? undefined);
    return readEventDate(show.event_date, zone)?.day === day;
  });
}

/**
 * Prints the check for every valid plan that is about to be created. `zoneOf`
 * is the zone the plan's own date is read in, so the night compared is the
 * night the show will be stored on.
 *
 * A matched room is compared by the city and state stored on it, read once per
 * room, and is not checked when either is blank; a room this run would create
 * is compared by the city the batch gives it, else the show's own.
 */
export async function reportSameNightOtherRooms<P extends SameNightPlan>(
  client: APIClient,
  plans: P[],
  zoneOf: (plan: P) => { state?: string; timezone?: string },
): Promise<void> {
  const lines: string[] = [];
  // One read per artist and year, and one per matched room: a residency or a
  // repeated act is many plans over the same pages.
  const pages = new Map<string, Promise<ArtistYearPage>>();
  const pageFor = (artistId: number, year: string) => {
    const key = `${artistId}:${year}`;
    let page = pages.get(key);
    if (!page) {
      page = client
        .get<{ shows?: ArtistShowForSameNight[]; total?: number }>(`/artists/${artistId}/shows`, {
          time_filter: "all",
          year,
          limit: String(ARTIST_SHOWS_PAGE),
        })
        .then((r) => ({ shows: r?.shows ?? [], total: r?.total ?? 0 }));
      pages.set(key, page);
    }
    return page;
  };
  const storedRooms = new Map<number, Promise<{ city?: string; state?: string }>>();
  const storedRoom = (id: number) => {
    let room = storedRooms.get(id);
    if (!room) {
      room = client.get<{ city?: string; state?: string }>(`/venues/${id}`).then((r) => r ?? {});
      storedRooms.set(id, room);
    }
    return room;
  };

  for (const plan of plans) {
    if (!plan.valid || plan.duplicate?.isDuplicate) continue;
    const headliner = headlinerOf(plan);
    const room = plan.venues[0];
    // A headliner this run would create has no shows yet; a room this run would
    // create is still compared, by name and by the city the batch gives it.
    if (headliner?.id === undefined || !room) continue;

    const zone = zoneOf(plan);
    const day = readEventDate(plan.input.event_date, resolveVenueTimezone(zone.state, zone.timezone))?.day;
    if (!day) continue;
    const label = `${clean(headliner.name)} on ${day} at ${clean(room.name)}`;

    let shows: ArtistShowForSameNight[];
    let roomCity: RoomCity;
    try {
      if (room.id !== undefined) {
        const stored = await storedRoom(room.id);
        roomCity = { city: stored.city?.trim() ?? "", state: stored.state?.trim() ?? "" };
      } else {
        roomCity = {
          city: room.city?.trim() || plan.input.city?.trim() || "",
          state: room.state?.trim() || plan.input.state?.trim() || "",
        };
      }
      if (!roomCity.city || !roomCity.state) {
        lines.push(`${label}: the room's city is not known; not checked`);
        continue;
      }
      const page = await pageFor(headliner.id, day.slice(0, 4));
      shows = page.shows;
      if (page.total > shows.length) {
        lines.push(`${label}: read ${shows.length} of the artist's ${page.total} shows that year; the rest were not checked`);
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : "Unknown error";
      lines.push(`${label}: could not read the artist's shows or the room (${clean(message)}); not checked`);
      continue;
    }
    for (const other of sameNightOtherRoomShows(plan, roomCity, day, shows)) {
      lines.push(
        `${label}: already has a show that night at ${clean(other.venue!.name)}` +
          ` (${other.slug ? clean(other.slug) : `ID ${other.id}`}); check which room the event is in`,
      );
    }
  }
  if (lines.length === 0) return;
  display.header("Same Night, Another Room (flag only)");
  for (const line of lines) display.warn(line);
}
