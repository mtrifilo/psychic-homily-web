import { describe, test, expect, spyOn, afterEach, afterAll } from "bun:test";
import {
  sameRoomShows,
  reportSameRoomOtherTime,
  storedHeadlinerId,
  localClock,
  type SameRoomPlan,
  type StoredShowForSameRoom,
} from "../src/lib/sameRoomOtherTime";
import { APIClient } from "../src/lib/api";
import * as display from "../src/lib/display";

const LA = "America/Los_Angeles";

// A listing that states its music time; the instant each test passes is the
// one that time is stored at.
const plan = (over: Partial<SameRoomPlan> = {}): SameRoomPlan => ({
  input: { event_date: "2026-10-09", music_at: "8:30 PM" },
  artists: [{ id: 42, name: "MARIS", set_type: "headliner" }],
  venues: [{ id: 7, name: "The Casbah" }],
  valid: true,
  ...over,
});

const stored = (
  id: number,
  event_date: string,
  over: Partial<StoredShowForSameRoom> = {},
): StoredShowForSameRoom => ({
  id,
  slug: `show-${id}`,
  event_date,
  venues: [{ id: 7, name: "The Casbah", state: "CA", timezone: LA }],
  artists: [{ id: 42, name: "MARIS", set_type: "headliner", position: 0 }],
  ...over,
});

describe("storedHeadlinerId", () => {
  test("ranks the curated headliner over the first-billed act, then position, then id", () => {
    expect(
      storedHeadlinerId(
        stored(1, "2026-10-10T03:00:00Z", {
          artists: [
            { id: 5, name: "Opener", set_type: "opener", position: 0 },
            { id: 9, name: "Top", set_type: "headliner", position: 1 },
          ],
        }),
      ),
    ).toBe(9);
    expect(
      storedHeadlinerId(
        stored(1, "2026-10-10T03:00:00Z", {
          artists: [
            { id: 8, name: "B", set_type: "performer", position: 0 },
            { id: 3, name: "A", set_type: "performer", position: 0 },
          ],
        }),
      ),
    ).toBe(3);
  });
});

describe("sameRoomShows", () => {
  test("splits the headliner's shows at the room into the same instant and another time", () => {
    const { sameInstant, otherTime } = sameRoomShows(7, 42, "2026-10-10T03:30:00.000Z", [
      stored(1, "2026-10-10T03:00:00Z"),
      stored(2, "2026-10-10T03:30:00Z"),
    ]);
    expect(otherTime.map((s) => s.id)).toEqual([1]);
    expect(sameInstant.map((s) => s.id)).toEqual([2]);
  });

  test("ignores another room, another headliner and an unreadable date", () => {
    const { sameInstant, otherTime } = sameRoomShows(7, 42, "2026-10-10T03:30:00Z", [
      stored(1, "2026-10-10T03:00:00Z", { venues: [{ id: 8, name: "Soda Bar" }] }),
      stored(2, "2026-10-10T03:00:00Z", {
        artists: [
          { id: 99, name: "Someone Else", set_type: "headliner", position: 0 },
          { id: 42, name: "MARIS", set_type: "performer", position: 1 },
        ],
      }),
      stored(3, "not a date"),
    ]);
    expect([...sameInstant, ...otherTime]).toEqual([]);
  });
});

describe("localClock", () => {
  test("reads an instant in the venue's zone", () => {
    expect(localClock("2026-10-10T03:30:00Z", LA)).toBe("20:30 PDT");
  });

  test("returns an unreadable instant cleaned", () => {
    expect(localClock(`bad${String.fromCharCode(27)}[2K`, LA)).toBe("bad?[2K");
  });
});

describe("reportSameRoomOtherTime", () => {
  const warnings: string[] = [];
  const warn = spyOn(display, "warn").mockImplementation((t: string) => {
    warnings.push(t);
  });
  const header = spyOn(display, "header").mockImplementation(() => {});
  afterEach(() => {
    warnings.length = 0;
    warn.mockClear();
  });
  afterAll(() => {
    warn.mockRestore();
    header.mockRestore();
  });

  function client(handler: (path: string, params?: Record<string, string>) => unknown | Promise<unknown>): {
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
  const pacific = () => ({ state: "CA", timezone: LA });

  test("flags a listing at another time the same day, naming both clocks and reading that day", async () => {
    const { client: c, calls } = client(() => ({ shows: [stored(1, "2026-10-10T03:00:00Z")], total: 1 }));
    await reportSameRoomOtherTime(c, [plan()], pacific, () => "2026-10-10T03:30:00.000Z");
    expect(calls).toHaveLength(1);
    expect(calls[0].path).toBe("/shows");
    expect(calls[0].params?.from_date).toBe("2026-10-09T07:00:00Z");
    expect(calls[0].params?.to_date).toBe("2026-10-10T06:59:00Z");
    expect(warnings).toHaveLength(1);
    expect(warnings[0]).toContain("MARIS on 2026-10-09 at The Casbah: show-1 is already at this room that day");
    expect(warnings[0]).toContain("at 20:00 PDT with no start time stored; this listing states 20:30 PDT");
  });

  test("an evening listing across midnight UTC reads the venue-local day that holds the matinee", async () => {
    const { client: c, calls } = client(() => ({ shows: [stored(1, "2026-12-06T23:00:00Z")], total: 1 }));
    await reportSameRoomOtherTime(
      c,
      [plan({ input: { event_date: "2026-12-06", music_at: "8:00 PM" } })],
      pacific,
      () => "2026-12-07T04:00:00.000Z",
    );
    // The window is the 6th in Pacific standard time, and it holds both instants.
    expect(calls[0].params?.from_date).toBe("2026-12-06T08:00:00Z");
    expect(calls[0].params?.to_date).toBe("2026-12-07T07:59:00Z");
    expect(warnings).toHaveLength(1);
    expect(warnings[0]).toContain("on 2026-12-06");
    expect(warnings[0]).toContain("at 15:00 PST; this listing states 20:00 PST");
  });

  test("says nothing for the same instant", async () => {
    const { client: c } = client(() => ({ shows: [stored(1, "2026-10-10T03:30:00Z")], total: 1 }));
    await reportSameRoomOtherTime(c, [plan()], pacific, () => "2026-10-10T03:30:00.000Z");
    expect(warnings).toEqual([]);
  });

  test("a duplicate ph skips is flagged only when the show it matched is at another time", async () => {
    const { client: c } = client(() => ({
      shows: [stored(1, "2026-10-10T03:00:00Z", { music_at: "2026-10-10T03:00:00Z" })],
      total: 1,
    }));
    await reportSameRoomOtherTime(
      c,
      [plan({ duplicate: { isDuplicate: true, existingShowId: 1 } }), plan({ duplicate: { isDuplicate: true, existingShowId: 9 } })],
      pacific,
      () => "2026-10-10T03:30:00.000Z",
    );
    expect(warnings).toHaveLength(1);
    expect(warnings[0]).toContain("ph skips this listing as a copy of show-1, which is at 20:00 PDT");
    expect(warnings[0]).toContain("ph cannot add this one");
  });

  test("a duplicate is not flagged when a stored show already sits at the listing's own instant", async () => {
    // A matinee and an evening show are both stored; the duplicate check matched
    // the matinee, but the evening listing's own show exists.
    const { client: c } = client(() => ({
      shows: [stored(1, "2026-10-09T22:00:00Z"), stored(2, "2026-10-10T03:30:00Z")],
      total: 2,
    }));
    await reportSameRoomOtherTime(
      c,
      [plan({ duplicate: { isDuplicate: true, existingShowId: 1 } })],
      pacific,
      () => "2026-10-10T03:30:00.000Z",
    );
    expect(warnings).toEqual([]);
  });

  test("a date-only refresh of a show stored with a time is not flagged as a duplicate at another time", async () => {
    const { client: c } = client(() => ({ shows: [stored(1, "2026-10-10T04:00:00Z")], total: 1 }));
    await reportSameRoomOtherTime(
      c,
      [plan({ input: { event_date: "2026-10-09" }, duplicate: { isDuplicate: true, existingShowId: 1 } })],
      pacific,
      () => "2026-10-10T03:00:00.000Z",
    );
    expect(warnings).toEqual([]);
  });

  test("a date-only listing that would be created says it states no time", async () => {
    const { client: c } = client(() => ({ shows: [stored(1, "2026-10-09T22:00:00Z")], total: 1 }));
    await reportSameRoomOtherTime(c, [plan({ input: { event_date: "2026-10-09" } })], pacific, () => "2026-10-10T03:00:00.000Z");
    expect(warnings).toHaveLength(1);
    expect(warnings[0]).toContain("this listing states no start time (it is stored at 20:00 PDT)");
  });

  test("a listing whose music time is 8pm states that start, though it is stored at the date-only hour", async () => {
    const { client: c } = client(() => ({ shows: [stored(1, "2026-10-09T22:00:00Z")], total: 1 }));
    await reportSameRoomOtherTime(
      c,
      [plan({ input: { event_date: "2026-10-09", music_at: "8:00 PM" } })],
      pacific,
      () => "2026-10-10T03:00:00.000Z",
    );
    expect(warnings).toHaveLength(1);
    expect(warnings[0]).toContain("this listing states 20:00 PDT");
  });

  test("a doors-only refresh, stored at the date-only hour, is not flagged as a duplicate at another time", async () => {
    const { client: c } = client(() => ({
      shows: [stored(1, "2026-10-10T03:30:00Z", { music_at: "2026-10-10T03:30:00Z" })],
      total: 1,
    }));
    await reportSameRoomOtherTime(
      c,
      [plan({ input: { event_date: "2026-10-09", doors_at: "7:30 PM" }, duplicate: { isDuplicate: true, existingShowId: 1 } })],
      pacific,
      () => "2026-10-10T03:00:00.000Z",
    );
    expect(warnings).toEqual([]);
  });

  test("a timed refresh of a show stored with no start is not flagged: --confirm fills its time", async () => {
    const { client: c } = client(() => ({ shows: [stored(1, "2026-10-10T03:00:00Z")], total: 1 }));
    await reportSameRoomOtherTime(
      c,
      [plan({ input: { event_date: "2026-10-09", music_at: "9:00 PM" }, duplicate: { isDuplicate: true, existingShowId: 1 } })],
      pacific,
      () => "2026-10-10T04:00:00.000Z",
    );
    expect(warnings).toEqual([]);
  });

  test("flags two listings in one run at one room, one day, one headliner, at two times", async () => {
    const { client: c } = client(() => ({ shows: [] }));
    await reportSameRoomOtherTime(
      c,
      [plan(), plan({ input: { event_date: "2026-10-09", music_at: "3:00 PM" } })],
      pacific,
      (p) => (p.input.music_at === "3:00 PM" ? "2026-10-09T22:00:00.000Z" : "2026-10-10T03:00:00.000Z"),
    );
    expect(warnings).toHaveLength(2);
    expect(warnings[0]).toContain("show #2 in this run is at this room that day with this headliner and states 15:00 PDT");
    expect(warnings[1]).toContain("show #1 in this run");
  });

  test("a room or headliner this run would create is compared within the run only", async () => {
    const { client: c, calls } = client(() => ({ shows: [] }));
    await reportSameRoomOtherTime(
      c,
      [plan({ venues: [{ name: "New Room" }] }), plan({ artists: [{ name: "New Act" }] }), plan({ valid: false })],
      pacific,
      () => "2026-10-10T03:00:00.000Z",
    );
    expect(calls).toEqual([]);
    expect(warnings).toEqual([]);
  });

  test("reads one venue-local day once across the batch", async () => {
    const { client: c, calls } = client(() => ({ shows: [], total: 0 }));
    await reportSameRoomOtherTime(
      c,
      [plan(), plan({ artists: [{ id: 43, name: "Other", set_type: "headliner" }] })],
      pacific,
      () => "2026-10-10T03:00:00.000Z",
    );
    expect(calls).toHaveLength(1);
  });

  test("reads no more than four days at once", async () => {
    let inFlight = 0;
    let most = 0;
    const { client: c, calls } = client(async () => {
      inFlight++;
      most = Math.max(most, inFlight);
      await new Promise((resolve) => setTimeout(resolve, 5));
      inFlight--;
      return { shows: [], total: 0 };
    });
    const days = Array.from({ length: 10 }, (_, i) => `2026-10-${String(10 + i).padStart(2, "0")}T03:00:00.000Z`);
    await reportSameRoomOtherTime(
      c,
      days.map((d) => plan({ input: { event_date: d } })),
      pacific,
      (p) => p.input.event_date,
    );
    expect(calls).toHaveLength(10);
    expect(most).toBeLessThanOrEqual(4);
    expect(most).toBeGreaterThan(1);
  });

  test("a failed read is reported as not checked and never throws", async () => {
    const { client: c } = client(() => {
      throw new Error("HTTP 503");
    });
    await reportSameRoomOtherTime(c, [plan()], pacific, () => "2026-10-10T03:00:00.000Z");
    expect(warnings).toEqual(["MARIS on 2026-10-09 at The Casbah: could not read that day's shows (HTTP 503); not checked"]);
  });

  test("control and bidi characters in stored slugs are replaced", async () => {
    const { client: c } = client(() => ({
      shows: [stored(1, "2026-10-10T03:00:00Z", { slug: "bad\u202eslug\u001b[2K" })],
      total: 1,
    }));
    await reportSameRoomOtherTime(c, [plan()], pacific, () => "2026-10-10T03:30:00.000Z");
    expect(warnings[0]).toContain("bad?slug?[2K");
  });

  // The real call site in submitShows.
  async function submit(confirm: boolean) {
    const { submitShows } = await import("../src/commands/submit-show");
    const { client: c, calls } = client((path, params) => {
      if (path.includes("/artists/search")) return { artists: [{ id: 42, name: "MARIS", slug: "maris" }] };
      if (path.includes("/venues/search")) {
        return {
          venues: [{ id: 7, name: "The Casbah", slug: "the-casbah", city: "San Diego", state: "CA", timezone: LA }],
        };
      }
      if (path === "/venues/7") return { id: 7, city: "San Diego", state: "CA" };
      if (path === "/shows" && params?.from_date) {
        return { shows: [stored(1, "2026-10-10T03:00:00Z", { music_at: "2026-10-10T03:00:00Z" })], total: 1 };
      }
      return { shows: [] };
    });
    (c as unknown as Record<string, unknown>).post = async () => ({ id: 2 });
    (c as unknown as Record<string, unknown>).put = async () => ({});
    const json = JSON.stringify({
      event_date: "2026-10-09",
      music_at: "8:30 PM",
      city: "San Diego",
      state: "CA",
      artists: [{ name: "MARIS", set_type: "headliner" }],
      venues: [{ name: "The Casbah", city: "San Diego", state: "CA" }],
    });
    const result = await submitShows(c, json, confirm);
    return { calls, result };
  }

  test("submitShows: today a same-room listing at another time is skipped as a duplicate, and the dry run says so", async () => {
    const { result } = await submit(false);
    expect(result.plans[0].duplicate?.isDuplicate).toBe(true);
    expect(warnings.some((w) => w.includes("ph skips this listing as a copy of show-1, which is at 20:00 PDT"))).toBe(true);
    expect(warnings.some((w) => w.includes("this listing says 20:30 PDT"))).toBe(true);
  });

  test("submitShows: a confirmed run does not run the check", async () => {
    await submit(true);
    expect(warnings.some((w) => w.includes("ph skips this listing"))).toBe(false);
  });
});
