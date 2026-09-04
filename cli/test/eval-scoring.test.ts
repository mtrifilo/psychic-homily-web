import { describe, test, expect } from "bun:test";
import {
  normalizeName,
  scoreEntitySet,
  scoreFestivalFields,
  scoreBillingTiers,
  scoreShowFields,
  scoreShowTimes,
  scoreExtraction,
  listMismatches,
  MAX_REPORTED_MISMATCHES,
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

// The two show fields the extraction rules govern by an "only when stated"
// rule. Both are scored on ABSENCE as well as value, which is the whole point:
// a spurious door price and a headliner read off list order are the failures
// the show-flyer fixtures exist to catch, and neither shows up in a recall
// metric.
describe("scoreShowFields", () => {
  const splitPriceGolden: BatchItem[] = [
    {
      entity_type: "show",
      event_date: "2026-04-18",
      city: "Phoenix",
      state: "AZ",
      price: 20,
      door_price: 25,
      artists: [
        { name: "Desert Hymn", is_headliner: true, set_type: "headliner" },
        { name: "Copper Ghost", set_type: "special_guest" },
        { name: "Low Ember", set_type: "opener" },
      ],
      venues: [{ name: "Valley Bar", city: "Phoenix", state: "AZ" }],
    },
  ];

  const barePriceGolden: BatchItem[] = [
    {
      entity_type: "show",
      event_date: "2026-05-08",
      city: "Phoenix",
      state: "AZ",
      price: 15,
      artists: [{ name: "Paper Tigers" }, { name: "Glass Arcade" }],
      venues: [{ name: "Rhythm Room", city: "Phoenix", state: "AZ" }],
    },
  ];

  test("a faithful split price and stated roles score 1", () => {
    const s = scoreShowFields(splitPriceGolden, splitPriceGolden);
    expect(s.shows).toEqual({
      expected: 1,
      matched: 1,
      missed: [],
      hallucinated: [],
      rate: 1,
    });
    expect(s.prices.rate).toBe(1);
    expect(s.billRoles.rate).toBe(1);
  });

  test("a show the model never produced leaves both rates uncomparable, not passing", () => {
    // The disarming case: the model got the date format wrong AND copied the
    // price into door_price AND designated a headliner. Both agreement rates
    // are a vacuous 1 because nothing was comparable, so `missed` is the only
    // thing that can say so, and the assertion layer has to read it.
    const drifted: BatchItem[] = [
      {
        ...barePriceGolden[0],
        event_date: "2026-05-08T00:00:00Z",
        door_price: 15,
        artists: [{ name: "Paper Tigers", is_headliner: true }, { name: "Glass Arcade" }],
      },
    ];
    const s = scoreShowFields(barePriceGolden, drifted);
    expect(s.shows.matched).toBe(0);
    expect(s.shows.missed).toEqual(["2026-05-08|rhythm room"]);
    expect(s.shows.hallucinated).toEqual(["2026-05-08T00:00:00Z|rhythm room"]);
    expect(s.prices.rate).toBe(1);
    expect(s.billRoles.rate).toBe(1);
  });

  test("a show no golden show claims is reported as hallucinated", () => {
    const invented: BatchItem[] = [
      barePriceGolden[0],
      { ...barePriceGolden[0], event_date: "2026-05-09" },
    ];
    const s = scoreShowFields(barePriceGolden, invented);
    expect(s.shows.matched).toBe(1);
    expect(s.shows.hallucinated).toEqual(["2026-05-09|rhythm room"]);
  });

  test("two golden shows on one date and venue need two produced shows", () => {
    // An early and a late set in one room share a key. Matching them against
    // one produced show would grade it twice and report full recall.
    const twoSets = [barePriceGolden[0], { ...barePriceGolden[0], title: "late set" }];
    const s = scoreShowFields(twoSets, barePriceGolden);
    expect(s.shows.matched).toBe(1);
    expect(s.shows.missed).toEqual(["2026-05-08|rhythm room"]);
    expect(s.prices.comparable).toBe(2);
  });

  test("a dropped door price is a price mismatch, not a silent pass", () => {
    const actual: BatchItem[] = [{ ...splitPriceGolden[0], door_price: undefined }];
    const s = scoreShowFields(splitPriceGolden, actual);
    expect(s.prices.matched).toBe(1);
    expect(s.prices.comparable).toBe(2);
    expect(s.prices.mismatches[0]).toContain("door_price: expected 25, got null");
  });

  test("a door price the source never stated is a mismatch", () => {
    const actual: BatchItem[] = [{ ...barePriceGolden[0], door_price: 15 }];
    const s = scoreShowFields(barePriceGolden, actual);
    expect(s.prices.rate).toBe(0.5);
    expect(s.prices.mismatches[0]).toContain("door_price: expected null, got 15");
  });

  test("a price written as a currency string still matches its number", () => {
    const actual: BatchItem[] = [{ ...barePriceGolden[0], price: "$15" }];
    expect(scoreShowFields(barePriceGolden, actual).prices.rate).toBe(1);
  });

  test("a price that is not a number matches nothing, including an absent one", () => {
    const actual: BatchItem[] = [{ ...barePriceGolden[0], price: "donation" }];
    expect(scoreShowFields(barePriceGolden, actual).prices.rate).toBe(0.5);
  });

  test.each([["  "], ["$"], [","], ["$ ,"]])(
    "a price of %p is unreadable, not a free show",
    (unreadable: string) => {
      // Number("") is 0, so stripping currency punctuation before the empty
      // test is what keeps these from matching a golden `price: 0`.
      const free: BatchItem[] = [{ ...barePriceGolden[0], price: 0 }];
      const actual: BatchItem[] = [{ ...barePriceGolden[0], price: unreadable }];
      expect(scoreShowFields(free, actual).prices.rate).toBe(0.5);
    }
  );

  test("a price that is not even a scalar is unreadable", () => {
    const free: BatchItem[] = [{ ...barePriceGolden[0], price: 0 }];
    const actual = [{ ...barePriceGolden[0], price: [] }] as unknown as BatchItem[];
    expect(scoreShowFields(free, actual).prices.rate).toBe(0.5);
  });

  test("an empty-string price is silence, not an unreadable value", () => {
    const omitted: BatchItem[] = [{ ...barePriceGolden[0], price: undefined }];
    const blank: BatchItem[] = [{ ...barePriceGolden[0], price: "" }];
    expect(scoreShowFields(omitted, blank).prices.rate).toBe(1);
  });

  test("zero is a stated price, not silence", () => {
    const free: BatchItem[] = [{ ...barePriceGolden[0], price: 0 }];
    const omitted: BatchItem[] = [{ ...barePriceGolden[0], price: undefined }];
    expect(scoreShowFields(free, omitted).prices.rate).toBe(0.5);
    expect(scoreShowFields(free, free).prices.rate).toBe(1);
  });

  test("a role read off list order is a bill-role mismatch", () => {
    // The classic failure: nothing on the flyer named a headliner, and the
    // model designated the first act anyway.
    const actual: BatchItem[] = [
      {
        ...barePriceGolden[0],
        artists: [{ name: "Paper Tigers", is_headliner: true }, { name: "Glass Arcade" }],
      },
    ];
    const s = scoreShowFields(barePriceGolden, actual);
    expect(s.billRoles.matched).toBe(1);
    expect(s.billRoles.comparable).toBe(2);
    expect(s.billRoles.mismatches[0]).toContain("expected null, got headliner");
  });

  test("performer and an absent set_type are the same silence", () => {
    const actual: BatchItem[] = [
      {
        ...barePriceGolden[0],
        artists: [
          { name: "Paper Tigers", set_type: "performer" },
          { name: "Glass Arcade", set_type: "  " },
        ],
      },
    ];
    expect(scoreShowFields(barePriceGolden, actual).billRoles.rate).toBe(1);
  });

  test("a flattened role is a mismatch", () => {
    const actual: BatchItem[] = [
      {
        ...splitPriceGolden[0],
        artists: [
          { name: "Desert Hymn", is_headliner: true, set_type: "headliner" },
          { name: "Copper Ghost" },
          { name: "Low Ember", set_type: "opener" },
        ],
      },
    ];
    const s = scoreShowFields(splitPriceGolden, actual);
    expect(s.billRoles.matched).toBe(2);
    expect(s.billRoles.mismatches[0]).toContain("expected special_guest, got null");
  });

  test("an act the model never produced is left to artist recall", () => {
    const actual: BatchItem[] = [
      { ...splitPriceGolden[0], artists: [{ name: "Desert Hymn", set_type: "headliner" }] },
    ];
    const s = scoreShowFields(splitPriceGolden, actual);
    expect(s.billRoles.comparable).toBe(1);
    expect(s.billRoles.rate).toBe(1);
  });

  test("a show on a different date is missed, and grades no fields", () => {
    const actual: BatchItem[] = [{ ...splitPriceGolden[0], event_date: "2026-04-19" }];
    const s = scoreShowFields(splitPriceGolden, actual);
    expect(s.shows.matched).toBe(0);
    expect(s.shows.missed).toEqual(["2026-04-18|valley bar"]);
    expect(s.prices.comparable).toBe(0);
    expect(s.prices.rate).toBe(1);
  });

  test("a batch with no shows scores a vacuous 1 and reports nothing", () => {
    const s = scoreShowFields(golden, golden);
    expect(s.shows.expected).toBe(0);
    expect(s.prices.rate).toBe(1);
    expect(formatScore(scoreExtraction(golden, golden))).not.toContain("Show prices");
  });

  test("mismatch lines are capped so one bad lineup cannot fill the report", () => {
    const acts = Array.from({ length: 30 }, (_, i) => ({ name: `Act ${i}` }));
    const many: BatchItem[] = [
      { ...barePriceGolden[0], artists: acts.map(a => ({ ...a, set_type: "opener" })) },
    ];
    const none: BatchItem[] = [{ ...barePriceGolden[0], artists: acts }];
    const s = scoreShowFields(many, none);
    expect(s.billRoles.mismatches).toHaveLength(30);
    const listed = listMismatches(s.billRoles);
    expect(listed).toHaveLength(MAX_REPORTED_MISMATCHES + 1);
    expect(listed.at(-1)).toBe(`and ${30 - MAX_REPORTED_MISMATCHES} more`);
  });

  test("show fields are reported but do not move overall", () => {
    // A fixture's overall stays comparable with the number it recorded before
    // this metric existed.
    const perfect = scoreExtraction(splitPriceGolden, splitPriceGolden).overall;
    const wrongPrice = scoreExtraction(splitPriceGolden, [
      { ...splitPriceGolden[0], door_price: undefined },
    ]);
    expect(wrongPrice.overall).toBe(perfect);
    expect(wrongPrice.showFields.prices.rate).toBe(0.5);
    expect(formatScore(wrongPrice)).toContain("Show prices (absence included): 1/2");
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
  test("overall stays the four-component weighting, shows or no shows", () => {
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

  test("invented show times are reported WITHOUT moving overall", () => {
    // Show-time agreement sits beside `overall`, like show prices and bill
    // roles: folding it in would give a fixture with no shows a vacuous perfect
    // component and make scores incomparable across fixtures. The assertion
    // layer is what gates on it.
    const g = [...golden, show({})];
    const clean = scoreExtraction(g, [...golden, show({})]);
    const invented = scoreExtraction(g, [...golden, show({ music_at: "10:00PM" })]);
    expect(invented.showTimes.rate).toBe(0);
    expect(clean.showTimes.rate).toBe(1);
    expect(invented.overall).toBeCloseTo(clean.overall, 10);
  });

  test("formatScore reports the schedule line only when the golden has shows", () => {
    expect(formatScore(scoreExtraction(golden, golden))).not.toContain("Show times:");
    const g = [...golden, show({ doors_at: "7:30PM", music_at: "8:30PM" })];
    expect(formatScore(scoreExtraction(g, g))).toContain("Show times: 1/1");
  });
});
