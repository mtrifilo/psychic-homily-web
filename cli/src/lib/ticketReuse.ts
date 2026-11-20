import type { APIClient } from "./api";
import { showDedupWindow } from "./duplicates";

/**
 * Another show at one of the same rooms, on a different venue-local day, that
 * already sells through the same ticket URL.
 *
 * Reported, never refused: a residency or a two-night stand can legitimately
 * share one link, and so does a year-replicated copy. Only a person reading
 * the dry run can tell those apart.
 */
export interface TicketURLReuse {
  showId: number;
  slug?: string;
  eventDate: string;
  venueName?: string;
}

export interface ShowWithTicket {
  id: number;
  slug?: string;
  event_date: string;
  ticket_url?: string | null;
  venues?: Array<{ id: number; name: string }>;
}

/**
 * The page size, equal to the `limit` maximum GET /shows accepts
 * (GetShowsRequest in backend/internal/api/handlers/catalog/show.go), and the
 * ceiling on how many matching shows are read.
 */
const TICKET_REUSE_PAGE = 200;
export const TICKET_REUSE_SCAN_CAP = 600;

/** What one ticket URL lookup found, and why it may be incomplete. */
export interface TicketURLLookup {
  shows: ShowWithTicket[];
  /** More shows matched than TICKET_REUSE_SCAN_CAP; the rest were not read. */
  truncated: boolean;
  /** The server returned shows with other ticket URLs, so it does not filter. */
  unsupported: boolean;
}

/** Lookups already made in a run, one per distinct URL. */
export type TicketURLLookupCache = Map<string, Promise<TicketURLLookup>>;

async function fetchShowsByTicketURL(client: APIClient, url: string): Promise<TicketURLLookup> {
  const shows: ShowWithTicket[] = [];
  // The offset advances by the rows actually returned, so a server that pages
  // below TICKET_REUSE_PAGE is still read to its total rather than stopped
  // after one short page.
  for (let offset = 0; offset < TICKET_REUSE_SCAN_CAP; offset = shows.length) {
    const page = await client.get<ShowWithTicket[] | { shows?: ShowWithTicket[]; total?: number }>("/shows", {
      ticket_url: url,
      limit: String(TICKET_REUSE_PAGE),
      offset: String(offset),
    });
    // Both shapes, as checkShowDuplicate reads this endpoint: a reader of one
    // shape alone silently sees nothing in the other.
    const rows = Array.isArray(page) ? page : (page?.shows ?? []);
    // A row with another link means the filter was not applied; reading on
    // would page through the whole catalog for nothing.
    if (rows.some((row) => (row.ticket_url ?? "").trim() !== url)) {
      return { shows: [], truncated: false, unsupported: true };
    }
    shows.push(...rows);
    const total = Array.isArray(page) ? undefined : page?.total;
    // With a total the server says when the list ends; without one, a short
    // page is the only end marker available.
    const done =
      rows.length === 0 ||
      (total !== undefined ? shows.length >= total : rows.length < TICKET_REUSE_PAGE);
    if (done) {
      return { shows, truncated: false, unsupported: false };
    }
  }
  return { shows, truncated: true, unsupported: false };
}

/**
 * Finds approved shows at one of `venueIds` whose ticket URL is exactly
 * `ticketUrl` and whose event_date is outside the new show's venue-local day
 * (the same day is the duplicate check's question, not this one).
 *
 * `state` and `timezone` are the zone the new show is written in, so the day
 * window brackets the instant the writer will store. Passing one `cache` for a
 * whole run reads each distinct URL once.
 */
export async function findTicketURLReuse(
  client: APIClient,
  ticketUrl: string,
  venueIds: number[],
  eventDate: string,
  state?: string,
  timezone?: string,
  cache: TicketURLLookupCache = new Map(),
): Promise<{ reuse: TicketURLReuse[]; truncated: boolean; unsupported: boolean }> {
  const url = ticketUrl.trim();
  if (!url || venueIds.length === 0) return { reuse: [], truncated: false, unsupported: false };

  const { fromDate, toDate } = showDedupWindow(eventDate, state, timezone);
  const from = Date.parse(fromDate);
  const to = Date.parse(toDate);

  let lookup = cache.get(url);
  if (!lookup) {
    lookup = fetchShowsByTicketURL(client, url);
    cache.set(url, lookup);
  }
  const { shows, truncated, unsupported } = await lookup;

  const reuse: TicketURLReuse[] = [];
  for (const show of shows) {
    // Re-checked here rather than trusted to the query, so a server that does
    // not apply the ticket_url filter cannot produce a false flag.
    if ((show.ticket_url ?? "").trim() !== url) continue;
    const room = (show.venues ?? []).find((v) => venueIds.includes(v.id));
    if (!room) continue;
    const at = Date.parse(show.event_date);
    const sameDay = Number.isFinite(at) && at >= from && at <= to;
    if (sameDay) continue;
    reuse.push({ showId: show.id, slug: show.slug, eventDate: show.event_date, venueName: room.name });
  }
  return { reuse, truncated, unsupported };
}

/**
 * Within one batch: for each show index, the other indices at the same room
 * (by resolved venue id, else by venue name) that state the same ticket URL on
 * a different date. A file that writes one event into several years has this
 * shape before any row exists for the API check to find.
 */
export function batchTicketURLReuse(
  shows: Array<{ ticketUrl?: string; venueKeys: string[]; day: string }>,
): Map<number, number[]> {
  const byKey = new Map<string, number[]>();
  shows.forEach((show, index) => {
    const url = show.ticketUrl?.trim();
    if (!url) return;
    for (const venue of show.venueKeys) {
      const key = `${venue}\u0000${url}`;
      const list = byKey.get(key) ?? [];
      list.push(index);
      byKey.set(key, list);
    }
  });

  const out = new Map<number, number[]>();
  for (const indices of byKey.values()) {
    for (const i of indices) {
      const others = indices.filter((j) => j !== i && shows[j].day !== shows[i].day);
      if (others.length === 0) continue;
      const merged = new Set([...(out.get(i) ?? []), ...others]);
      out.set(i, [...merged].sort((a, b) => a - b));
    }
  }
  return out;
}
