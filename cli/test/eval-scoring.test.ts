import { describe, test, expect } from "bun:test";
import {
  normalizeName,
  scoreEntitySet,
  scoreFestivalFields,
  scoreBillingTiers,
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
