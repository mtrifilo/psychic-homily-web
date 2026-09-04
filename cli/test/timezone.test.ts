import { describe, test, expect } from "bun:test";
import { getTimezoneForState, localTimeToUTC } from "../src/lib/timezone";

describe("getTimezoneForState", () => {
  test("Arizona returns America/Phoenix", () => {
    expect(getTimezoneForState("AZ")).toBe("America/Phoenix");
  });

  test("California returns America/Los_Angeles", () => {
    expect(getTimezoneForState("CA")).toBe("America/Los_Angeles");
  });

  test("New York returns America/New_York", () => {
    expect(getTimezoneForState("NY")).toBe("America/New_York");
  });

  test("Texas returns America/Chicago", () => {
    expect(getTimezoneForState("TX")).toBe("America/Chicago");
  });

  test("Colorado returns America/Denver", () => {
    expect(getTimezoneForState("CO")).toBe("America/Denver");
  });

  test("case insensitive", () => {
    expect(getTimezoneForState("az")).toBe("America/Phoenix");
    expect(getTimezoneForState("ca")).toBe("America/Los_Angeles");
  });

  test("unknown state defaults to America/Phoenix", () => {
    expect(getTimezoneForState("XX")).toBe("America/Phoenix");
  });
});

describe("localTimeToUTC", () => {
  test("Arizona 8pm = 3am UTC next day (UTC-7, no DST)", () => {
    const result = localTimeToUTC("2026-04-15", "20:00", "America/Phoenix");
    expect(result).toBe("2026-04-16T03:00:00Z");
  });

  test("Arizona 8pm in winter = 3am UTC next day (still UTC-7)", () => {
    // Arizona doesn't observe DST
    const result = localTimeToUTC("2026-01-15", "20:00", "America/Phoenix");
    expect(result).toBe("2026-01-16T03:00:00Z");
  });

  test("Los Angeles summer (PDT, UTC-7): 8pm = 3am UTC", () => {
    const result = localTimeToUTC("2026-07-15", "20:00", "America/Los_Angeles");
    expect(result).toBe("2026-07-16T03:00:00Z");
  });

  test("Los Angeles winter (PST, UTC-8): 8pm = 4am UTC", () => {
    const result = localTimeToUTC("2026-01-15", "20:00", "America/Los_Angeles");
    expect(result).toBe("2026-01-16T04:00:00Z");
  });

  test("New York summer (EDT, UTC-4): 8pm = midnight UTC", () => {
    const result = localTimeToUTC("2026-07-15", "20:00", "America/New_York");
    expect(result).toBe("2026-07-16T00:00:00Z");
  });

  test("New York winter (EST, UTC-5): 8pm = 1am UTC", () => {
    const result = localTimeToUTC("2026-01-15", "20:00", "America/New_York");
    expect(result).toBe("2026-01-16T01:00:00Z");
  });

  test("Chicago summer (CDT, UTC-5): 8pm = 1am UTC", () => {
    const result = localTimeToUTC("2026-07-15", "20:00", "America/Chicago");
    expect(result).toBe("2026-07-16T01:00:00Z");
  });

  test("handles HH:MM:SS format", () => {
    const result = localTimeToUTC("2026-04-15", "19:30:00", "America/Phoenix");
    expect(result).toBe("2026-04-16T02:30:00Z");
  });

  test("handles HH:MM format", () => {
    const result = localTimeToUTC("2026-04-15", "19:30", "America/Phoenix");
    expect(result).toBe("2026-04-16T02:30:00Z");
  });

  test("midnight local = offset hours UTC", () => {
    // Midnight Phoenix = 7am UTC
    const result = localTimeToUTC("2026-04-15", "00:00", "America/Phoenix");
    expect(result).toBe("2026-04-15T07:00:00Z");
  });
});

describe("localTimeToUTC across DST transitions", () => {
  // Every expectation is the instant Go's time.Date(y, m, d, h, min, 0, 0, loc)
  // produces for the same wall clock and zone. The pipeline anchors its own
  // doors/music instants that way, so a clock a venue prints on a transition
  // night has to land on the same second through either ingest path.
  const rows: Array<[string, string, string, string]> = [
    // Spring forward, America/Chicago at 02:00 local.
    ["2026-03-08", "02:30", "America/Chicago", "2026-03-08T07:30:00Z"],
    ["2026-03-08", "03:00", "America/Chicago", "2026-03-08T08:00:00Z"],
    ["2026-03-08", "07:30", "America/Chicago", "2026-03-08T12:30:00Z"],
    // Fall back, America/Chicago at 02:00 local. 01:30 happens twice and
    // resolves to the first, like Go.
    ["2026-11-01", "00:30", "America/Chicago", "2026-11-01T05:30:00Z"],
    ["2026-11-01", "01:30", "America/Chicago", "2026-11-01T06:30:00Z"],
    ["2026-11-01", "02:30", "America/Chicago", "2026-11-01T08:30:00Z"],
    // A European club listing that states doors and music either side of the
    // 02:00 spring-forward. The two must stay an hour apart.
    ["2026-03-29", "00:30", "Europe/Berlin", "2026-03-28T23:30:00Z"],
    ["2026-03-29", "01:30", "Europe/Berlin", "2026-03-29T00:30:00Z"],
    // 02:30 does not exist that night; it normalizes forward, like Go.
    ["2026-03-29", "02:30", "Europe/Berlin", "2026-03-29T01:30:00Z"],
    ["2026-03-29", "03:30", "Europe/Berlin", "2026-03-29T01:30:00Z"],
    ["2026-10-25", "02:30", "Europe/Berlin", "2026-10-25T01:30:00Z"],
    ["2026-03-08", "02:00", "America/New_York", "2026-03-08T06:00:00Z"],
  ];

  for (const [date, time, zone, expected] of rows) {
    test(`${date} ${time} ${zone} -> ${expected}`, () => {
      expect(localTimeToUTC(date, time, zone)).toBe(expected);
    });
  }

  test("a stated doors/music pair stays an hour apart across the gap", () => {
    const doors = localTimeToUTC("2026-03-29", "00:30", "Europe/Berlin");
    const music = localTimeToUTC("2026-03-29", "01:30", "Europe/Berlin");
    expect(Date.parse(music) - Date.parse(doors)).toBe(60 * 60 * 1000);
  });
});
