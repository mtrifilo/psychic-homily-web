import { describe, test, expect } from "bun:test";
import {
  formatLabelCandidateReport,
  type LabelCandidate,
  type LabelCandidateListing,
} from "../src/lib/radio";

function candidate(overrides: Partial<LabelCandidate>): LabelCandidate {
  return {
    label_key: "p(doom) records",
    display_name: "p(doom) Records",
    label_spellings: [{ name: "p(doom) Records", play_count: 31 }],
    label_spelling_count: 1,
    lane: "human",
    score: 96.1534,
    velocity: 14,
    play_count: 31,
    recent_play_count: 14,
    prior_play_count: 0,
    station_row_count: 2,
    station_family_count: 2,
    show_count: 10,
    first_aired_on: "2026-02-03",
    last_aired_on: "2026-07-24",
    stations: [
      { station_id: 7, slug: "kexp", name: "KEXP", play_count: 30 },
      { station_id: 8, slug: "wfmu", name: "WFMU", play_count: 1 },
    ],
    artists: [
      { name: "King Gizzard & the Lizard Wizard", play_count: 13, artist_ids: [] },
      { name: "GUM", play_count: 6, artist_ids: [42] },
    ],
    artist_count: 2,
    release_names: [{ name: "Phantom Island", play_count: 12 }],
    release_name_count: 1,
    mb_release_ids: [],
    mb_release_id_count: 0,
    non_latin_spelling: false,
    ...overrides,
  };
}

const listing = (candidates: LabelCandidate[]): LabelCandidateListing => ({
  candidates,
  total: 526,
  auto_total: 0,
  human_total: 526,
  unkeyable_plays: 71,
  as_of: "2026-07-24",
  window_start: "2026-01-26",
  window_days: 180,
  recent_start: "2026-06-25",
  prior_start: "2026-05-26",
  velocity_days: 30,
  min_station_rows: 2,
  min_velocity: 1.5,
  min_new_plays: 3,
});

describe("formatLabelCandidateReport", () => {
  test("prints the windows, floors and totals, then one block per candidate", () => {
    const lines = formatLabelCandidateReport(listing([candidate({})]));
    expect(lines.slice(0, 4)).toEqual([
      "Label candidates as of 2026-07-24: plays aired 2026-01-26 through 2026-07-24 (180 days); floor 2 station rows.",
      "Rising: plays aired 2026-06-25 through 2026-07-24 at least 1.5x those aired 2026-05-26 through the day before 2026-06-25 (30-day windows), or at least 3 when there were none.",
      "526 in the requested lanes (0 auto, 526 human overall); showing 1.",
      "71 plays counted toward no candidate (the label name normalizes to empty).",
    ]);
    expect(lines.slice(4)).toEqual([
      "",
      "  1. [human] p(doom) Records  score 96.15  velocity 14.00 (14 recent / 0 prior)  plays 31  rows 2  families 2  shows 10  aired 2026-02-03 to 2026-07-24",
      "       key:      p(doom) records",
      "       stations: KEXP (kexp) 30, WFMU (wfmu) 1",
      "       artists:  King Gizzard & the Lizard Wizard (13); GUM [catalog 42] (6)",
      "       releases: Phantom Island (12)",
    ]);
  });

  test("shows the spellings, the distinct counts, the release ids and the non-Latin note when they apply", () => {
    const lines = formatLabelCandidateReport(
      listing([
        candidate({
          label_spellings: [
            { name: "p(doom) Records", play_count: 29 },
            { name: "P(DOOM) RECORDS", play_count: 2 },
          ],
          label_spelling_count: 3,
          artist_count: 7,
          mb_release_ids: ["11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"],
          mb_release_id_count: 4,
          non_latin_spelling: true,
        }),
      ]),
    );
    expect(lines).toContain("       spellings: p(doom) Records (29); P(DOOM) RECORDS (2); 3 distinct");
    expect(lines).toContain(
      "       artists:  King Gizzard & the Lizard Wizard (13); GUM [catalog 42] (6); 7 distinct",
    );
    expect(lines).toContain(
      "       mb releases: 11111111-1111-4111-8111-111111111111, 22222222-2222-4222-8222-222222222222; 4 distinct",
    );
    expect(lines).toContain(
      "       note:     a spelling carries a character normalization may not fold; the key may be a fragment of it",
    );
  });

  test("an artist linked to several catalog artists lists every id", () => {
    const lines = formatLabelCandidateReport(
      listing([candidate({ artists: [{ name: "GUM", play_count: 6, artist_ids: [42, 43] }], artist_count: 1 })]),
    );
    expect(lines).toContain("       artists:  GUM [catalog 42, 43] (6)");
  });

  test("an empty ranking prints only the header", () => {
    expect(formatLabelCandidateReport(listing([]))).toHaveLength(4);
  });

  test("control characters from playlist text print as replacement characters in every field", () => {
    const bad = "\u001b[2K\u202e\u2066\u200B";
    const lines = formatLabelCandidateReport(
      listing([
        candidate({
          display_name: `p(doom)${bad} Records`,
          label_key: `p(doom)${bad}`,
          label_spellings: [
            { name: `p(doom)${bad} Records`, play_count: 29 },
            { name: `P(DOOM)${bad}`, play_count: 2 },
          ],
          label_spelling_count: 2,
          mb_release_ids: [`1111${bad}`],
          mb_release_id_count: 1,
          artists: [{ name: `GUM${bad}`, play_count: 6, artist_ids: [] }],
          release_names: [{ name: `Island${bad}`, play_count: 1 }],
          stations: [{ station_id: 7, slug: `kexp${bad}`, name: `KEXP${bad}`, play_count: 30 }],
        }),
      ]),
    );
    const text = lines.join(" | ");
    expect(text).not.toMatch(/[\u0000-\u001f\u007f-\u009f\u200B\u202a-\u202e\u2066-\u2069]/);
    expect(text).toContain("p(doom)\uFFFD[2K\uFFFD\uFFFD\uFFFD Records");
  });

  test("the station line falls back to none and the release ids omit an equal distinct count", () => {
    const lines = formatLabelCandidateReport(
      listing([
        candidate({
          stations: [],
          mb_release_ids: ["11111111-1111-4111-8111-111111111111"],
          mb_release_id_count: 1,
        }),
      ]),
    );
    expect(lines).toContain("       stations: none");
    expect(lines).toContain("       mb releases: 11111111-1111-4111-8111-111111111111");
  });
});
