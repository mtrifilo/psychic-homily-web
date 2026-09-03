import { describe, test, expect } from "bun:test";
import {
  SET_TYPE_VOCABULARY,
  SET_TYPE_UNCURATED,
  curatedSetType,
  isUnroundtrippableSetType,
  isValidSetType,
  setTypeVocabularyCSV,
} from "../src/lib/setType";

describe("isValidSetType", () => {
  test("accepts every value in the vocabulary", () => {
    for (const value of SET_TYPE_VOCABULARY) {
      expect(isValidSetType(value)).toBe(true);
    }
  });

  test("is strict about case, spacing and near-misses", () => {
    for (const value of [
      "Headliner",
      "HEADLINER",
      " headliner",
      "headliner ",
      "support",
      "co-headliner",
      "host",
      "mc",
      "",
    ]) {
      expect(isValidSetType(value)).toBe(false);
    }
  });
});

describe("setTypeVocabularyCSV", () => {
  test("names every accepted role, top of bill first", () => {
    expect(setTypeVocabularyCSV()).toBe(
      "headliner, direct_support, opener, special_guest, dj, performer",
    );
  });
});

describe("curatedSetType", () => {
  test("returns a curated role unchanged", () => {
    expect(curatedSetType("direct_support")).toBe("direct_support");
    expect(curatedSetType("headliner")).toBe("headliner");
    expect(curatedSetType("dj")).toBe("dj");
  });

  test("reads the two spellings of an unknown slot as nothing to state", () => {
    expect(curatedSetType(SET_TYPE_UNCURATED)).toBeUndefined();
    expect(curatedSetType("")).toBeUndefined();
    expect(curatedSetType("   ")).toBeUndefined();
    expect(curatedSetType(null)).toBeUndefined();
    expect(curatedSetType(undefined)).toBeUndefined();
  });

  test("returns nothing for a role the API would reject", () => {
    expect(curatedSetType("co-headliner")).toBeUndefined();
    expect(curatedSetType("Headliner")).toBeUndefined();
  });

  test("tolerates surrounding whitespace on a real role", () => {
    expect(curatedSetType("  opener  ")).toBe("opener");
  });
});

describe("isUnroundtrippableSetType", () => {
  test("is true only for a stated role outside the vocabulary", () => {
    expect(isUnroundtrippableSetType("co-headliner")).toBe(true);
    expect(isUnroundtrippableSetType("Headliner")).toBe(true);
  });

  test("is false for every curated role and for silence", () => {
    for (const value of SET_TYPE_VOCABULARY) {
      expect(isUnroundtrippableSetType(value)).toBe(false);
    }
    expect(isUnroundtrippableSetType("")).toBe(false);
    expect(isUnroundtrippableSetType("  ")).toBe(false);
    expect(isUnroundtrippableSetType(null)).toBe(false);
    expect(isUnroundtrippableSetType(undefined)).toBe(false);
  });
});
