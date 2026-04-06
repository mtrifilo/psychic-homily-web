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
