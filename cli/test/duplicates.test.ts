import { describe, test, expect } from "bun:test";
import {
  normalizeForComparison,
  similarityScore,
  compareFields,
  classifyAction,
  classifyMatch,
} from "../src/lib/duplicates";

describe("normalizeForComparison", () => {
  test("lowercases text", () => {
    expect(normalizeForComparison("Hello World")).toBe("hello world");
  });

  test("trims whitespace", () => {
    expect(normalizeForComparison("  hello  ")).toBe("hello");
  });

  test("collapses multiple spaces", () => {
    expect(normalizeForComparison("hello    world")).toBe("hello world");
  });

  test("collapses tabs and newlines into single space", () => {
    expect(normalizeForComparison("hello\t\n  world")).toBe("hello world");
  });

  test("strips accents/diacritics", () => {
    expect(normalizeForComparison("Motörhead")).toBe("motorhead");
    expect(normalizeForComparison("café")).toBe("cafe");
    expect(normalizeForComparison("naïve")).toBe("naive");
    expect(normalizeForComparison("résumé")).toBe("resume");
  });

  test("handles empty string", () => {
    expect(normalizeForComparison("")).toBe("");
  });

  test("handles already normalized string", () => {
    expect(normalizeForComparison("already clean")).toBe("already clean");
  });

  test("handles combined transformations", () => {
    expect(normalizeForComparison("  Beyoncé   Knowles  ")).toBe("beyonce knowles");
  });
});

describe("similarityScore", () => {
  test("exact match returns 1.0", () => {
    expect(similarityScore("Radiohead", "radiohead")).toBe(1.0);
  });

  test("exact match after normalization returns 1.0", () => {
    expect(similarityScore("  The  Beatles  ", "the beatles")).toBe(1.0);
  });

  test("exact match with accents returns 1.0", () => {
    expect(similarityScore("Motörhead", "Motorhead")).toBe(1.0);
  });

  test("substring match returns > 0.6", () => {
    const score = similarityScore("The National", "National");
    expect(score).toBeGreaterThan(0.6);
  });

  test("contained string returns > 0.8", () => {
    const score = similarityScore("National", "The National");
    expect(score).toBeGreaterThan(0.8);
  });

  test("completely different strings return < 0.3", () => {
    const score = similarityScore("Radiohead", "Beyonce");
    expect(score).toBeLessThan(0.3);
  });

  test("empty string against non-empty returns 0", () => {
    expect(similarityScore("", "something")).toBe(0);
    expect(similarityScore("something", "")).toBe(0);
  });

  test("both empty strings return 1.0", () => {
    expect(similarityScore("", "")).toBe(1.0);
  });

  test("common prefix boosts score", () => {
    const score = similarityScore("Radiohead", "Radiograph");
    // Shares "Radio" prefix (5/10)
    expect(score).toBeGreaterThan(0.3);
  });

  test("very similar names score high", () => {
    // "the shins" vs "the shin" — one character difference, common prefix + suffix
    const score = similarityScore("The Shins", "The Shin");
    expect(score).toBeGreaterThan(0.6);
  });
});

describe("compareFields", () => {
  test("detects new_info when existing is empty", () => {
    const result = compareFields(
      { name: "Test", city: "" },
      { name: "Test", city: "Phoenix" },
      ["name", "city"],
    );

    const cityField = result.find((f) => f.field === "city");
    expect(cityField).toBeDefined();
    expect(cityField!.status).toBe("new_info");
    expect(cityField!.existing).toBe("");
    expect(cityField!.proposed).toBe("Phoenix");
  });

  test("detects unchanged when values match", () => {
    const result = compareFields(
      { name: "Test" },
      { name: "Test" },
      ["name"],
    );

    expect(result[0].status).toBe("unchanged");
  });

  test("detects already_set when existing has different value", () => {
    const result = compareFields(
      { city: "Phoenix" },
      { city: "Tempe" },
      ["city"],
    );

    expect(result[0].status).toBe("already_set");
    expect(result[0].existing).toBe("Phoenix");
    expect(result[0].proposed).toBe("Tempe");
  });

  test("skips fields not in proposed data", () => {
    const result = compareFields(
      { name: "Test", city: "Phoenix" },
      { name: "Test" },
      ["name", "city"],
    );

    // Only name should be in results since city is missing from proposed
    expect(result.length).toBe(1);
    expect(result[0].field).toBe("name");
  });

  test("skips fields with null or undefined proposed values", () => {
    const result = compareFields(
      { name: "Test", city: "Phoenix" },
      { name: "Test", city: null },
      ["name", "city"],
    );

    expect(result.length).toBe(1);
    expect(result[0].field).toBe("name");
  });

  test("skips fields with empty string proposed values", () => {
    const result = compareFields(
      { name: "Test", city: "Phoenix" },
      { name: "Test", city: "" },
      ["name", "city"],
    );

    expect(result.length).toBe(1);
  });

  test("handles multiple fields with mixed statuses", () => {
    const result = compareFields(
      { name: "Test", city: "", state: "AZ", website: "https://old.com" },
      { name: "Test", city: "Phoenix", state: "AZ", website: "https://new.com" },
      ["name", "city", "state", "website"],
    );

    const statusMap = Object.fromEntries(result.map((f) => [f.field, f.status]));
    expect(statusMap.name).toBe("unchanged");
    expect(statusMap.city).toBe("new_info");
    expect(statusMap.state).toBe("unchanged");
    expect(statusMap.website).toBe("already_set");
  });

  test("converts non-string values to strings for comparison", () => {
    const result = compareFields(
      { capacity: 500 },
      { capacity: 500 },
      ["capacity"],
    );

    expect(result[0].status).toBe("unchanged");
    expect(result[0].existing).toBe("500");
    expect(result[0].proposed).toBe("500");
  });

  test("treats null existing as empty string for new_info detection", () => {
    const result = compareFields(
      { website: null },
      { website: "https://example.com" },
      ["website"],
    );

    expect(result[0].status).toBe("new_info");
    expect(result[0].existing).toBe("");
  });
});

describe("classifyAction", () => {
  test("returns create when confidence < 0.6", () => {
    expect(classifyAction(0.5, [])).toBe("create");
    expect(classifyAction(0.0, [])).toBe("create");
    expect(classifyAction(0.59, [])).toBe("create");
  });

  test("returns update when match found and has new_info", () => {
    const fields = [
      { field: "name", existing: "Test", proposed: "Test", status: "unchanged" as const },
      { field: "city", existing: "", proposed: "Phoenix", status: "new_info" as const },
    ];
    expect(classifyAction(0.8, fields)).toBe("update");
  });

  test("returns skip when match found and no new_info", () => {
    const fields = [
      { field: "name", existing: "Test", proposed: "Test", status: "unchanged" as const },
      { field: "city", existing: "Phoenix", proposed: "Tempe", status: "already_set" as const },
    ];
    expect(classifyAction(0.9, fields)).toBe("skip");
  });

  test("returns skip when match found and fields are empty", () => {
    expect(classifyAction(1.0, [])).toBe("skip");
  });

  test("returns update at exact 0.6 threshold with new_info", () => {
    const fields = [
      { field: "city", existing: "", proposed: "Phoenix", status: "new_info" as const },
    ];
    expect(classifyAction(0.6, fields)).toBe("update");
  });
});

describe("classifyMatch", () => {
  test("returns exact for confidence 1.0", () => {
    expect(classifyMatch(1.0)).toBe("exact");
  });

  test("returns fuzzy for confidence >= 0.6 and < 1.0", () => {
    expect(classifyMatch(0.6)).toBe("fuzzy");
    expect(classifyMatch(0.8)).toBe("fuzzy");
    expect(classifyMatch(0.99)).toBe("fuzzy");
  });

  test("returns none for confidence < 0.6", () => {
    expect(classifyMatch(0.5)).toBe("none");
    expect(classifyMatch(0.0)).toBe("none");
    expect(classifyMatch(0.59)).toBe("none");
  });
});
