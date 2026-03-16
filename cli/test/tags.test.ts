import { describe, test, expect } from "bun:test";
import { TagResolver, formatTagsPreview, formatFuzzyWarning } from "../src/lib/tags";
import type { ResolvedTag, TagInput } from "../src/lib/tags";

describe("TagResolver.parseTags", () => {
  test("parses string tags as genre", () => {
    const result = TagResolver.parseTags(["punk", "noise rock"]);
    expect(result).toEqual([
      { name: "punk", category: "genre" },
      { name: "noise rock", category: "genre" },
    ]);
  });

  test("parses object tags with category", () => {
    const result = TagResolver.parseTags([
      { name: "Japanese", category: "locale" },
      { name: "industrial" },
    ]);
    expect(result).toEqual([
      { name: "Japanese", category: "locale" },
      { name: "industrial", category: "genre" },
    ]);
  });

  test("handles mixed string and object tags", () => {
    const result = TagResolver.parseTags([
      "punk",
      { name: "Japanese", category: "locale" },
      "noise rock",
    ]);
    expect(result).toHaveLength(3);
    expect(result[0]).toEqual({ name: "punk", category: "genre" });
    expect(result[1]).toEqual({ name: "Japanese", category: "locale" });
    expect(result[2]).toEqual({ name: "noise rock", category: "genre" });
  });

  test("trims whitespace from tag names", () => {
    const result = TagResolver.parseTags(["  punk  ", "  noise rock "]);
    expect(result[0].name).toBe("punk");
    expect(result[1].name).toBe("noise rock");
  });

  test("filters out empty tags", () => {
    const result = TagResolver.parseTags(["punk", "", "  ", "rock"]);
    expect(result).toHaveLength(2);
    expect(result[0].name).toBe("punk");
    expect(result[1].name).toBe("rock");
  });

  test("returns empty array for undefined", () => {
    expect(TagResolver.parseTags(undefined)).toEqual([]);
  });

  test("returns empty array for empty array", () => {
    expect(TagResolver.parseTags([])).toEqual([]);
  });
});

describe("formatTagsPreview", () => {
  test("formats existing tags", () => {
    const tags: ResolvedTag[] = [
      { id: 1, name: "punk", category: "genre", status: "exists" },
    ];
    const result = formatTagsPreview(tags);
    expect(result).toContain("punk");
    expect(result).toContain("genre");
  });

  test("formats new tags with NEW marker", () => {
    const tags: ResolvedTag[] = [
      { id: 0, name: "lukthung", category: "genre", status: "created" },
    ];
    const result = formatTagsPreview(tags);
    expect(result).toContain("lukthung");
    expect(result).toContain("NEW");
  });

  test("formats fuzzy matches with arrow", () => {
    const tags: ResolvedTag[] = [
      {
        id: 5,
        name: "post-punk",
        category: "genre",
        status: "fuzzy_match",
        originalName: "post punk",
      },
    ];
    const result = formatTagsPreview(tags);
    expect(result).toContain("post punk");
    expect(result).toContain("post-punk");
    expect(result).toContain("MATCHED");
  });

  test("returns empty string for no tags", () => {
    expect(formatTagsPreview([])).toBe("");
  });
});

describe("formatFuzzyWarning", () => {
  test("returns warning for fuzzy match", () => {
    const tag: ResolvedTag = {
      id: 5,
      name: "post-punk",
      category: "genre",
      status: "fuzzy_match",
      originalName: "post punk",
    };
    const warning = formatFuzzyWarning(tag);
    expect(warning).toContain("post punk");
    expect(warning).toContain("post-punk");
  });

  test("returns empty for non-fuzzy match", () => {
    const tag: ResolvedTag = {
      id: 1,
      name: "punk",
      category: "genre",
      status: "exists",
    };
    expect(formatFuzzyWarning(tag)).toBe("");
  });

  test("returns empty for new tag", () => {
    const tag: ResolvedTag = {
      id: 0,
      name: "lukthung",
      category: "genre",
      status: "created",
    };
    expect(formatFuzzyWarning(tag)).toBe("");
  });
});
