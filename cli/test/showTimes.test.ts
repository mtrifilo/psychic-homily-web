import { describe, test, expect } from "bun:test";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import {
  parseClockTime,
  calendarDateOf,
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

  test("refuses every string the pipeline's own parser table pins as unreadable", () => {
    // Drift gate. Two ingest paths write shows.doors_at / music_at, and they
    // must read the same grammar or one publishes a time the other refuses.
    // The strings are located IN the Go test rather than restated here, so a
    // case deleted there fails this test instead of leaving a stale mirror.
    const goTest = readFileSync(
      join(import.meta.dir, "../../backend/internal/services/pipeline/discovery_test.go"),
      "utf-8",
    );
    const refusedByGo = ["doors at 7", "8PM", "19:00 pm", "7:00", "25:00", "7:75 pm"];
    for (const raw of refusedByGo) {
      expect(goTest).toContain(`"${raw}"`);
      expect(parseClockTime(raw)).toBeNull();
    }
  });
});

// -- calendarDateOf ----------------------------------------------------------

describe("calendarDateOf", () => {
  test("reads a date-only string", () => {
    expect(calendarDateOf("2026-09-04")).toBe("2026-09-04");
  });

  test("reads the day out of a full timestamp", () => {
    expect(calendarDateOf("2026-09-04T20:00:00Z")).toBe("2026-09-04");
  });

  test("refuses anything that does not start with a calendar day", () => {
    expect(calendarDateOf("Sep 4 2026")).toBeNull();
    expect(calendarDateOf("")).toBeNull();
    expect(calendarDateOf(undefined)).toBeNull();
  });
});

// -- resolveShowTimes --------------------------------------------------------

describe("resolveShowTimes", () => {
  const chicago = { eventDate: "2026-09-04", state: "IL", timezone: "America/Chicago" };

  test("writes both instants for a stated pair, anchored in the venue's zone", () => {
    // 2026-09-04 is CDT (UTC-5).
    const times = resolveShowTimes({ ...chicago, doorsAt: "7:30PM", musicAt: "8:30PM" });
    expect(times.doorsAt).toBe("2026-09-05T00:30:00Z");
    expect(times.musicAt).toBe("2026-09-05T01:30:00Z");
    expect(times.notes).toEqual([]);
  });

  test("reads the same wall clock differently on either side of DST", () => {
    const summer = resolveShowTimes({
      eventDate: "2026-07-15",
      state: "CA",
      timezone: "America/Los_Angeles",
      doorsAt: undefined,
      musicAt: "8:00 PM",
    });
    const winter = resolveShowTimes({
      eventDate: "2026-01-15",
      state: "CA",
      timezone: "America/Los_Angeles",
      doorsAt: undefined,
      musicAt: "8:00 PM",
    });
    expect(summer.musicAt).toBe("2026-07-16T03:00:00Z");
    expect(winter.musicAt).toBe("2026-01-16T04:00:00Z");
  });

  test("prefers the venue's own zone over the state map", () => {
    const times = resolveShowTimes({
      eventDate: "2026-09-04",
      state: "AZ",
      timezone: "America/Chicago",
      doorsAt: undefined,
      musicAt: "8:00 PM",
    });
    expect(times.musicAt).toBe("2026-09-05T01:00:00Z");
  });

  test("says nothing when the source stated nothing", () => {
    const times = resolveShowTimes({ ...chicago, doorsAt: undefined, musicAt: undefined });
    expect(times).toEqual({ notes: [] });
  });

  test("writes music alone when the source states only a show time", () => {
    const times = resolveShowTimes({ ...chicago, doorsAt: undefined, musicAt: "8:30PM" });
    expect(times.musicAt).toBe("2026-09-05T01:30:00Z");
    expect(times.doorsAt).toBeUndefined();
    expect(times.notes).toEqual([]);
  });

  test("refuses a doors-only listing and says why", () => {
    const times = resolveShowTimes({ ...chicago, doorsAt: "7:30PM", musicAt: undefined });
    expect(times.doorsAt).toBeUndefined();
    expect(times.musicAt).toBeUndefined();
    expect(times.notes[0]).toContain("no music time");
  });

  test("refuses both when the stated music time is unreadable", () => {
    const times = resolveShowTimes({ ...chicago, doorsAt: "7:30PM", musicAt: "TBD" });
    expect(times.doorsAt).toBeUndefined();
    expect(times.musicAt).toBeUndefined();
    expect(times.notes[0]).toContain("TBD");
  });

  test("keeps a readable music time when only the door time is unreadable", () => {
    const times = resolveShowTimes({ ...chicago, doorsAt: "doors at 7", musicAt: "8:30PM" });
    expect(times.musicAt).toBe("2026-09-05T01:30:00Z");
    expect(times.doorsAt).toBeUndefined();
    expect(times.notes[0]).toContain("doors at 7");
  });

  test("writes neither half of a contradictory pair", () => {
    const times = resolveShowTimes({ ...chicago, doorsAt: "11:00 PM", musicAt: "12:00 AM" });
    expect(times.doorsAt).toBeUndefined();
    expect(times.musicAt).toBeUndefined();
    expect(times.notes[0]).toContain("before doors");
  });

  test("accepts an equal pair, which is a schedule and not a contradiction", () => {
    const times = resolveShowTimes({ ...chicago, doorsAt: "9:00PM", musicAt: "9:00PM" });
    expect(times.doorsAt).toBe("2026-09-05T02:00:00Z");
    expect(times.musicAt).toBe("2026-09-05T02:00:00Z");
  });

  test("refuses everything when no zone is known for the venue", () => {
    const times = resolveShowTimes({
      eventDate: "2026-09-04",
      state: "England",
      timezone: undefined,
      doorsAt: "7:30PM",
      musicAt: "8:30PM",
    });
    expect(times.doorsAt).toBeUndefined();
    expect(times.musicAt).toBeUndefined();
    expect(times.notes[0]).toContain("no timezone is known");
  });

  test("a malformed venue zone falls through to the state map rather than refusing", () => {
    const times = resolveShowTimes({
      eventDate: "2026-09-04",
      state: "IL",
      timezone: "Not/AZone",
      doorsAt: undefined,
      musicAt: "8:30PM",
    });
    expect(times.musicAt).toBe("2026-09-05T01:30:00Z");
  });

  test("refuses when the event date states no calendar day", () => {
    const times = resolveShowTimes({
      eventDate: "next Friday",
      state: "IL",
      timezone: "America/Chicago",
      doorsAt: undefined,
      musicAt: "8:30PM",
    });
    expect(times.musicAt).toBeUndefined();
    expect(times.notes[0]).toContain("calendar day");
  });
});
