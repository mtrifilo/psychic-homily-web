import { describe, test, expect } from "bun:test";
import {
  aggregateRematchResults,
  dedupeArtistNames,
} from "../src/lib/radio";

describe("dedupeArtistNames", () => {
  test("removes duplicates preserving order", () => {
    expect(dedupeArtistNames(["Metric", "Metric", "Boy Harsher"])).toEqual([
      "Metric",
      "Boy Harsher",
    ]);
  });

  test("trims whitespace and drops empties", () => {
    expect(dedupeArtistNames(["  Metric  ", "", "   "])).toEqual(["Metric"]);
  });
});

describe("aggregateRematchResults", () => {
  test("sums totals across per-name results", () => {
    const agg = aggregateRematchResults([
      { total: 10, matched: 3, unmatched: 7 },
      { total: 5, matched: 2, unmatched: 3, persist_errors: 1 },
    ]);
    expect(agg).toEqual({
      total: 15,
      matched: 5,
      unmatched: 10,
      persist_errors: 1,
    });
  });

  test("returns zeros for empty input", () => {
    expect(aggregateRematchResults([])).toEqual({
      total: 0,
      matched: 0,
      unmatched: 0,
      persist_errors: 0,
    });
  });
});
