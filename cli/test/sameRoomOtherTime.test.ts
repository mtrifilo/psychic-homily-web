import { describe, test, expect, spyOn, afterEach, afterAll } from "bun:test";
import {
  sameRoomShows,
  reportSameRoomOtherTime,
  storedHeadlinerId,
  localClock,
  gapReading,
  type SameRoomPlan,
  type StoredShowForSameRoom,
} from "../src/lib/sameRoomOtherTime";
import { APIClient } from "../src/lib/api";
import * as display from "../src/lib/display";

const LA = "America/Los_Angeles";

// A listing that states its music time; each test passes the start it is
// written with.
const plan = (over: Partial<SameRoomPlan> = {}): SameRoomPlan => ({
  input: { event_date: "2026-10-09", music_at: "8:30 PM" },
  artists: [{ id: 42, name: "MARIS", set_type: "headliner" }],
  venues: [{ id: 7, name: "The Casbah" }],
  valid: true,
  ...over,
});

const stored = (id: number, event_date: string, over: Partial<StoredShowForSameRoom> = {}): StoredShowForSameRoom => ({
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
  test("splits the headliner's shows at the room by their start, music time first", () => {
    const { sameInstant, otherTime } = sameRoomShows(7, 42, "2026-10-10T03:30:00.000Z", [
      stored(1, "2026-10-10T03:00:00Z"),
      stored(2, "2026-10-10T03:30:00Z"),
      stored(3, "2026-10-10T03:00:00Z", { music_at: "2026-10-10T03:30:00Z" }),
    ]);
    expect(otherTime.map((s) => s.id)).toEqual([1]);
    expect(sameInstant.map((s) => s.id)).toEqual([2, 3]);
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

describe("localClock and gapReading", () => {
  test("reads an instant in the venue's zone", () => {
    expect(localClock("2026-10-10T03:30:00Z", LA)).toBe("20:30 PDT");
  });

  test("returns an unreadable instant cleaned", () => {
    expect(localClock(`bad${String.fromCharCode(27)}[2K`, LA)).toBe("bad?[2K");
  });

  test("reads a gap under 90 minutes as the same show, 90 or more as a second show", () => {
    expect(gapReading(89)).toBe("89 minutes apart: probably the same show, stored at another time");
    expect(gapReading(90)).toBe("90 minutes apart: may be a second show the same day");
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
  const day = (shows: StoredShowForSameRoom[]) => () => ({ shows, total: shows.length });

  test("flags a listing at another time the same day, naming both starts and reading that day", async () => {
    const { client: c, calls } = client(day([stored(1, "2026-10-10T03:00:00Z")]));
    await reportSameRoomOtherTime(c, [plan()], pacific, () => "2026-10-10T03:30:00.000Z");
    expect(calls).toHaveLength(1);
    expect(calls[0].params?.from_date).toBe("2026-10-09T07:00:00Z");
    expect(calls[0].params?.to_date).toBe("2026-10-10T06:59:00Z");
    expect(warnings).toEqual([
      "MARIS on 2026-10-09 at The Casbah: show-1 is already at this room that day with this headliner, starting " +
        "20:00 PDT; this listing states 20:30 PDT, 30 minutes apart: probably the same show, stored at another time. " +
        "Check before --confirm",
    ]);
  });

  test("a listing identical to what is stored is not flagged, on its music time", async () => {
    // Stored with event_date 19:00 and music 20:30; the listing states the same.
    const { client: c } = client(day([stored(1, "2026-10-10T02:00:00Z", { music_at: "2026-10-10T03:30:00Z" })]));
    await reportSameRoomOtherTime(
      c,
      [plan({ duplicate: { isDuplicate: true, existingShowId: 1 } })],
      pacific,
      () => "2026-10-10T03:30:00.000Z",
    );
    expect(warnings).toEqual([]);
  });

  test("an evening listing across midnight UTC reads the venue-local day that holds the matinee", async () => {
    const { client: c, calls } = client(day([stored(1, "2026-12-06T23:00:00Z")]));
    await reportSameRoomOtherTime(
      c,
      [plan({ input: { event_date: "2026-12-06", music_at: "8:00 PM" } })],
      pacific,
      () => "2026-12-07T04:00:00.000Z",
    );
    expect(calls[0].params?.from_date).toBe("2026-12-06T08:00:00Z");
    expect(calls[0].params?.to_date).toBe("2026-12-07T07:59:00Z");
    expect(warnings).toHaveLength(1);
    expect(warnings[0]).toContain("starting 15:00 PST; this listing states 20:00 PST, 300 minutes apart: may be a second show the same day");
  });

  test("a duplicate ph skips is flagged when the show it matched starts at another time", async () => {
    const { client: c } = client(day([stored(1, "2026-10-10T03:00:00Z", { music_at: "2026-10-10T03:00:00Z" })]));
    await reportSameRoomOtherTime(
      c,
      [plan({ duplicate: { isDuplicate: true, existingShowId: 1 } }), plan({ duplicate: { isDuplicate: true, existingShowId: 9 } })],
      pacific,
      () => "2026-10-10T03:30:00.000Z",
    );
    expect(warnings).toHaveLength(1);
    expect(warnings[0]).toContain("ph skips this listing as a copy of show-1, which starts 20:00 PDT where this listing states 20:30 PDT");
    expect(warnings[0]).toContain("probably the same show, stored at another time");
  });

  test("a matinee listing matched to an untimed 8pm stored show reads as a second show", async () => {
    const { client: c } = client(day([stored(1, "2026-10-10T03:00:00Z")]));
    await reportSameRoomOtherTime(
      c,
      [plan({ input: { event_date: "2026-10-09", music_at: "3:00 PM" }, duplicate: { isDuplicate: true, existingShowId: 1 } })],
      pacific,
      () => "2026-10-09T22:00:00.000Z",
    );
    expect(warnings).toHaveLength(1);
    expect(warnings[0]).toContain("300 minutes apart: may be a second show the same day. If it is a second show, ph cannot add it");
  });

  test("a duplicate is not flagged when a stored show already starts at the listing's own start", async () => {
    const { client: c } = client(day([stored(1, "2026-10-09T22:00:00Z"), stored(2, "2026-10-10T03:30:00Z")]));
    await reportSameRoomOtherTime(
      c,
      [plan({ duplicate: { isDuplicate: true, existingShowId: 1 } })],
      pacific,
      () => "2026-10-10T03:30:00.000Z",
    );
    expect(warnings).toEqual([]);
  });

  test("a listing that states no start is compared only when it is 90 minutes or more from a stored start", async () => {
    const near = client(day([stored(1, "2026-10-10T04:00:00Z", { music_at: "2026-10-10T04:00:00Z" })]));
    await reportSameRoomOtherTime(
      near.client,
      [plan({ input: { event_date: "2026-10-09" }, duplicate: { isDuplicate: true, existingShowId: 1 } })],
      pacific,
      () => "2026-10-10T03:00:00.000Z",
    );
    expect(warnings).toEqual([]);

    const far = client(day([stored(1, "2026-10-09T22:00:00Z", { music_at: "2026-10-09T22:00:00Z" })]));
    await reportSameRoomOtherTime(far.client, [plan({ input: { event_date: "2026-10-09" } })], pacific, () => "2026-10-10T03:00:00.000Z");
    expect(warnings).toHaveLength(1);
    expect(warnings[0]).toContain("this listing states no start time (it is stored at 20:00 PDT), 300 minutes apart");
  });

  test("a stored show's music time is the start compared and printed", async () => {
    const { client: c } = client(day([stored(1, "2026-10-10T03:00:00Z", { music_at: "2026-10-09T21:00:00Z" })]));
    await reportSameRoomOtherTime(c, [plan()], pacific, () => "2026-10-10T03:30:00.000Z");
    expect(warnings).toHaveLength(1);
    expect(warnings[0]).toContain("starting 14:00 PDT (its music time; it is stored at 20:00 PDT)");
    expect(warnings[0]).toContain("390 minutes apart");
  });

  test("flags two listings in one run at one room, one day, one headliner, at two starts", async () => {
    const { client: c } = client(() => ({ shows: [] }));
    await reportSameRoomOtherTime(
      c,
      [plan(), plan({ input: { event_date: "2026-10-09", music_at: "3:00 PM" } })],
      pacific,
      (p) => (p.input.music_at === "3:00 PM" ? "2026-10-09T22:00:00.000Z" : "2026-10-10T03:30:00.000Z"),
    );
    expect(warnings).toHaveLength(2);
    expect(warnings[0]).toContain("show #2 in this run is at this room that day with this headliner and states 15:00 PDT");
    expect(warnings[0]).toContain("330 minutes apart: may be a second show the same day");
    expect(warnings[1]).toContain("show #1 in this run");
  });

  test("the in-run check matches a new room and a new act by name, and leaves out duplicates", async () => {
    const { client: c } = client(() => ({ shows: [] }));
    const fresh = (music: string, over: Partial<SameRoomPlan> = {}) =>
      plan({
        input: { event_date: "2026-10-09", music_at: music },
        artists: [{ name: "Brand New Act", set_type: "headliner" }],
        venues: [{ name: "New Room" }],
        ...over,
      });
    const starts: Record<string, string> = {
      "3:00 PM": "2026-10-09T22:00:00.000Z",
      "8:30 PM": "2026-10-10T03:30:00.000Z",
      "9:00 PM": "2026-10-10T04:00:00.000Z",
    };
    await reportSameRoomOtherTime(
      c,
      [fresh("3:00 PM"), fresh("8:30 PM", { venues: [{ name: " new room " }] }), fresh("9:00 PM", { duplicate: { isDuplicate: true } })],
      pacific,
      (p) => starts[p.input.music_at!],
    );
    expect(warnings).toHaveLength(2);
    expect(warnings[0]).toContain("show #2 in this run");
    expect(warnings[1]).toContain("show #1 in this run");
    expect(warnings.some((w) => w.includes("show #3"))).toBe(false);
  });

  test("a room or headliner this run would create is not read from the catalog", async () => {
    const { client: c, calls } = client(() => ({ shows: [] }));
    await reportSameRoomOtherTime(
      c,
      [plan({ venues: [{ name: "New Room" }] }), plan({ artists: [{ name: "New Act" }] }), plan({ valid: false })],
      pacific,
      () => "2026-10-10T03:30:00.000Z",
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
      () => "2026-10-10T03:30:00.000Z",
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
    const days = Array.from({ length: 10 }, (_, i) => `2026-10-${String(10 + i).padStart(2, "0")}T03:30:00.000Z`);
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

  test("says so when a day holds more shows than the read cap", async () => {
    const page = Array.from({ length: 200 }, (_, i) => stored(1000 + i, "2026-10-10T03:00:00Z", { venues: [] }));
    const { client: c, calls } = client(() => ({ shows: page, total: 900 }));
    await reportSameRoomOtherTime(c, [plan()], pacific, () => "2026-10-10T03:30:00.000Z");
    expect(calls).toHaveLength(3);
    expect(warnings).toEqual([
      "MARIS on 2026-10-09 at The Casbah: read the first 600 shows that day and stopped; the rest were not checked",
    ]);
  });

  test("reads a bare-array response, ending on a short page", async () => {
    const { client: c, calls } = client(() => [stored(1, "2026-10-10T03:00:00Z")]);
    await reportSameRoomOtherTime(c, [plan()], pacific, () => "2026-10-10T03:30:00.000Z");
    expect(calls).toHaveLength(1);
    expect(warnings).toHaveLength(1);
  });

  test("a failed read is reported as not checked and never throws", async () => {
    const { client: c } = client(() => {
      throw new Error("HTTP 503");
    });
    await reportSameRoomOtherTime(c, [plan()], pacific, () => "2026-10-10T03:30:00.000Z");
    expect(warnings).toEqual(["MARIS on 2026-10-09 at The Casbah: could not read that day's shows (HTTP 503); not checked"]);
  });

  test("control and bidi characters in stored slugs are replaced", async () => {
    const { client: c } = client(day([stored(1, "2026-10-10T03:00:00Z", { slug: `bad${String.fromCharCode(0x202e)}slug${String.fromCharCode(27)}[2K` })]));
    await reportSameRoomOtherTime(c, [plan()], pacific, () => "2026-10-10T03:30:00.000Z");
    expect(warnings[0]).toContain("bad?slug?[2K");
  });

  // The real call site in submitShows.
  async function submit(confirm: boolean, storedShow: StoredShowForSameRoom) {
    const { submitShows } = await import("../src/commands/submit-show");
    const { client: c, calls } = client((path, params) => {
      if (path.includes("/artists/search")) return { artists: [{ id: 42, name: "MARIS", slug: "maris" }] };
      if (path.includes("/venues/search")) {
        return { venues: [{ id: 7, name: "The Casbah", slug: "the-casbah", city: "San Diego", state: "CA", timezone: LA }] };
      }
      if (path === "/venues/7") return { id: 7, city: "San Diego", state: "CA" };
      if (path === "/shows" && params?.from_date) return { shows: [storedShow], total: 1 };
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
    return { calls, result: await submitShows(c, json, confirm) };
  }

  test("submitShows: a same-room listing at another time is skipped as a duplicate, and the dry run says so", async () => {
    const { result } = await submit(false, stored(1, "2026-10-10T03:00:00Z", { music_at: "2026-10-10T03:00:00Z" }));
    expect(result.plans[0].duplicate?.isDuplicate).toBe(true);
    expect(warnings.some((w) => w.includes("ph skips this listing as a copy of show-1, which starts 20:00 PDT where this listing states 20:30 PDT"))).toBe(true);
  });

  test("submitShows: the listing that wrote a show compares equal to it on the next run", async () => {
    // What this listing writes: event_date and music_at both 20:30.
    await submit(false, stored(1, "2026-10-10T03:30:00Z", { music_at: "2026-10-10T03:30:00Z" }));
    expect(warnings.some((w) => w.includes("ph skips this listing"))).toBe(false);
  });

  test("submitShows: a confirmed run does not run the check", async () => {
    await submit(true, stored(1, "2026-10-10T03:00:00Z", { music_at: "2026-10-10T03:00:00Z" }));
    expect(warnings.some((w) => w.includes("ph skips this listing"))).toBe(false);
  });
});
