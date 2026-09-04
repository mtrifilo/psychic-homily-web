import { describe, test, expect } from "bun:test";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import assert from "../eval/assert";
import type { BatchItem } from "../eval/scoring";

/**
 * The promptfoo assertion adapter, which is the layer that decides whether a
 * fixture passes and which numbers get reported. `scoring.ts` is unit-tested
 * separately; this covers the wiring around it, including the one fatal
 * condition (schema-invalid output) and the door-time component.
 */

function fixture(slug: string): BatchItem[] {
  return JSON.parse(
    readFileSync(join(import.meta.dir, "../eval/fixtures", slug, "expected.json"), "utf-8"),
  );
}

const positive = fixture("lh-st-lincoln-hall-2026-09");
const negative = fixture("empty-bottle-2026-09");

function grade(expected: BatchItem[], actual: BatchItem[]) {
  return assert(JSON.stringify(actual), { vars: { expected_json: expected } });
}

describe("eval assertion adapter", () => {
  test("the golden reproduced exactly passes with every component at 1", () => {
    const result = grade(positive, positive);
    expect(result.pass).toBe(true);
    expect(result.namedScores?.schema_valid).toBe(1);
    expect(result.namedScores?.show_times_agreement).toBe(1);
    expect(result.score).toBe(1);
  });

  test("schema-invalid output is the one fatal condition", () => {
    const result = grade(positive, [
      { entity_type: "show", city: "Chicago", state: "IL" } as BatchItem,
    ]);
    expect(result.pass).toBe(false);
    expect(result.reason).toBeTruthy();
  });

  test("dropping a labelled door time scores the door-time component zero", () => {
    const withoutDoors = positive.map((item) =>
      item.entity_type === "show" ? { ...item, doors_at: undefined } : item,
    );
    const result = grade(positive, withoutDoors);
    expect(result.namedScores?.show_times_agreement).toBe(0);
  });

  test("inventing a time on an unlabelled listing fails the door-time component", () => {
    // The negative fixture's whole purpose: both goldens state no time, so a
    // model that files the bare clock as music_at must not come out clean.
    const invented = negative.map((item) =>
      item.entity_type === "show" ? { ...item, music_at: "10:00PM" } : item,
    );
    const result = grade(negative, invented);
    expect(result.namedScores?.show_times_agreement).toBe(0);

    const component = result.componentResults?.find((c) =>
      c.namedScores?.show_times_agreement !== undefined,
    );
    expect(component?.pass).toBe(false);
    expect(component?.reason).toContain("music=22:00");
  });

  test("the negative fixture reproduced exactly reports no invented time", () => {
    const result = grade(negative, negative);
    expect(result.namedScores?.show_times_agreement).toBe(1);
    const component = result.componentResults?.find((c) =>
      c.namedScores?.show_times_agreement !== undefined,
    );
    expect(component?.pass).toBe(true);
    expect(component?.reason).toContain("invented: none");
  });

  test("a fixture with no golden shows omits the component entirely", () => {
    // A vacuous 1.0 reads as "perfect" and averages into the cross-fixture
    // column alongside fixtures that actually measured something.
    const noShows: BatchItem[] = [
      { entity_type: "artist", name: "Tool" },
      { entity_type: "venue", name: "Douglas Park", city: "Chicago", state: "IL" },
    ];
    const result = grade(noShows, noShows);
    expect("show_times_agreement" in (result.namedScores ?? {})).toBe(false);
    expect(
      result.componentResults?.some(
        (c) => c.namedScores?.show_times_agreement !== undefined,
      ),
    ).toBe(false);
  });

  test("a misconfigured fixture is reported as a harness error, not a model failure", () => {
    const result = assert("[]", { vars: { expected_json: [] } });
    expect(result.pass).toBe(false);
    expect(result.reason).toContain("expected_json");
  });
});
