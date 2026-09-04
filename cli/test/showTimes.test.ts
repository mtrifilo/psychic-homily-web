import { describe, test, expect } from "bun:test";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import {
  parseClockTime,
  readEventDate,
  resolveShowTimes,
} from "../src/lib/showTimes";

// -- parseClockTime ----------------------------------------------------------

describe("parseClockTime", () => {
  // Every shape a registered venue source is known to emit, named for the
  // source that emits it, plus the raw-passthrough forms a formatter leaks.
  const reads: Array<[string, number, number, string]> = [
    ["7:30PM", 19, 30, "lh-st .doorsTime"],
    ["8:30PM", 20, 30, "lh-st .showTime"],
    ["9:00PM", 21, 0, "empty bottle .start-time"],
    ["6:30PM", 18, 30, "cactus club .eventTime"],
    ["6:30 pm", 18, 30, "ticketweb parseTime"],
    ["7:00 PM", 19, 0, "seetickets normalized"],
    ["19:00", 19, 0, "thalia dates.start.localTime"],
    ["12:00 PM", 12, 0, "noon"],
    ["12:00 AM", 0, 0, "midnight"],
    ["7:00 p.m.", 19, 0, "raw passthrough with periods"],
    ["7:00 PM", 19, 0, "scraped HTML non-breaking space"],
    ["7:00\tPM", 19, 0, "tab"],
    ["23:45", 23, 45, "24-hour late"],
    ["00:30", 0, 30, "24-hour after midnight"],
  ];

  for (const [raw, hour, minute, why] of reads) {
    test(`reads ${JSON.stringify(raw)} (${why})`, () => {
      expect(parseClockTime(raw)).toEqual({ hour, minute });
    });
  }

  const refuses: Array<[unknown, string]> = [
    ["doors at 7", "prose, not a clock"],
    ["8PM", "no colon"],
    ["TBD", "not a time at all"],
    ["9pm - 1am", "a range names two times"],
    ["19:00 pm", "a 24-hour clock wearing a meridiem"],
    ["0:30 am", "hour 0 cannot carry a meridiem"],
    ["7:75 pm", "minute out of range"],
    ["25:00", "hour out of range"],
    ["7:00", "either half of the day"],
    ["12:00", "either half of the day"],
    ["", "nothing stated"],
    [undefined, "absent"],
    [1900, "a number is not a stated clock"],
  ];

  for (const [raw, why] of refuses) {
    test(`refuses ${JSON.stringify(raw)} (${why})`, () => {
      expect(parseClockTime(raw)).toBeNull();
    });
  }

  test("agrees with the pipeline's own parser table, case for case", () => {
    // Drift gate. Two ingest paths write shows.doors_at / music_at, and they
    // must read the same grammar or one publishes a time the other refuses.
    // The cases are PARSED out of the Go table rather than restated here, so a
    // row that changes its verdict there fails this test.
    const goTest = readFileSync(
      join(import.meta.dir, "../../backend/internal/services/pipeline/discovery_test.go"),
      "utf-8",
    );
    const table = /\{"[^"]*",\s*"((?:[^"\\]|\\.)*)",\s*(\d+),\s*(\d+),\s*(true|false)\},/g;
    const rows: Array<[string, number, number, boolean]> = [];
    for (const m of goTest.matchAll(table)) {
      rows.push([JSON.parse(`"${m[1]}"`), Number(m[2]), Number(m[3]), m[4] === "true"]);
    }
    // The table is the corpus this parser exists to agree with, so an empty or
    // truncated read would pass every assertion below without checking one. The
    // count is EXACT: a row deleted on the Go side is a case this parser stops
    // being held to, which is the drift this gate exists to catch. If the Go
    // table legitimately grew or shrank, read the new rows and change this
    // number deliberately.
    expect(rows.length).toBe(27);
    expect(rows.some(([, , , ok]) => ok)).toBe(true);
    expect(rows.some(([, , , ok]) => !ok)).toBe(true);

    for (const [raw, hour, minute, ok] of rows) {
      const parsed = parseClockTime(raw);
      if (!ok) {
        expect(parsed).toBeNull();
      } else {
        expect(parsed).toEqual({ hour, minute });
      }
    }
  });
});

// -- calendarDateOf ----------------------------------------------------------

describe("readEventDate", () => {
  const chicago = "America/Chicago";

  test("a bare day is a day", () => {
    expect(readEventDate("2026-09-04", chicago)).toEqual({
      kind: "day",
      day: "2026-09-04",
    });
  });

  test("a naive time is venue-local", () => {
    expect(readEventDate("2026-09-04T21:00", chicago)).toEqual({
      kind: "local",
      day: "2026-09-04",
      time: "21:00",
    });
    expect(readEventDate("2026-09-04T21:00:30", chicago)).toEqual({
      kind: "local",
      day: "2026-09-04",
      time: "21:00",
    });
  });

  test("a zoned timestamp names its day in the VENUE's zone, not in UTC", () => {
    // 2026-09-06T01:00Z is 8 PM on September 5 in Chicago. Reading the UTC day
    // would anchor the show's clocks a full day late.
    expect(readEventDate("2026-09-06T01:00:00Z", chicago)).toEqual({
      kind: "instant",
      day: "2026-09-05",
    });
    expect(readEventDate("2026-09-06T01:00:00Z", "Europe/London")).toEqual({
      kind: "instant",
      day: "2026-09-06",
    });
  });

  test("an offset timestamp reads the same way", () => {
    expect(readEventDate("2026-09-05T20:00:00-05:00", chicago)).toEqual({
      kind: "instant",
      day: "2026-09-05",
    });
  });

  const unreadable = [
    "Sep 4 2026",
    "",
    "2026-09-05 ",
    " 2026-09-05",
    "2026-09-05T",
    "2026-09-05 20:00",
    "2026-09-05T25:00",
    "2026-09-05T20:61",
    "2026-02-31",
    "2026-13-45",
  ];

  for (const raw of unreadable) {
    test(`refuses ${JSON.stringify(raw)}`, () => {
      expect(readEventDate(raw, chicago)).toBeNull();
    });
  }

  test("refuses a value that is not a string", () => {
    expect(readEventDate(undefined, chicago)).toBeNull();
    expect(readEventDate(20260904, chicago)).toBeNull();
  });
});

// -- resolveShowTimes --------------------------------------------------------

describe("resolveShowTimes", () => {
  const chicago = { eventDate: "2026-09-04", state: "IL", timezone: "America/Chicago" };

  test("writes both instants for a stated pair, anchored in the venue's zone", () => {
    // 2026-09-04 is CDT (UTC-5).
    const times = resolveShowTimes({ ...chicago, statedDoors: "7:30PM", statedMusic: "8:30PM" });
    expect(times.doorsAt).toBe("2026-09-05T00:30:00Z");
    expect(times.musicAt).toBe("2026-09-05T01:30:00Z");
    expect(times.refusals).toEqual([]);
  });

  test("reads the same wall clock differently on either side of DST", () => {
    const summer = resolveShowTimes({
      eventDate: "2026-07-15",
      state: "CA",
      timezone: "America/Los_Angeles",
      statedDoors: undefined,
      statedMusic: "8:00 PM",
    });
    const winter = resolveShowTimes({
      eventDate: "2026-01-15",
      state: "CA",
      timezone: "America/Los_Angeles",
      statedDoors: undefined,
      statedMusic: "8:00 PM",
    });
    expect(summer.musicAt).toBe("2026-07-16T03:00:00Z");
    expect(winter.musicAt).toBe("2026-01-16T04:00:00Z");
  });

  test("prefers the venue's own zone over the state map", () => {
    const times = resolveShowTimes({
      eventDate: "2026-09-04",
      state: "AZ",
      timezone: "America/Chicago",
      statedDoors: undefined,
      statedMusic: "8:00 PM",
    });
    expect(times.musicAt).toBe("2026-09-05T01:00:00Z");
  });

  test("says nothing when the source stated nothing", () => {
    const times = resolveShowTimes({ ...chicago, statedDoors: undefined, statedMusic: undefined });
    expect(times).toEqual({ refusals: [] });
  });

  test("writes music alone when the source states only a show time", () => {
    const times = resolveShowTimes({ ...chicago, statedDoors: undefined, statedMusic: "8:30PM" });
    expect(times.musicAt).toBe("2026-09-05T01:30:00Z");
    expect(times.doorsAt).toBeUndefined();
    expect(times.refusals).toEqual([]);
  });

  test("refuses a doors-only listing and says why", () => {
    const times = resolveShowTimes({ ...chicago, statedDoors: "7:30PM", statedMusic: undefined });
    expect(times.doorsAt).toBeUndefined();
    expect(times.musicAt).toBeUndefined();
    expect(times.refusals).toEqual([{ reason: "doors-without-music", doors: "7:30PM" }]);
  });

  test("refuses both when the stated music time is unreadable", () => {
    const times = resolveShowTimes({ ...chicago, statedDoors: "7:30PM", statedMusic: "TBD" });
    expect(times.doorsAt).toBeUndefined();
    expect(times.musicAt).toBeUndefined();
    expect(times.refusals).toEqual([{ reason: "unreadable-music", music: "TBD" }]);
  });

  test("keeps a readable music time when only the door time is unreadable", () => {
    const times = resolveShowTimes({ ...chicago, statedDoors: "doors at 7", statedMusic: "8:30PM" });
    expect(times.musicAt).toBe("2026-09-05T01:30:00Z");
    expect(times.doorsAt).toBeUndefined();
    expect(times.refusals).toEqual([{ reason: "unreadable-doors", doors: "doors at 7" }]);
  });

  test("writes neither half of a contradictory pair", () => {
    const times = resolveShowTimes({ ...chicago, statedDoors: "11:00 PM", statedMusic: "12:00 AM" });
    expect(times.doorsAt).toBeUndefined();
    expect(times.musicAt).toBeUndefined();
    expect(times.refusals).toEqual([
      { reason: "music-before-doors", doors: "11:00 PM", music: "12:00 AM" },
    ]);
  });

  test("accepts an equal pair, which is a schedule and not a contradiction", () => {
    const times = resolveShowTimes({ ...chicago, statedDoors: "9:00PM", statedMusic: "9:00PM" });
    expect(times.doorsAt).toBe("2026-09-05T02:00:00Z");
    expect(times.musicAt).toBe("2026-09-05T02:00:00Z");
  });

  test("refuses everything when no zone is known for the venue", () => {
    const times = resolveShowTimes({
      eventDate: "2026-09-04",
      state: "England",
      timezone: undefined,
      statedDoors: "7:30PM",
      statedMusic: "8:30PM",
    });
    expect(times.doorsAt).toBeUndefined();
    expect(times.musicAt).toBeUndefined();
    expect(times.refusals).toEqual([{ reason: "no-timezone" }]);
  });

  test("a malformed venue zone falls through to the state map rather than refusing", () => {
    const times = resolveShowTimes({
      eventDate: "2026-09-04",
      state: "IL",
      timezone: "Not/AZone",
      statedDoors: undefined,
      statedMusic: "8:30PM",
    });
    expect(times.musicAt).toBe("2026-09-05T01:30:00Z");
  });

  test("refuses when the event date states no calendar day", () => {
    const times = resolveShowTimes({
      eventDate: "next Friday",
      state: "IL",
      timezone: "America/Chicago",
      statedDoors: undefined,
      statedMusic: "8:30PM",
    });
    expect(times.musicAt).toBeUndefined();
    expect(times.refusals).toEqual([
      { reason: "no-calendar-day", eventDate: "next Friday" },
    ]);
  });
});

describe("impossible calendar days", () => {
  // Date.UTC overflow-normalizes, so a shape-only check would create the show on
  // a day nobody stated. Go's time.Parse rejects these; so must this.
  const impossible = ["2026-13-45", "2026-02-31", "2026-00-10", "2026-01-32"];

  for (const day of impossible) {
    test(`refuses ${day}`, () => {
      expect(readEventDate(day, "America/Chicago")).toBeNull();
    });
  }

  test("an impossible day stores no times and says why", () => {
    const times = resolveShowTimes({
      eventDate: "2026-02-31",
      state: "IL",
      timezone: "America/Chicago",
      statedDoors: "7:30PM",
      statedMusic: "8:30PM",
    });
    expect(times.doorsAt).toBeUndefined();
    expect(times.musicAt).toBeUndefined();
    expect(times.refusals).toEqual([
      { reason: "no-calendar-day", eventDate: "2026-02-31" },
    ]);
  });

  test("real days on either side still read", () => {
    expect(readEventDate("2026-02-28", "America/Chicago")).toEqual({
      kind: "day",
      day: "2026-02-28",
    });
    expect(readEventDate("2028-02-29", "America/Chicago")).toEqual({
      kind: "day",
      day: "2028-02-29",
    });
  });
});

describe("whitespace parity with unicode.IsSpace", () => {
  test("reads a clock separated by U+0085, which JS \\s does not match", () => {
    expect(parseClockTime("7:00\u0085PM")).toEqual({ hour: 19, minute: 0 });
  });

  test("refuses a clock separated by U+FEFF, which Go does not strip", () => {
    expect(parseClockTime("7:00\ufeffPM")).toBeNull();
  });

  test("reads the other unicode spaces a scrape can carry", () => {
    for (const sep of ["\u2009", "\u202f", "\u3000", "\u1680"]) {
      expect(parseClockTime(`7:00${sep}PM`)).toEqual({ hour: 19, minute: 0 });
    }
  });
});

describe("a value that is not a string", () => {
  test("is reported as unreadable rather than dropped", () => {
    const times = resolveShowTimes({
      eventDate: "2026-09-04",
      state: "IL",
      timezone: "America/Chicago",
      statedDoors: undefined,
      statedMusic: 1930,
    });
    expect(times.musicAt).toBeUndefined();
    expect(times.refusals).toEqual([{ reason: "unreadable-music", music: "1930" }]);
  });

  test("null is silence, not a statement", () => {
    const times = resolveShowTimes({
      eventDate: "2026-09-04",
      state: "IL",
      timezone: "America/Chicago",
      statedDoors: null,
      statedMusic: null,
    });
    expect(times).toEqual({ refusals: [] });
  });
});

describe("known limitations, pinned so they stay decisions", () => {
  test("a small-hours music time on a DATE-ONLY listing refuses rather than re-anchors", () => {
    // Owner decision 2026-09-04: below 06:00 the listing does not say which day
    // the clock belongs to, and anchoring event_date on it would move the show
    // ~20 hours earlier than the date-only convention puts it.
    const times = resolveShowTimes({
      eventDate: "2026-09-04",
      state: "IL",
      timezone: "America/Chicago",
      statedDoors: undefined,
      statedMusic: "12:30 AM",
    });
    expect(times.musicAt).toBeUndefined();
    expect(times.refusals).toEqual([
      { reason: "small-hours-music", music: "12:30 AM", cutoff: "06:00" },
    ]);
  });

  test("the same clock IS stored when the source states the day itself", () => {
    // event_date names its own time of day, so there is no re-anchor to refuse
    // and no ambiguity about which day the clock sits on.
    const times = resolveShowTimes({
      eventDate: "2026-09-05T00:30",
      state: "IL",
      timezone: "America/Chicago",
      statedDoors: undefined,
      statedMusic: "12:30 AM",
    });
    expect(times.musicAt).toBe("2026-09-05T05:30:00Z");
    expect(times.refusals).toEqual([]);
  });

  test("a clock inside a spring-forward gap is refused, and names the doors time it takes with it", () => {
    // 2:30 AM does not happen on 2026-03-08 in Chicago. localTimeToUTC still
    // returns Go's instant, which reads back as 1:30 AM; storing that would
    // publish a clock the source did not print.
    const times = resolveShowTimes({
      eventDate: "2026-03-08T02:30",
      state: "IL",
      timezone: "America/Chicago",
      statedDoors: "1:30 AM",
      statedMusic: "2:30 AM",
    });
    expect(times.musicAt).toBeUndefined();
    expect(times.doorsAt).toBeUndefined();
    expect(times.refusals).toEqual([
      {
        reason: "clock-does-not-exist",
        clock: "2:30 AM",
        day: "2026-03-08",
        alsoDropped: "1:30 AM",
      },
    ]);
  });

  test("a clock inside a fall-back repeat is stored, because it did happen", () => {
    const times = resolveShowTimes({
      eventDate: "2026-11-01T01:30",
      state: "IL",
      timezone: "America/Chicago",
      statedDoors: undefined,
      statedMusic: "1:30 AM",
    });
    expect(times.musicAt).toBe("2026-11-01T06:30:00Z");
  });

  test("a contradictory pair is diagnosed as one, not as whichever rule it also trips", () => {
    // Both clocks are small-hours-adjacent and the pair is inverted; the
    // inversion is the more specific fact and is what gets reported.
    const times = resolveShowTimes({
      eventDate: "2026-09-04",
      state: "IL",
      timezone: "America/Chicago",
      statedDoors: "11:00 PM",
      statedMusic: "12:00 AM",
    });
    expect(times.refusals).toEqual([
      { reason: "music-before-doors", doors: "11:00 PM", music: "12:00 AM" },
    ]);
  });
});
