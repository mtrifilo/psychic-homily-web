import { describe, test, expect } from "bun:test";
import {
  aggregateRematchResults,
  dedupeArtistNames,
  formatArtistCandidateReport,
  type ArtistCandidate,
  type ArtistCandidateListing,
} from "../src/lib/radio";

function candidate(overrides: Partial<ArtistCandidate>): ArtistCandidate {
  return {
    artist_key: "telehealth",
    display_name: "Telehealth",
    lane: "human",
    score: 87.7512,
    play_count: 131,
    station_row_count: 5,
    station_family_count: 2,
    show_count: 26,
    first_aired_on: "2026-04-20",
    last_aired_on: "2026-07-23",
    stations: [
      { station_id: 7, slug: "kexp", name: "KEXP", play_count: 100 },
      { station_id: 8, slug: "wfmu", name: "WFMU", play_count: 31 },
    ],
    top_tracks: [{ track_id: 1, title: "Content", play_count: 40 }],
    album_names: [{ name: "Content", play_count: 90 }],
    album_name_count: 1,
    label_names: [],
    label_name_count: 0,
    mb_artist_ids: [],
    mb_artist_id_conflict: false,
    ...overrides,
  };
}

const listing = (candidates: ArtistCandidate[]): ArtistCandidateListing => ({
  candidates,
  total: 6465,
  auto_total: 2961,
  human_total: 3504,
  as_of: "2026-10-14",
  window_start: "2026-04-18",
  window_days: 180,
  min_station_rows: 2,
  min_plays: 3,
});

describe("formatArtistCandidateReport", () => {
  test("prints the window, floors and totals, then one block per candidate", () => {
    const lines = formatArtistCandidateReport(listing([candidate({})]));
    expect(lines.slice(0, 3)).toEqual([
      "Artist candidates as of 2026-10-14: plays aired 2026-04-18 or later (180 days); floors 2 station rows and 3 plays.",
      "6465 in the requested lanes (2961 auto, 3504 human overall); showing 1.",
      "Top tracks reflect the radio_tracks rebuild of (none recorded).",
    ]);
    expect(lines.slice(3)).toEqual([
      "",
      "  1. [human] Telehealth  score 87.75  plays 131  rows 5  families 2  shows 26  last 2026-07-23",
      "       stations: kexp 100, wfmu 31",
      "       tracks:   Content (40)",
      "       albums:   Content (90)",
      "       labels:   none",
    ]);
  });

  test("shows the lane, the single id, and the distinct count past the top names", () => {
    const lines = formatArtistCandidateReport(
      listing([
        candidate({
          lane: "auto",
          mb_artist_id: "11111111-1111-4111-8111-111111111111",
          mb_artist_ids: ["11111111-1111-4111-8111-111111111111"],
          label_names: [
            { name: "Label One", play_count: 4 },
            { name: "Label Two", play_count: 3 },
          ],
          label_name_count: 9,
        }),
      ]),
    );
    expect(lines).toContain("  1. [auto ] Telehealth  score 87.75  plays 131  rows 5  families 2  shows 26  last 2026-07-23");
    expect(lines).toContain("       labels:   Label One (4); Label Two (3); 9 distinct");
    expect(lines).toContain("       mbid:     11111111-1111-4111-8111-111111111111");
  });

  test("lists every id when they conflict", () => {
    const lines = formatArtistCandidateReport(
      listing([candidate({ mb_artist_ids: ["a", "b"], mb_artist_id_conflict: true })]),
    );
    expect(lines).toContain("       mbids:    a, b (conflict: more than one id)");
    expect(lines.some((l) => l.includes("mbid: "))).toBe(false);
  });

  test("an empty ranking prints only the header", () => {
    expect(formatArtistCandidateReport(listing([]))).toHaveLength(3);
  });
});

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
