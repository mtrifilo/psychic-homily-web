import { describe, test, expect } from "bun:test";
import {
  SET_TYPE_VOCABULARY,
  SET_TYPE_UNCURATED,
  SET_TYPE_VOCABULARY_CSV,
  roundTrippableRole,
  isUnroundtrippableSetType,
  isValidSetType,
  statesASlot,
  statedSlot,
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

describe("SET_TYPE_VOCABULARY_CSV", () => {
  test("names every accepted role, top of bill first", () => {
    expect(SET_TYPE_VOCABULARY_CSV).toBe(
      "headliner, direct_support, opener, special_guest, dj, performer",
    );
  });
});

describe("roundTrippableRole", () => {
  test("returns a curated role unchanged", () => {
    expect(roundTrippableRole("direct_support")).toBe("direct_support");
    expect(roundTrippableRole("headliner")).toBe("headliner");
    expect(roundTrippableRole("dj")).toBe("dj");
  });

  test("reads the two spellings of an unknown slot as nothing to state", () => {
    expect(roundTrippableRole(SET_TYPE_UNCURATED)).toBeUndefined();
    expect(roundTrippableRole("")).toBeUndefined();
    expect(roundTrippableRole("   ")).toBeUndefined();
    expect(roundTrippableRole(null)).toBeUndefined();
    expect(roundTrippableRole(undefined)).toBeUndefined();
  });

  test("returns nothing for a role the API would reject", () => {
    expect(roundTrippableRole("co-headliner")).toBeUndefined();
    expect(roundTrippableRole("Headliner")).toBeUndefined();
  });

  test("judges the value untrimmed, exactly as the API would", () => {
    expect(roundTrippableRole("  opener  ")).toBeUndefined();
    expect(isUnroundtrippableSetType("  opener  ")).toBe(true);
  });
});

describe("vocabulary drift", () => {
  /**
   * The CLI's copy of the enum against its source of truth: the OpenAPI tag on
   * the shared show `Artist` schema, which is what the API actually enforces.
   *
   * Read from the Go file rather than restated, because a hand-copied enum with
   * only a hand-copied test is self-referential — it passes just as happily
   * when the CLI is the stale side. Drift here is not cosmetic: a role the CLI
   * does not know is dropped from every preserved act on every bill edit.
   */
  test("matches the enum the API publishes, value for value and in order", async () => {
    const source = await Bun.file(
      new URL(
        "../../backend/internal/api/handlers/catalog/show.go",
        import.meta.url,
      ).pathname,
    ).text();
    const match = source.match(/SetType\s+\*string\s+`json:"set_type,omitempty"\s+enum:"([^"]+)"/);
    expect(match).not.toBeNull();
    expect(match![1].split(",")).toEqual([...SET_TYPE_VOCABULARY]);
  });

  test("matches the batch schema's show-artist enum", async () => {
    const schema = await Bun.file(
      new URL("../eval/batch-schema.json", import.meta.url).pathname,
    ).json();
    expect(
      schema.definitions.show.properties.artists.items.properties.set_type.enum,
    ).toEqual([...SET_TYPE_VOCABULARY]);
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

describe("statesASlot", () => {
  test("the three spellings of silence are all silence", () => {
    expect(statesASlot(undefined)).toBe(false);
    expect(statesASlot(null)).toBe(false);
    expect(statesASlot("")).toBe(false);
    expect(statesASlot("   ")).toBe(false);
    expect(statesASlot(SET_TYPE_UNCURATED)).toBe(false);
  });

  test("any other value states a slot, vocabulary or not", () => {
    expect(statesASlot("headliner")).toBe(true);
    // Out of vocabulary is still a claim about the slot: the API refuses it,
    // it does not read it as silence.
    expect(statesASlot("co-headliner")).toBe(true);
  });
});

describe("statedSlot", () => {
  test("a curated role wins", () => {
    expect(statedSlot({ set_type: "direct_support" })).toBe("direct_support");
    expect(statedSlot({ set_type: "opener", is_headliner: true })).toBe("opener");
  });

  test("the legacy flag decides only when no role is stated", () => {
    expect(statedSlot({ is_headliner: true })).toBe("headliner");
    expect(statedSlot({ set_type: SET_TYPE_UNCURATED, is_headliner: true })).toBe(
      "headliner",
    );
  });

  test("silence and an unusable role state nothing", () => {
    expect(statedSlot({})).toBeUndefined();
    expect(statedSlot({ set_type: SET_TYPE_UNCURATED })).toBeUndefined();
    expect(statedSlot({ set_type: "co-headliner" })).toBeUndefined();
    expect(statedSlot({ is_headliner: false })).toBeUndefined();
  });
});
