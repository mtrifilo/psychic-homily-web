import { describe, test, expect } from "bun:test";
import {
  aggregateRematchResults,
  dedupeArtistNames,
  formatArtistCandidateReport,
  formatTrackCandidateReport,
  printableJSON,
  type ArtistCandidate,
  type ArtistCandidateListing,
  type TrackCandidate,
  type TrackCandidateListing,
} from "../src/lib/radio";

function candidate(overrides: Partial<ArtistCandidate>): ArtistCandidate {
  return {
    artist_key: "telehealth",
    display_name: "Telehealth",
    artist_spellings: [{ name: "Telehealth", play_count: 131 }],
    artist_spelling_count: 1,
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
    shared_mb_artist_keys: [],
    non_latin_spelling: false,
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
  unkeyable_plays: 369,
});

describe("formatArtistCandidateReport", () => {
  test("prints the window, floors and totals, then one block per candidate", () => {
    const lines = formatArtistCandidateReport(listing([candidate({})]));
    expect(lines.slice(0, 4)).toEqual([
      "Artist candidates as of 2026-10-14: plays aired 2026-04-18 or later (180 days); floors 2 station rows and 3 plays.",
      "6465 in the requested lanes (2961 auto, 3504 human overall); showing 1.",
      "369 plays counted toward no candidate (the name normalizes to empty).",
      "Top tracks reflect the radio_tracks rebuild of (none recorded).",
    ]);
    expect(lines.slice(4)).toEqual([
      "",
      "  1. [human] Telehealth  score 87.75  plays 131  rows 5  families 2  shows 26  aired 2026-04-20 to 2026-07-23",
      "       key:      telehealth",
      "       stations: KEXP (kexp) 100, WFMU (wfmu) 31",
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
    expect(lines).toContain(
      "  1. [auto ] Telehealth  score 87.75  plays 131  rows 5  families 2  shows 26  aired 2026-04-20 to 2026-07-23",
    );
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
    expect(formatArtistCandidateReport(listing([]))).toHaveLength(4);
  });

  test("shows the spellings and the non-Latin note when they apply", () => {
    const lines = formatArtistCandidateReport(
      listing([
        candidate({
          display_name: "Faye Wong",
          artist_spellings: [
            { name: "Faye Wong", play_count: 10 },
            { name: "Faye Wong (王菲)", play_count: 5 },
          ],
          artist_spelling_count: 3,
          non_latin_spelling: true,
        }),
      ]),
    );
    expect(lines).toContain("       spellings: Faye Wong (10); Faye Wong (王菲) (5); 3 distinct");
    expect(lines).toContain(
      "       note:     a spelling carries a character normalization may not fold; the key may be a fragment of it",
    );
  });

  test("names the keys that share its MusicBrainz id", () => {
    const lines = formatArtistCandidateReport(
      listing([candidate({ shared_mb_artist_keys: ["telehealth band", "the telehealth"] })]),
    );
    expect(lines).toContain("       shares its MusicBrainz id with: telehealth band, the telehealth");
  });

  test("invisible characters print as replacement characters, joiners stay", () => {
    const lines = formatArtistCandidateReport(
      listing([candidate({ display_name: "Tele\u200bhealth \u{1F469}\u200d\u{1F3A4}" })]),
    );
    expect(lines.join(" | ")).toContain("Tele\uFFFDhealth \u{1F469}\u200d\u{1F3A4}");
  });

  test("bidirectional controls print as replacement characters", () => {
    const lines = formatArtistCandidateReport(listing([candidate({ display_name: "Tele\u202eLEHhealth" })]));
    expect(lines.join(" | ")).not.toMatch(/[\u202a\u202b\u202c\u202d\u202e\u2066-\u2069]/);
  });

  test("JSON output escapes terminal controls and stays valid JSON", () => {
    const value = { name: "a\u001b[2Kb\u009bc\u202ed" };
    const out = printableJSON(value);
    expect(out).not.toMatch(/[\u0000-\u0009\u000b-\u001f\u007f-\u009f\u202e]/);
    expect(JSON.parse(out)).toEqual(value);
  });

  test("control characters from playlist text print as replacement characters", () => {
    const lines = formatArtistCandidateReport(
      listing([
        candidate({
          display_name: "Tele\u001b[2K\u001b[1Ahealth",
          album_names: [{ name: "Album\u001b]52;c;ZXZpbA==" + "\u0007", play_count: 1 }],
        }),
      ]),
    );
    const text = lines.join(" | ");
    expect(text).not.toMatch(/[\u0000-\u001f\u007f-\u009f]/);
    expect(text).toContain("Tele\uFFFD[2K\uFFFD[1Ahealth");
    expect(text).toContain("Album\uFFFD]52;c;ZXZpbA==\uFFFD (1)");
  });
});

function trackCandidate(overrides: Partial<TrackCandidate>): TrackCandidate {
  return {
    track_id: 85405,
    display_title: "Special Unit",
    title_spellings: [{ name: "Special Unit", play_count: 35 }],
    title_spelling_count: 1,
    display_artist: "Party Dozen",
    artist_keys: [{ name: "party dozen", play_count: 35 }],
    artist_candidate_keys: [],
    artists: [{ artist_id: 806, name: "Party Dozen", slug: "party-dozen", play_count: 35 }],
    lane: "auto",
    score: 14.2214,
    play_count: 35,
    station_row_count: 4,
    station_family_count: 2,
    show_count: 12,
    first_aired_on: "2026-05-21",
    last_aired_on: "2026-07-07",
    stations: [
      { station_id: 7, slug: "kexp", name: "KEXP", play_count: 28 },
      { station_id: 8, slug: "wfmu", name: "WFMU", play_count: 7 },
    ],
    album_names: [{ name: "Special Unit", play_count: 30 }],
    album_name_count: 1,
    mb_release_id: "65f4fc5d-e5fa-4fd8-9718-1629293b2b5e",
    mb_release_ids: ["65f4fc5d-e5fa-4fd8-9718-1629293b2b5e"],
    non_latin_spelling: false,
    variant_release_id: false,
    ...overrides,
  };
}

const trackListing = (candidates: TrackCandidate[], view: TrackCandidateListing["view"] = "unlinked"): TrackCandidateListing => ({
  view,
  candidates,
  total: 223,
  auto_total: 116,
  human_total: 107,
  qualifying_tracks: 3080,
  untracked_plays: 554,
  as_of: "2026-10-14",
  window_start: "2026-04-18",
  window_days: 180,
  min_station_rows: 2,
  min_plays: 3,
  tracks_rebuilt_at: "2026-10-14T17:51:11Z",
});

describe("formatTrackCandidateReport", () => {
  test("prints the view, window, floors and totals, then one block per candidate", () => {
    const lines = formatTrackCandidateReport(trackListing([trackCandidate({})]));
    expect(lines).toEqual([
      "Track candidates, unlinked view, as of 2026-10-14: plays aired 2026-04-18 or later (180 days); floors 2 station rows and 3 plays.",
      "223 in the view and the requested lanes (116 auto, 107 human in the view); showing 1.",
      "3080 tracks clear the floors, in either view or in neither.",
      "554 plays counted toward no track (an unkeyable name, or a play newer than the last tracks rebuild).",
      "Tracks reflect the radio_tracks rebuild of 2026-10-14T17:51:11Z.",
      "",
      "  1. [auto ] Party Dozen - Special Unit  score 14.22  plays 35  rows 4  families 2  shows 12  aired 2026-05-21 to 2026-07-07",
      "       track:    85405",
      "       artists:  Party Dozen (id 806, party-dozen) 35",
      "       keys:     party dozen (35)",
      "       stations: KEXP (kexp) 28, WFMU (wfmu) 7",
      "       albums:   Special Unit (30)",
      "       release:  65f4fc5d-e5fa-4fd8-9718-1629293b2b5e",
    ]);
  });

  test("an unmatched row has no artists line; spellings, the note and every release id show when they apply", () => {
    const lines = formatTrackCandidateReport(
      trackListing(
        [
          trackCandidate({
            lane: "human",
            artists: [],
            artist_keys: [
              { name: "t joined", play_count: 4 },
              { name: "t joined band", play_count: 4 },
            ],
            artist_candidate_keys: ["t joined"],
            variant_release_id: true,
            title_spellings: [
              { name: "Song One", play_count: 6 },
              { name: "song one", play_count: 2 },
            ],
            title_spelling_count: 3,
            non_latin_spelling: true,
            mb_release_id: undefined,
            mb_release_ids: ["a", "b"],
          }),
        ],
        "unmatched",
      ),
    );
    expect(lines[0]).toStartWith("Track candidates, unmatched view, as of 2026-10-14");
    expect(lines.some((l) => l.includes("artists:"))).toBe(false);
    expect(lines).toContain("       keys:     t joined (4); t joined band (4)");
    expect(lines).toContain("       artist candidates: t joined");
    expect(lines).toContain(
      "       note:     every release id is on a title radio_track_title_key strips (a featured credit, a trailing bracket group, or a dash tail naming live, remix, edit, version, mono or stereo); it may be that version's release",
    );
    expect(lines).toContain("       titles:   Song One (6); song one (2); 3 distinct");
    expect(lines).toContain(
      "       note:     a spelling carries a character normalization may not fold; a key may be a fragment of it",
    );
    expect(lines).toContain("       releases: a, b (more than one MusicBrainz release id)");
    expect(lines.some((l) => l.includes("release: "))).toBe(false);
  });

  test("no release id prints no release line; an empty view prints only the header", () => {
    const lines = formatTrackCandidateReport(
      trackListing([trackCandidate({ mb_release_id: undefined, mb_release_ids: [] })]),
    );
    expect(lines.some((l) => l.includes("release"))).toBe(false);
    expect(formatTrackCandidateReport({ ...trackListing([]), tracks_rebuilt_at: undefined })).toEqual([
      "Track candidates, unlinked view, as of 2026-10-14: plays aired 2026-04-18 or later (180 days); floors 2 station rows and 3 plays.",
      "223 in the view and the requested lanes (116 auto, 107 human in the view); showing 0.",
      "3080 tracks clear the floors, in either view or in neither.",
      "554 plays counted toward no track (an unkeyable name, or a play newer than the last tracks rebuild).",
      "Tracks reflect the radio_tracks rebuild of (none recorded).",
    ]);
  });

  test("control and bidirectional characters from playlist text print as replacement characters", () => {
    const lines = formatTrackCandidateReport(
      trackListing([
        trackCandidate({
          display_title: "Spe\u001b[2Kcial‮Unit",
          album_names: [{ name: "Alb​um", play_count: 1 }],
        }),
      ]),
    );
    const text = lines.join(" | ");
    expect(text).not.toMatch(/[\u0000-\u001f\u007f-\u009f​‪-‮]/);
    expect(text).toContain("Spe�[2Kcial�Unit");
    expect(text).toContain("Alb�um (1)");
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
