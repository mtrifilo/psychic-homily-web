import { describe, test, expect } from "bun:test";
import {
  normalizeName,
  scoreEntitySet,
  scoreFestivalFields,
  scoreBillingTiers,
  scoreShowTimes,
  scoreExtraction,
  parseModelBatch,
  formatScore,
  type BatchItem,
} from "../eval/scoring";

const golden: BatchItem[] = [
  { entity_type: "venue", name: "Douglas Park", city: "Chicago", state: "IL" },
  { entity_type: "artist", name: "Tool" },
  { entity_type: "artist", name: "Pixies" },
  { entity_type: "artist", name: "3OH!3" },
  {
    entity_type: "festival",
    name: "Riot Fest 2026",
    series_slug: "riot-fest",
    edition_year: 2026,
    start_date: "2026-09-18",
    end_date: "2026-09-20",
    artists: [
      { name: "Tool", billing_tier: "headliner" },
      { name: "Pixies", billing_tier: "sub_headliner" },
      { name: "3OH!3", billing_tier: "mid_card" },
    ],
  },
];

describe("normalizeName", () => {
  test("lowercases and trims", () => {
    expect(normalizeName("  Tool  ")).toBe("tool");
  });
  test("collapses internal whitespace", () => {
    expect(normalizeName("Twenty  One   Pilots")).toBe("twenty one pilots");
  });
  test("preserves accents and punctuation", () => {
    expect(normalizeName("División Minúscula")).toBe("división minúscula");
    expect(normalizeName("3OH!3")).toBe("3oh!3");
  });
});

describe("scoreEntitySet", () => {
  test("perfect match scores recall 1, no misses or hallucinations", () => {
    const expected: BatchItem[] = [
      { entity_type: "artist", name: "Tool" },
      { entity_type: "artist", name: "Pixies" },
    ];
    const actual: BatchItem[] = [
      { entity_type: "artist", name: "tool" }, // case-insensitive match
      { entity_type: "artist", name: "Pixies" },
    ];
    const s = scoreEntitySet(expected, actual);
    expect(s.recall).toBe(1);
    expect(s.found).toBe(2);
    expect(s.missed).toHaveLength(0);
    expect(s.hallucinated).toHaveLength(0);
  });

  test("counts missed artists", () => {
    const expected: BatchItem[] = [
      { entity_type: "artist", name: "Tool" },
      { entity_type: "artist", name: "Pixies" },
    ];
    const actual: BatchItem[] = [{ entity_type: "artist", name: "Tool" }];
    const s = scoreEntitySet(expected, actual);
    expect(s.recall).toBe(0.5);
    expect(s.missed).toEqual(["Pixies"]);
  });

  test("counts hallucinated artists", () => {
    const expected: BatchItem[] = [{ entity_type: "artist", name: "Tool" }];
    const actual: BatchItem[] = [
      { entity_type: "artist", name: "Tool" },
      { entity_type: "artist", name: "Imaginary Band" },
    ];
    const s = scoreEntitySet(expected, actual);
    expect(s.found).toBe(1);
    expect(s.hallucinated).toEqual(["Imaginary Band"]);
  });

  test("empty expected set scores recall 1", () => {
    const s = scoreEntitySet([], [{ entity_type: "artist", name: "X" }]);
    expect(s.recall).toBe(1);
    expect(s.hallucinated).toEqual(["X"]);
  });
});

describe("scoreFestivalFields", () => {
  test("all fields correct", () => {
    const actual: BatchItem = {
      entity_type: "festival",
      name: "Riot Fest 2026",
      series_slug: "riot-fest",
      edition_year: 2026,
      start_date: "2026-09-18",
      end_date: "2026-09-20",
    };
    const fields = scoreFestivalFields(golden[4], actual);
    expect(fields).toHaveLength(5);
    expect(fields.every((f) => f.correct)).toBe(true);
  });

  test("flags a wrong date", () => {
    const actual: BatchItem = {
      entity_type: "festival",
      name: "Riot Fest 2026",
      series_slug: "riot-fest",
      edition_year: 2026,
      start_date: "2026-09-19",
      end_date: "2026-09-20",
    };
    const fields = scoreFestivalFields(golden[4], actual);
    const startField = fields.find((f) => f.field === "start_date");
    expect(startField?.correct).toBe(false);
  });

  test("missing festival yields all-incorrect", () => {
    const fields = scoreFestivalFields(golden[4], undefined);
    expect(fields.every((f) => !f.correct)).toBe(true);
  });

  test("no expected festival yields empty array", () => {
    expect(scoreFestivalFields(undefined, undefined)).toHaveLength(0);
  });
});

describe("scoreBillingTiers", () => {
  test("perfect tier agreement", () => {
    const actual: BatchItem = {
      entity_type: "festival",
      artists: [
        { name: "Tool", billing_tier: "headliner" },
        { name: "Pixies", billing_tier: "sub_headliner" },
        { name: "3OH!3", billing_tier: "mid_card" },
      ],
    };
    const b = scoreBillingTiers(golden[4], actual);
    expect(b.matched).toBe(3);
    expect(b.comparable).toBe(3);
    expect(b.rate).toBe(1);
  });

  test("partial tier disagreement", () => {
    const actual: BatchItem = {
      entity_type: "festival",
      artists: [
        { name: "Tool", billing_tier: "headliner" },
        { name: "Pixies", billing_tier: "mid_card" }, // wrong
        { name: "3OH!3", billing_tier: "mid_card" },
      ],
    };
    const b = scoreBillingTiers(golden[4], actual);
    expect(b.matched).toBe(2);
    expect(b.comparable).toBe(3);
  });

  test("artist missing from model output is not counted as comparable", () => {
    const actual: BatchItem = {
      entity_type: "festival",
      artists: [{ name: "Tool", billing_tier: "headliner" }],
    };
    const b = scoreBillingTiers(golden[4], actual);
    expect(b.comparable).toBe(1);
    expect(b.matched).toBe(1);
  });
});

describe("scoreExtraction", () => {
  test("perfect extraction scores overall 1.0", () => {
    const s = scoreExtraction(golden, golden);
    expect(s.artists.recall).toBe(1);
    expect(s.venues.recall).toBe(1);
    expect(s.billingTierAgreement.rate).toBe(1);
    expect(s.overall).toBeCloseTo(1, 5);
  });

  test("hallucinations drag the overall score below the recall ceiling", () => {
    const actual: BatchItem[] = [
      ...golden,
      { entity_type: "artist", name: "Fake Band 1" },
      { entity_type: "artist", name: "Fake Band 2" },
    ];
    const s = scoreExtraction(golden, actual);
    expect(s.artists.recall).toBe(1); // all real artists still found
    expect(s.artists.hallucinated).toHaveLength(2);
    expect(s.overall).toBeLessThan(1); // but overall penalized
  });

  test("a missed venue lowers the venue component", () => {
    const actual = golden.filter((x) => x.entity_type !== "venue");
    const s = scoreExtraction(golden, actual);
    expect(s.venues.recall).toBe(0);
    expect(s.venues.missed).toEqual(["Douglas Park"]);
    expect(s.overall).toBeLessThan(1);
  });
});

describe("parseModelBatch", () => {
  test("parses a bare JSON array", () => {
    const out = parseModelBatch('[{"entity_type":"artist","name":"Tool"}]');
    expect(out).toHaveLength(1);
    expect(out[0].name).toBe("Tool");
  });

  test("strips ```json fences", () => {
    const out = parseModelBatch('```json\n[{"entity_type":"artist","name":"Tool"}]\n```');
    expect(out[0].name).toBe("Tool");
  });

  test("strips bare ``` fences", () => {
    const out = parseModelBatch('```\n[{"entity_type":"artist","name":"Tool"}]\n```');
    expect(out[0].name).toBe("Tool");
  });

  test("extracts an array surrounded by prose", () => {
    const out = parseModelBatch('Here is the data:\n[{"entity_type":"artist","name":"Tool"}]\nDone.');
    expect(out[0].name).toBe("Tool");
  });

  test("throws on non-array JSON", () => {
    expect(() => parseModelBatch('{"entity_type":"artist"}')).toThrow();
  });

  test("throws on unparseable output", () => {
    expect(() => parseModelBatch("not json at all")).toThrow();
  });
});

describe("formatScore", () => {
  test("renders headline metrics", () => {
    const s = scoreExtraction(golden, golden);
    const text = formatScore(s);
    expect(text).toContain("Artists: 3/3");
    expect(text).toContain("Venues: 1/1");
    expect(text).toContain("Billing-tier agreement: 3/3");
    expect(text).toContain("Overall score: 100.0%");
  });

  test("lists missed and hallucinated names", () => {
    const actual: BatchItem[] = [
      { entity_type: "artist", name: "Tool" },
      { entity_type: "artist", name: "Ghost Band" },
    ];
    const s = scoreExtraction(golden, actual);
    const text = formatScore(s);
    expect(text).toContain("missed:");
    expect(text).toContain("hallucinated: Ghost Band");
  });
});

// -- scoreShowTimes ----------------------------------------------------------

function show(times: { doors_at?: string; music_at?: string }): BatchItem {
  return {
    entity_type: "show",
    event_date: "2026-09-04",
    city: "Chicago",
    state: "IL",
    artists: [{ name: "Wolves of Glendale" }],
    venues: [{ name: "Lincoln Hall" }],
    ...times,
  };
}

describe("scoreShowTimes", () => {
  test("a labelled pair reproduced exactly scores 1", () => {
    const g = [show({ doors_at: "7:30PM", music_at: "8:30PM" })];
    const s = scoreShowTimes(g, [show({ doors_at: "7:30PM", music_at: "8:30PM" })]);
    expect(s.rate).toBe(1);
    expect(s.invented).toEqual([]);
    expect(s.missed).toEqual([]);
  });

  test("spacing is not an extraction error, because both reach the same instant", () => {
    const g = [show({ doors_at: "7:30PM", music_at: "8:30PM" })];
    const s = scoreShowTimes(g, [show({ doors_at: "7:30 pm", music_at: "8:30 PM" })]);
    expect(s.rate).toBe(1);
  });

  test("a dropped door time is a miss, not a match", () => {
    const g = [show({ doors_at: "7:30PM", music_at: "8:30PM" })];
    const s = scoreShowTimes(g, [show({ music_at: "8:30PM" })]);
    expect(s.rate).toBe(0);
    expect(s.missed).toEqual(["19:30|20:30"]);
  });

  test("a time invented for a listing that labelled none scores zero and is named", () => {
    const g = [show({}), show({})];
    const s = scoreShowTimes(g, [show({ music_at: "10:00PM" }), show({})]);
    expect(s.matched).toBe(1);
    expect(s.rate).toBe(0.5);
    expect(s.invented).toEqual(["|22:00"]);
  });

  test("a stated but unreadable time stays distinct from an absent one", () => {
    const g = [show({ music_at: "doors at 7" })];
    const s = scoreShowTimes(g, [show({})]);
    expect(s.rate).toBe(0);
    expect(s.missed).toEqual(["|?doors at 7"]);
  });

  test("a fixture with no shows scores 1 and reports nothing", () => {
    const s = scoreShowTimes(golden, golden);
    expect(s.expected).toEqual([]);
    expect(s.rate).toBe(1);
  });
});

describe("scoreExtraction with shows", () => {
  test("a fixture with no shows scores exactly what the four-component weighting gives", () => {
    // The show-times component must not move an existing baseline: with no
    // golden shows the weights renormalize back to 0.55/0.10/0.20/0.15.
    const actual: BatchItem[] = [
      { entity_type: "venue", name: "Douglas Park", city: "Chicago", state: "IL" },
      { entity_type: "artist", name: "Tool" },
      { entity_type: "artist", name: "Pixies" },
      golden[4],
    ];
    const s = scoreExtraction(golden, actual);
    const artistRecall = 2 / 3;
    expect(s.overall).toBeCloseTo(0.55 * artistRecall + 0.1 + 0.2 + 0.15, 10);
  });

  test("invented show times pull the overall score down", () => {
    const g = [...golden, show({})];
    const clean = scoreExtraction(g, [...golden, show({})]);
    const invented = scoreExtraction(g, [...golden, show({ music_at: "10:00PM" })]);
    expect(clean.overall).toBeGreaterThan(invented.overall);
    expect(invented.showTimes.rate).toBe(0);
  });

  test("formatScore reports the schedule line only when the golden has shows", () => {
    expect(formatScore(scoreExtraction(golden, golden))).not.toContain("Show times:");
    const g = [...golden, show({ doors_at: "7:30PM", music_at: "8:30PM" })];
    expect(formatScore(scoreExtraction(g, g))).toContain("Show times: 1/1");
  });
});
