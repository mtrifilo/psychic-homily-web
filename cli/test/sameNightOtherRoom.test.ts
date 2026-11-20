import { describe, test, expect, spyOn, afterEach, afterAll } from "bun:test";
import {
  sameNightOtherRoomShows,
  reportSameNightOtherRooms,
  type SameNightPlan,
  type ArtistShowForSameNight,
} from "../src/lib/sameNightOtherRoom";
import { APIClient } from "../src/lib/api";
import * as display from "../src/lib/display";

const plan = (over: Partial<SameNightPlan> = {}): SameNightPlan => ({
  input: { event_date: "2026-11-25" },
  artists: [{ id: 42, name: "As I Lay Dying", set_type: "headliner" }],
  venues: [{ id: 184, name: "Launchpad", city: "Albuquerque", state: "NM" }],
  valid: true,
  ...over,
});

const at = (id: number, name: string, city: string, event_date: string, state = "NM"): ArtistShowForSameNight => ({
  id: id * 10,
  slug: `show-at-${id}`,
  event_date,
  venue: { id, name, city, state, timezone: "America/Denver" },
});

describe("sameNightOtherRoomShows", () => {
  test("finds the headliner at another room of the same city that venue-local night", () => {
    const found = sameNightOtherRoomShows(plan(), "2026-11-25", [
      at(954, "Sunshine Theater Albuquerque", "Albuquerque", "2026-11-26T03:00:00Z"),
    ]);
    expect(found.map((s) => s.slug)).toEqual(["show-at-954"]);
  });

  test("ignores the same room, another city, another state and another night", () => {
    const found = sameNightOtherRoomShows(plan(), "2026-11-25", [
      at(184, "Launchpad", "Albuquerque", "2026-11-26T03:00:00Z"),
      at(5, "Meow Wolf", "Santa Fe", "2026-11-26T03:00:00Z"),
      at(6, "Elsewhere", "Albuquerque", "2026-11-26T03:00:00Z", "TX"),
      at(954, "Sunshine Theater Albuquerque", "Albuquerque", "2026-11-27T03:00:00Z"),
    ]);
    expect(found).toEqual([]);
  });

  test("reads the night in the other room's zone, not in UTC", () => {
    // 11pm Mountain on the 25th is already the 26th in UTC.
    const late = at(954, "Sunshine Theater Albuquerque", "Albuquerque", "2026-11-26T06:00:00Z");
    expect(sameNightOtherRoomShows(plan(), "2026-11-25", [late])).toHaveLength(1);
    expect(sameNightOtherRoomShows(plan(), "2026-11-26", [late])).toHaveLength(0);
  });

  test("compares an unmatched room by name", () => {
    const p = plan({ venues: [{ name: "Launchpad", city: "Albuquerque", state: "NM" }] });
    const found = sameNightOtherRoomShows(p, "2026-11-25", [
      at(184, "launchpad", "albuquerque", "2026-11-26T03:00:00Z"),
      at(954, "Sunshine Theater Albuquerque", "Albuquerque ", "2026-11-26T03:00:00Z"),
    ]);
    expect(found.map((s) => s.venue!.id)).toEqual([954]);
  });
});

describe("reportSameNightOtherRooms", () => {
  const warnings: string[] = [];
  const warn = spyOn(display, "warn").mockImplementation((t: string) => {
    warnings.push(t);
  });
  const header = spyOn(display, "header").mockImplementation(() => {});
  afterEach(() => {
    warnings.length = 0;
    warn.mockClear();
  });
  // Spies outlive this file in bun's shared process unless restored.
  afterAll(() => {
    warn.mockRestore();
    header.mockRestore();
  });

  function client(handler: (path: string, params?: Record<string, string>) => unknown): {
    client: APIClient;
    calls: Array<{ path: string; params?: Record<string, string> }>;
  } {
    const calls: Array<{ path: string; params?: Record<string, string> }> = [];
    const c = Object.create(APIClient.prototype) as APIClient;
    (c as unknown as Record<string, unknown>).get = async (path: string, params?: Record<string, string>) => {
      calls.push({ path, params });
      return handler(path, params);
    };
    return { client: c, calls };
  }
  const denver = () => ({ state: "NM", timezone: "America/Denver" });

  test("warns, naming the other show, and asks the headliner's year", async () => {
    const { client: c, calls } = client(() => ({
      shows: [at(954, "Sunshine Theater Albuquerque", "Albuquerque", "2026-11-26T03:00:00Z")],
      total: 1,
    }));
    await reportSameNightOtherRooms(c, [plan()], denver);

    expect(calls).toEqual([
      { path: "/artists/42/shows", params: { time_filter: "all", year: "2026", limit: "200" } },
    ]);
    expect(warnings).toHaveLength(1);
    expect(warnings[0]).toContain("As I Lay Dying on 2026-11-25 at Launchpad");
    expect(warnings[0]).toContain("Sunshine Theater Albuquerque (show-at-954)");
  });

  test("uses the stated headliner, not the first act", async () => {
    const { client: c, calls } = client(() => ({ shows: [] }));
    await reportSameNightOtherRooms(
      c,
      [plan({ artists: [{ id: 1, name: "Opener" }, { id: 2, name: "Top", is_headliner: true }] })],
      denver,
    );
    expect(calls.map((x) => x.path)).toEqual(["/artists/2/shows"]);
  });

  test("skips invalid, duplicate and new-headliner plans without a request", async () => {
    const { client: c, calls } = client(() => ({ shows: [] }));
    await reportSameNightOtherRooms(
      c,
      [
        plan({ valid: false }),
        plan({ duplicate: { isDuplicate: true } }),
        plan({ artists: [{ name: "Brand New Act", set_type: "headliner" }] }),
      ],
      denver,
    );
    expect(calls).toEqual([]);
    expect(warnings).toEqual([]);
  });

  test("a failed read is reported as not checked and never throws", async () => {
    const { client: c } = client(() => {
      throw new Error("HTTP 503");
    });
    await reportSameNightOtherRooms(c, [plan()], denver);
    expect(warnings).toHaveLength(1);
    expect(warnings[0]).toContain("not checked");
  });

  test("says so when the year holds more shows than one page", async () => {
    const { client: c } = client(() => ({ shows: [], total: 250 }));
    await reportSameNightOtherRooms(c, [plan()], denver);
    expect(warnings[0]).toContain("read 0 of the artist's 250 shows");
  });

  // The real call site in submitShows: the dry run asks, a confirmed run does not.
  async function submit(confirm: boolean) {
    const { submitShows } = await import("../src/commands/submit-show");
    const { client: c, calls } = client((path) => {
      if (path.includes("/artists/search")) {
        return { artists: [{ id: 42, name: "As I Lay Dying", slug: "as-i-lay-dying" }] };
      }
      if (path.includes("/venues/search")) {
        return {
          venues: [
            { id: 184, name: "Launchpad", slug: "launchpad", city: "Albuquerque", state: "NM", timezone: "America/Denver" },
          ],
        };
      }
      if (path === "/artists/42/shows") {
        return { shows: [at(954, "Sunshine Theater Albuquerque", "Albuquerque", "2026-11-26T03:00:00Z")], total: 1 };
      }
      return { shows: [] };
    });
    (c as unknown as Record<string, unknown>).post = async () => ({ id: 1 });
    const json = JSON.stringify({
      event_date: "2026-11-25",
      city: "Albuquerque",
      state: "NM",
      artists: [{ name: "As I Lay Dying", is_headliner: true }],
      venues: [{ name: "Launchpad", city: "Albuquerque", state: "NM" }],
    });
    const result = await submitShows(c, json, confirm);
    return { paths: calls.map((x) => x.path), result };
  }

  test("submitShows: the dry run flags the show and still plans to create it", async () => {
    const { paths, result } = await submit(false);
    expect(paths).toContain("/artists/42/shows");
    expect(warnings.some((w) => w.includes("already has a show that night at Sunshine Theater Albuquerque"))).toBe(true);
    expect(result.created).toBe(1);
  });

  test("submitShows: a confirmed run does not run the check", async () => {
    const { paths, result } = await submit(true);
    expect(paths).not.toContain("/artists/42/shows");
    expect(result.created).toBe(1);
  });
});
