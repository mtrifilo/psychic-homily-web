import { describe, test, expect, afterEach } from "bun:test";
import { processBatch } from "../src/commands/batch";
import {
  applyEventDateBound,
  buildShowPayload,
  quoteLineValue,
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

function show(eventDate: string, ticket = TICKET, extra: Record<string, unknown> = {}) {
  return {
    ...extra,
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
    expect(far.errors.join(" ")).toContain(`"allow_far_future_date": true`);
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

  test("--allow-far-future sends the override only on a marked show past the bound", async () => {
    const { get } = catalog();
    const posts: Array<Record<string, unknown>> = [];
    const result = await submitShows(
      client(get, posts),
      JSON.stringify([
        show("2028-10-03", TICKET, { allow_far_future_date: true }),
        show("2027-10-03", "https://tix.example/other", { allow_far_future_date: true }),
      ]),
      true,
      { now: NOW, allowFarFuture: true },
    );
    expect(result.created).toBe(2);
    expect(posts[0].allow_far_future_date).toBe(true);
    expect("allow_far_future_date" in posts[1]).toBe(false);
  });

  test("the flag alone does not lift the bound for an unmarked show", async () => {
    const { get } = catalog();
    const posts: unknown[] = [];
    const result = await submitShows(
      client(get, posts),
      JSON.stringify([show("2028-10-03", TICKET, { allow_far_future_date: true }), show("2029-10-03", "https://tix.example/b")]),
      true,
      { now: NOW, allowFarFuture: true },
    );
    expect(result.created).toBe(1);
    expect(result.plans[1].valid).toBe(false);
    expect(result.plans[1].errors.join(" ")).toContain("does not carry");
    expect(posts).toHaveLength(1);
  });

  test("a marked show without the flag is still refused", async () => {
    const { get } = catalog();
    const result = await submitShows(client(get), JSON.stringify([show("2028-10-03", TICKET, { allow_far_future_date: true })]), true, { now: NOW });
    expect(result.plans[0].valid).toBe(false);
    expect(result.plans[0].errors.join(" ")).toContain("re-run with --allow-far-future");
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

  test("a server that ignores the filter is detected after one page, not flagged", async () => {
    let lookups = 0;
    const get = async (path: string, params?: Record<string, string>) => {
      if (params?.ticket_url) {
        lookups++;
        return {
          shows: Array.from({ length: 200 }, (_, i) => ({
            id: i + 1, event_date: "2027-01-01T03:00:00Z", ticket_url: `https://other.example/${i}`,
            venues: [{ id: 7, name: "Great American Music Hall" }],
          })),
          total: 5000,
        };
      }
      return catalog().get(path, params);
    };
    const result = await submitShows(client(get), JSON.stringify([show("2026-10-03")]), false, { now: NOW });
    expect(lookups).toBe(1);
    expect(result.plans[0].ticketReuse).toBeUndefined();
    expect(result.plans[0].ticketReuseNote).toContain("does not filter");
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

describe("ph batch carries the far-future option and prints the flags", () => {
  const realFetch = globalThis.fetch;
  const realWrite = process.stderr.write.bind(process.stderr);
  afterEach(() => {
    globalThis.fetch = realFetch;
    process.stderr.write = realWrite;
  });

  async function runBatch(confirm: boolean, allowFarFuture: boolean) {
    const posts: Array<Record<string, unknown>> = [];
    const { get } = catalog({
      [TICKET]: [
        { id: 18089, slug: "2026-10-03-igorrr", event_date: "2026-10-04T03:00:00Z", ticket_url: TICKET, venues: [{ id: 7, name: "Great American Music Hall" }] },
      ],
    });
    globalThis.fetch = (async (input: string | URL | Request, init?: RequestInit) => {
      const url = new URL(String(input));
      if (init?.method === "POST" && url.pathname === "/shows") {
        posts.push(JSON.parse(String(init.body)));
        return new Response(JSON.stringify({ id: 901, slug: "created" }), { status: 200 });
      }
      const params = Object.fromEntries(url.searchParams.entries());
      const body = await get(url.pathname, params);
      return new Response(JSON.stringify(body ?? {}), { status: 200 });
    }) as typeof fetch;
    let out = "";
    process.stderr.write = ((chunk: string | Uint8Array) => {
      out += String(chunk);
      return true;
    }) as typeof process.stderr.write;

    const items = [
      { entity_type: "show", ...show("2027-10-03") },
      { entity_type: "show", ...show("2028-10-03", "https://tix.example/far", { allow_far_future_date: true }) },
    ] as unknown as Parameters<typeof processBatch>[0];
    await processBatch(items, { url: "http://api.test", token: "phk_test" }, confirm, { now: NOW, allowFarFuture });
    return { out, posts };
  }

  test("the dry run prints the ticket URL flag and the bound refusal", async () => {
    const { out, posts } = await runBatch(false, false);
    expect(out).toContain("ticket URL already sells show 18089");
    expect(out).toContain("more than 18 months ahead");
    expect(posts).toHaveLength(0);
  });

  test("a confirmed batch with the flag sends the override for the marked show", async () => {
    const { posts } = await runBatch(true, true);
    expect(posts).toHaveLength(2);
    const far = posts.find((p) => p.ticket_url === "https://tix.example/far");
    expect(far?.allow_far_future_date).toBe(true);
    const near = posts.find((p) => p.ticket_url === TICKET);
    expect(near && "allow_far_future_date" in near).toBe(false);
  });
});

describe("quoteLineValue", () => {
  test("a stored name cannot start a line of its own", () => {
    expect(quoteLineValue("Room\n! forged line\tx")).toBe("Room ! forged line x");
    expect(quoteLineValue("\u001b[2KRoom")).toBe("[2KRoom");
  });
});

describe("findTicketURLReuse paging", () => {
  test("a server that pages below 200 is read to its total", async () => {
    const all = Array.from({ length: 150 }, (_, i) => ({
      id: i + 1,
      event_date: "2027-01-01T03:00:00Z",
      ticket_url: TICKET,
      venues: [{ id: i === 149 ? 7 : 99, name: "Room" }],
    }));
    const offsets: string[] = [];
    const c = client(async (_path: string, params?: Record<string, string>) => {
      const offset = Number(params?.offset ?? "0");
      offsets.push(String(offset));
      return { shows: all.slice(offset, offset + 100), total: all.length };
    });
    const { reuse, truncated } = await findTicketURLReuse(c, TICKET, [7], "2026-10-03", "CA", "America/Los_Angeles");
    expect(offsets).toEqual(["0", "100"]);
    expect(truncated).toBe(false);
    expect(reuse.map((r) => r.showId)).toEqual([150]);
  });
});
