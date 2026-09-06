import { describe, test, expect } from "bun:test";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import {
  getTimezoneForState,
  localClockExists,
  localTimeToUTC,
} from "../src/lib/timezone";

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

// The corpus is read at RUNTIME rather than imported.
//
// `import ... from "../../backend/...json"` would pull a backend file into this
// package's TypeScript program, which `tsc --noEmit` and the compiled binary
// both walk. readFileSync keeps one shared source of truth for three languages
// while staying invisible to both. Same idiom as the Go-table read in
// test/showTimes.test.ts.
//
// The path is anchored on this file's own directory, so it resolves the same
// whatever directory `bun test` is invoked from.
const CORPUS_PATH = join(
  import.meta.dir,
  "../../backend/internal/utils/testdata/dst_clock_corpus.json",
);

type DSTCorpusCase = {
  date: string;
  clock: string;
  zone: string;
  utc: string;
  exists: boolean;
  ambiguous: boolean;
  why: string;
};

/**
 * The shared corpus rows, or a thrown error naming the file.
 *
 * A missing or malformed corpus fails loudly rather than yielding an empty
 * array: an empty array turns every generated test below into zero tests, so
 * the gate would stop asserting with nothing failing.
 */
function readDSTCorpus(): DSTCorpusCase[] {
  let raw: string;
  try {
    raw = readFileSync(CORPUS_PATH, "utf-8");
  } catch (cause) {
    throw new Error(
      `cannot read the shared DST clock corpus at ${CORPUS_PATH}; this suite asserts nothing without it`,
      { cause },
    );
  }
  const parsed = JSON.parse(raw) as { cases?: unknown };
  if (!Array.isArray(parsed.cases)) {
    throw new Error(
      `the DST clock corpus at ${CORPUS_PATH} has no "cases" array`,
    );
  }
  return parsed.cases as DSTCorpusCase[];
}

const dstCorpus = readDSTCorpus();

describe("the shared DST clock corpus", () => {
  // Drift gate. Three implementations resolve a venue-local wall clock to an
  // instant: Go's time.Date, the frontend resolver, and this one. All three
  // assert every row of this one file, so a clock printed on a transition night
  // lands on the same second whichever path writes it.
  test("carries the transition rows this resolver is held to", () => {
    // A FLOOR, not an exact count: a row added on any side is a case this
    // resolver starts being held to and needs no edit here, while a row removed
    // is a case it stops being held to, which is the drift worth failing on.
    expect(dstCorpus.length).toBeGreaterThanOrEqual(31);
    expect(dstCorpus.filter((c) => !c.exists).length).toBeGreaterThanOrEqual(2);
    expect(dstCorpus.filter((c) => c.ambiguous).length).toBeGreaterThanOrEqual(
      2,
    );
    expect(new Set(dstCorpus.map((c) => c.zone)).size).toBeGreaterThanOrEqual(8);
  });

  for (const c of dstCorpus) {
    const name = `${c.date} ${c.clock} ${c.zone} (${c.why})`;

    test(`${name} resolves to the instant Go does`, () => {
      expect(localTimeToUTC(c.date, c.clock, c.zone)).toBe(c.utc);
    });

    test(`${name} agrees about whether the clock exists`, () => {
      expect(localClockExists(c.date, c.clock, c.zone)).toBe(c.exists);
    });
  }
});

describe("what the transition rows buy", () => {
  test("a stated doors/music pair stays an hour apart across the gap", () => {
    const doors = localTimeToUTC("2026-03-29", "00:30", "Europe/Berlin");
    const music = localTimeToUTC("2026-03-29", "01:30", "Europe/Berlin");
    expect(Date.parse(music) - Date.parse(doors)).toBe(60 * 60 * 1000);
  });
});

describe("cases the corpus does not carry", () => {
  // Every corpus row states one HH:MM clock in one named zone. These are the
  // cases no row expresses, so they stay spelled out here rather than being
  // asked of a shared row.

  test("reads a HH:MM:SS clock", () => {
    expect(localTimeToUTC("2026-04-15", "19:30:00", "America/Phoenix")).toBe(
      "2026-04-16T02:30:00Z",
    );
  });

  test("reads a HH:MM clock as the same instant", () => {
    expect(localTimeToUTC("2026-04-15", "19:30", "America/Phoenix")).toBe(
      localTimeToUTC("2026-04-15", "19:30:00", "America/Phoenix"),
    );
  });

  test("reads midnight rather than hour 24", () => {
    // Intl renders venue-local midnight as hour 24 in some runtimes, which
    // would place the instant a day late.
    expect(localTimeToUTC("2026-04-15", "00:00", "America/Phoenix")).toBe(
      "2026-04-15T07:00:00Z",
    );
  });

  test("reads a summer evening in America/New_York", () => {
    // The corpus carries New York only for its spring-forward gap and a winter
    // night, so the daylight-time offset for that zone is asserted here.
    expect(localTimeToUTC("2026-07-15", "20:00", "America/New_York")).toBe(
      "2026-07-16T00:00:00Z",
    );
  });

  test("holds the state-map default zone still across the seasons", () => {
    // getTimezoneForState answers America/Phoenix for every state it does not
    // map, so an offset that moved with the season there would mis-time every
    // show whose state is unmapped.
    expect(localTimeToUTC("2026-07-15", "20:00", "America/Phoenix")).toBe(
      "2026-07-16T03:00:00Z",
    );
    expect(localTimeToUTC("2026-01-15", "20:00", "America/Phoenix")).toBe(
      "2026-01-16T03:00:00Z",
    );
  });
});
