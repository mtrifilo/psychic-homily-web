import { describe, test, expect } from "bun:test";
import {
  applyEventDateBound,
  buildShowPayload,
  submitShows,
  type ShowPlan,
} from "../src/commands/submit-show";
import { APIClient } from "../src/lib/api";
import {
  SHOW_EVENT_DATE_BOUND_MONTHS,
  isBeyondShowEventDateBound,
  showEventDateBound,
} from "../src/lib/dateBound";
import { batchTicketURLReuse, findTicketURLReuse } from "../src/lib/ticketReuse";

const NOW = new Date("2026-11-20T18:00:00Z");
const TICKET = "https://wl.seetickets.us/event/igorrr/691689?afflky=GreatAmericanMusicHall";

function client(get: (path: string, params?: Record<string, string>) => Promise<unknown>, posts: unknown[] = []): APIClient {
  const c = Object.create(APIClient.prototype) as APIClient;
  (c as unknown as Record<string, unknown>).get = get;
  (c as unknown as Record<string, unknown>).post = async (_path: string, body?: unknown) => {
    posts.push(body);
    return { id: 900 + posts.length };
  };
  return c;
}

/** Resolves the Igorrr bill at Great American Music Hall (id 7, Pacific). */
function catalog(showsByTicket: Record<string, unknown[]> = {}) {
  const calls: Array<{ path: string; params?: Record<string, string> }> = [];
  const get = async (path: string, params?: Record<string, string>) => {
    calls.push({ path, params });
    if (path.includes("/artists/search")) return { artists: [{ id: 42, name: "Igorrr" }] };
    if (path.includes("/venues/search")) {
      return {
        venues: [
          { id: 7, name: "Great American Music Hall", city: "San Francisco", state: "CA", timezone: "America/Los_Angeles", verified: true },
        ],
      };
    }
    if (path === "/shows" && params?.ticket_url) return { shows: showsByTicket[params.ticket_url] ?? [] };
    return { shows: [] };
  };
  return { get, calls };
}

function show(eventDate: string, ticket = TICKET) {
  return {
    event_date: eventDate,
    city: "San Francisco",
    state: "CA",
    ticket_url: ticket,
    artists: [{ name: "Igorrr" }],
    venues: [{ name: "Great American Music Hall", city: "San Francisco", state: "CA" }],
  };
}

describe("showEventDateBound", () => {
  test("is 18 calendar months after now, in UTC", () => {
    expect(SHOW_EVENT_DATE_BOUND_MONTHS).toBe(18);
    expect(showEventDateBound(NOW).toISOString()).toBe("2028-05-20T18:00:00.000Z");
  });

  test("overflows a short month the way Go's AddDate does", () => {
    expect(showEventDateBound(new Date("2026-08-31T12:00:00Z")).toISOString()).toBe("2028-03-02T12:00:00.000Z");
  });

  test("both sides of the bound", () => {
    expect(isBeyondShowEventDateBound("2028-05-20T18:00:00Z", NOW)).toBe(false);
    expect(isBeyondShowEventDateBound("2028-05-20T18:00:01Z", NOW)).toBe(true);
    expect(isBeyondShowEventDateBound("2028-05-19T18:00:00Z", NOW)).toBe(false);
    expect(isBeyondShowEventDateBound("not a date", NOW)).toBe(false);
  });
});

describe("far-future bound in the show plan", () => {
  test("the dry run refuses a show past the bound and names the override", async () => {
    const { get } = catalog();
    const posts: unknown[] = [];
    const result = await submitShows(client(get, posts), JSON.stringify([show("2028-10-03"), show("2027-10-03")]), false, { now: NOW });

    const far = result.plans[0];
    expect(far.valid).toBe(false);
    expect(far.errors.join(" ")).toContain("more than 18 months ahead");
    expect(far.errors.join(" ")).toContain("--allow-far-future");
    expect(result.plans[1].valid).toBe(true);
    expect(result.created).toBe(1);
    expect(posts).toHaveLength(0);
  });

  test("a confirmed run never posts a refused show", async () => {
    const { get } = catalog();
    const posts: unknown[] = [];
    const result = await submitShows(client(get, posts), JSON.stringify([show("2083-08-15")]), true, { now: NOW });
    expect(result.created).toBe(0);
    expect(posts).toHaveLength(0);
  });

  test("--allow-far-future sends the override only on the shows past the bound", async () => {
    const { get } = catalog();
    const posts: Array<Record<string, unknown>> = [];
    const result = await submitShows(
      client(get, posts),
      JSON.stringify([show("2028-10-03"), show("2027-10-03", "https://tix.example/other")]),
      true,
      { now: NOW, allowFarFuture: true },
    );
    expect(result.created).toBe(2);
    expect(posts[0].allow_far_future_date).toBe(true);
    expect("allow_far_future_date" in posts[1]).toBe(false);
  });

  test("a duplicate is not judged: nothing is created for it", () => {
    const plan = {
      input: show("2083-08-15"),
      artists: [{ id: 42, name: "Igorrr", status: "existing" }],
      venues: [{ id: 7, name: "Great American Music Hall", timezone: "America/Los_Angeles", status: "existing" }],
      valid: true,
      errors: [],
      duplicate: { isDuplicate: true, existingShowId: 1 },
    } as ShowPlan;
    applyEventDateBound([plan], { now: NOW });
    expect(plan.valid).toBe(true);
    expect("allow_far_future_date" in buildShowPayload(plan)).toBe(false);
  });
});

describe("ticket URL reuse", () => {
  test("flags another show at the same room on another date, and only flags", async () => {
    const { get, calls } = catalog({
      [TICKET]: [
        { id: 18089, slug: "2028-10-03-igorrr-at-great-american-music-hall", event_date: "2028-10-04T03:00:00Z", ticket_url: TICKET, venues: [{ id: 7, name: "Great American Music Hall" }] },
        // Same venue-local day: the duplicate check's question, not this one.
        { id: 18065, slug: "2026-10-03-igorrr", event_date: "2026-10-04T03:00:00Z", ticket_url: TICKET, venues: [{ id: 7, name: "Great American Music Hall" }] },
        // Another room.
        { id: 5, slug: "elsewhere", event_date: "2027-01-01T03:00:00Z", ticket_url: TICKET, venues: [{ id: 99, name: "Elsewhere" }] },
        // A server that ignored the filter: a different ticket URL is never a flag.
        { id: 6, slug: "unrelated", event_date: "2027-02-01T03:00:00Z", ticket_url: "https://tix.example/x", venues: [{ id: 7, name: "Great American Music Hall" }] },
      ],
    });
    const posts: unknown[] = [];
    const result = await submitShows(client(get, posts), JSON.stringify([show("2026-10-03")]), true, { now: NOW });

    const plan = result.plans[0];
    expect(plan.valid).toBe(true);
    expect(plan.ticketReuse?.map((r) => r.showId)).toEqual([18089]);
    expect(result.created).toBe(1);
    expect(posts).toHaveLength(1);
    const query = calls.find((c) => c.params?.ticket_url);
    expect(query?.params?.ticket_url).toBe(TICKET);
  });

  test("flags shows in the same run at the same room and ticket URL on other dates", async () => {
    const { get, calls } = catalog();
    const result = await submitShows(
      client(get),
      JSON.stringify([show("2026-10-03"), show("2027-10-03"), show("2027-10-04", "https://tix.example/other")]),
      false,
      { now: NOW },
    );
    const lookups = calls.filter((c) => c.params?.ticket_url).map((c) => c.params?.ticket_url);
    expect(lookups).toEqual([TICKET, "https://tix.example/other"]);
    expect(result.plans[0].batchTicketReuse).toEqual([1]);
    expect(result.plans[1].batchTicketReuse).toEqual([0]);
    expect(result.plans[2].batchTicketReuse).toBeUndefined();
    expect(result.created).toBe(3);
  });

  test("a failed lookup is reported, not fatal", async () => {
    const get = async (path: string, params?: Record<string, string>) => {
      if (params?.ticket_url) throw new Error("HTTP 500");
      return catalog().get(path, params);
    };
    const result = await submitShows(client(get), JSON.stringify([show("2026-10-03")]), false, { now: NOW });
    expect(result.plans[0].valid).toBe(true);
    expect(result.plans[0].ticketReuseNote).toContain("HTTP 500");
  });

  test("findTicketURLReuse skips the call without a ticket URL or a matched room", async () => {
    let called = false;
    const c = client(async () => {
      called = true;
      return { shows: [] };
    });
    expect((await findTicketURLReuse(c, "  ", [7], "2026-10-03")).reuse).toEqual([]);
    expect((await findTicketURLReuse(c, TICKET, [], "2026-10-03")).reuse).toEqual([]);
    expect(called).toBe(false);
  });

  test("batchTicketURLReuse ignores the same day and empty URLs", () => {
    const out = batchTicketURLReuse([
      { ticketUrl: TICKET, venueKeys: ["id:7"], day: "2026-10-03" },
      { ticketUrl: TICKET, venueKeys: ["id:7"], day: "2026-10-03" },
      { ticketUrl: "", venueKeys: ["id:7"], day: "2027-10-03" },
      { ticketUrl: TICKET, venueKeys: ["id:8"], day: "2027-10-03" },
    ]);
    expect(out.size).toBe(0);
  });
});
