import type { APIClient } from "./api";
import * as display from "./display";
import { readEventDate } from "./showTimes";
import { resolveVenueTimezone } from "./timezone";

/**
 * Flag-only QA check for the dry run: a show whose headliner already has a show
 * the same venue-local night at ANOTHER room in the same city.
 *
 * That shape is how a calendar that serves several rooms gets ingested into one
 * of them: the copy at the other room is usually the real one. It is also
 * legitimate (an in-store and a gig, two sets), so this never refuses and never
 * rewrites. It prints a warning naming the other show and leaves the call to
 * the reviewer.
 */

/** The part of a show plan this check reads. */
export interface SameNightPlan {
  input: { event_date: string };
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

/** The most shows `GET /artists/{id}/shows` returns in one page. */
const ARTIST_SHOWS_PAGE = 200;

/** The act the show is billed under: the stated headliner, else the first act. */
function headlinerOf(plan: SameNightPlan) {
  return (
    plan.artists.find((a) => a.set_type === "headliner" || a.is_headliner === true) ?? plan.artists[0]
  );
}

const fold = (s: string | undefined) => (s ?? "").trim().toLowerCase();

/**
 * The shows in `shows` that sit at another room of the plan's city on the
 * plan's venue-local `day`. Each existing show's day is read in its own room's
 * zone, which is how the site dates it.
 */
export function sameNightOtherRoomShows(
  plan: SameNightPlan,
  day: string,
  shows: ArtistShowForSameNight[],
): ArtistShowForSameNight[] {
  const room = plan.venues[0];
  if (!room) return [];
  return shows.filter((show) => {
    const venue = show.venue;
    if (!venue) return false;
    if (room.id !== undefined ? venue.id === room.id : fold(venue.name) === fold(room.name)) return false;
    if (fold(venue.city) !== fold(room.city) || fold(venue.state) !== fold(room.state)) return false;
    const zone = resolveVenueTimezone(venue.state, venue.timezone ?? undefined);
    return readEventDate(show.event_date, zone)?.day === day;
  });
}

/**
 * Prints the check for every valid plan that is about to be created. `zoneOf`
 * is the zone the plan's own date is read in, so the night compared is the
 * night the show will be stored on.
 */
export async function reportSameNightOtherRooms<P extends SameNightPlan>(
  client: APIClient,
  plans: P[],
  zoneOf: (plan: P) => { state?: string; timezone?: string },
): Promise<void> {
  const lines: string[] = [];
  for (const plan of plans) {
    if (!plan.valid || plan.duplicate?.isDuplicate) continue;
    const headliner = headlinerOf(plan);
    const room = plan.venues[0];
    // A headliner or room this run would create has no shows to compare with.
    if (headliner?.id === undefined || !room) continue;

    const zone = zoneOf(plan);
    const day = readEventDate(plan.input.event_date, resolveVenueTimezone(zone.state, zone.timezone))?.day;
    if (!day) continue;

    let shows: ArtistShowForSameNight[];
    try {
      const page = await client.get<{ shows?: ArtistShowForSameNight[]; total?: number }>(
        `/artists/${headliner.id}/shows`,
        { time_filter: "all", year: day.slice(0, 4), limit: String(ARTIST_SHOWS_PAGE) },
      );
      shows = page?.shows ?? [];
      if ((page?.total ?? 0) > shows.length) {
        lines.push(
          `${headliner.name} on ${day}: read ${shows.length} of the artist's ${page!.total} shows that year; the rest were not checked`,
        );
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : "Unknown error";
      lines.push(`${headliner.name} on ${day}: could not read the artist's shows (${message}); not checked`);
      continue;
    }
    for (const other of sameNightOtherRoomShows(plan, day, shows)) {
      lines.push(
        `${headliner.name} on ${day} at ${room.name}: already has a show that night at ${other.venue!.name}` +
          ` (${other.slug || `ID ${other.id}`}); check which room the event is in`,
      );
    }
  }
  if (lines.length === 0) return;
  display.header("Same Night, Another Room (flag only)");
  for (const line of lines) display.warn(line);
}
