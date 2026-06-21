import { describe, test, expect } from "bun:test";
import { expandInlineRosters } from "../src/lib/roster";

describe("expandInlineRosters", () => {
  test("expands a label roster into label + artist items with label injected", () => {
    const { items, expandedLabels, expandedArtists } = expandInlineRosters([
      {
        entity_type: "label",
        name: "Sacred Bones Records",
        website: "https://sacredbonesrecords.com",
        artists: [{ name: "Anika" }, { name: "Amen Dunes" }],
      },
    ]);

    expect(expandedLabels).toBe(1);
    expect(expandedArtists).toBe(2);
    expect(items).toHaveLength(3);

    // Label item is preserved, minus the roster.
    expect(items[0]).toEqual({
      entity_type: "label",
      name: "Sacred Bones Records",
      website: "https://sacredbonesrecords.com",
    });
    expect(items[0]).not.toHaveProperty("artists");

    // Each artist becomes a flat item with the label name injected.
    expect(items[1]).toEqual({
      entity_type: "artist",
      name: "Anika",
      label: "Sacred Bones Records",
    });
    expect(items[2]).toEqual({
      entity_type: "artist",
      name: "Amen Dunes",
      label: "Sacred Bones Records",
    });
  });

  test("accepts bare name strings as roster entries", () => {
    const { items, expandedArtists } = expandInlineRosters([
      { entity_type: "label", name: "Drag City", artists: ["Bill Callahan", " Joanna Newsom "] },
    ]);

    expect(expandedArtists).toBe(2);
    expect(items[1]).toEqual({ entity_type: "artist", name: "Bill Callahan", label: "Drag City" });
    // Whitespace is trimmed.
    expect(items[2]).toEqual({ entity_type: "artist", name: "Joanna Newsom", label: "Drag City" });
  });

  test("preserves an explicit per-artist label override and extra fields", () => {
    const { items } = expandInlineRosters([
      {
        entity_type: "label",
        name: "Label A",
        artists: [
          { name: "Crossover Act", label: "Label B", city: "Berlin", tags: ["krautrock"] },
        ],
      },
    ]);

    expect(items[1]).toEqual({
      entity_type: "artist",
      name: "Crossover Act",
      label: "Label B",
      city: "Berlin",
      tags: ["krautrock"],
    });
  });

  test("drops empty/garbage roster entries without minting nameless artists", () => {
    const { items, expandedArtists } = expandInlineRosters([
      { entity_type: "label", name: "Label", artists: ["", "   ", null, 42] },
    ]);

    // Only the label survives; string entries are empty and non-string scalars are dropped.
    expect(expandedArtists).toBe(0);
    expect(items).toHaveLength(1);
    expect(items[0].entity_type).toBe("label");
  });

  test("strips an empty or non-array artists key from label items", () => {
    const empty = expandInlineRosters([{ entity_type: "label", name: "L1", artists: [] }]);
    expect(empty.items[0]).toEqual({ entity_type: "label", name: "L1" });
    expect(empty.expandedLabels).toBe(0);

    const nonArray = expandInlineRosters([
      { entity_type: "label", name: "L2", artists: "oops" as unknown },
    ]);
    expect(nonArray.items[0]).toEqual({ entity_type: "label", name: "L2" });
  });

  test("passes through non-label items and rosterless labels unchanged", () => {
    const input = [
      { entity_type: "artist", name: "Solo Artist", city: "Phoenix" },
      { entity_type: "label", name: "Rosterless Label", country: "US" },
      { entity_type: "show", event_date: "2026-06-01" },
    ];
    const { items, expandedLabels, expandedArtists } = expandInlineRosters(input);

    expect(expandedLabels).toBe(0);
    expect(expandedArtists).toBe(0);
    expect(items).toEqual(input);
  });
});
