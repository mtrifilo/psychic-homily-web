/**
 * Pure scoring logic for /ingest extraction evals.
 *
 * Compares a model's extracted batch JSON against a human-verified golden batch
 * JSON and produces per-entity accuracy metrics: artists found / missed /
 * hallucinated, venue correctness, festival field correctness, and billing-tier
 * agreement. Kept free of any promptfoo / I/O dependency so it is unit-testable
 * with `bun test` (cli/test/eval-scoring.test.ts) and reusable by a fallback
 * harness if promptfoo is ever dropped.
 */

import { parseClockTime } from "../src/lib/showTimes";

export interface BatchItem {
  entity_type: string;
  name?: string;
  title?: string;
  city?: string;
  state?: string;
  series_slug?: string;
  edition_year?: number;
  start_date?: string;
  end_date?: string;
  artists?: Array<{ name: string; billing_tier?: string; is_headliner?: boolean }>;
  [key: string]: unknown;
}

export interface EntityScore {
  /** count present in the golden set */
  expected: number;
  /** count correctly produced */
  found: number;
  /** golden items the model failed to produce */
  missed: string[];
  /** model items not in the golden set (false positives) */
  hallucinated: string[];
  /** found / expected, in [0, 1]; 1.0 when expected is 0 */
  recall: number;
}

export interface FestivalFieldScore {
  field: string;
  expected: unknown;
  actual: unknown;
  correct: boolean;
}

export interface ShowTimesScore {
  /** count of golden shows, each of which states one schedule */
  expected: number;
  /** count of golden schedules the model reproduced */
  found: number;
  /** golden schedules the model did not produce */
  missed: string[];
  /** schedules the model produced that no golden show states */
  invented: string[];
  /** found / expected, in [0, 1]; 1.0 when there are no golden shows */
  recall: number;
}

export interface ExtractionScore {
  artists: EntityScore;
  venues: EntityScore;
  /** Agreement between the golden shows' stated schedules and the model's. */
  showTimes: ShowTimesScore;
  /** Per-festival field correctness (name, dates, slug, year). */
  festivalFields: FestivalFieldScore[];
  /** Fraction of golden lineup artists whose billing_tier matches in the model output. */
  billingTierAgreement: { matched: number; comparable: number; rate: number };
  /** Weighted overall score in [0, 1]. */
  overall: number;
}

/** Case-insensitive, whitespace-trimmed key for name matching. Preserves accents. */
export function normalizeName(name: string): string {
  return name.trim().toLowerCase().replace(/\s+/g, " ");
}

function itemsOfType(batch: BatchItem[], type: string): BatchItem[] {
  return batch.filter((x) => x.entity_type === type);
}

/**
 * Score a single entity type (artist/venue) by name-set recall and false positives.
 *
 * Names are matched on a normalized key (case/whitespace-insensitive), so two
 * golden entries that normalize to the same key collapse to one — `expected`
 * counts UNIQUE normalized names, not raw rows. Today's fixtures have no such
 * collisions; if a future fixture has a legitimately duplicated name, count it
 * once in the golden or this metric will report fewer expected than rows.
 */
export function scoreEntitySet(
  expected: BatchItem[],
  actual: BatchItem[],
  nameKey: "name" | "title" = "name",
): EntityScore {
  const expectedNames = new Map<string, string>();
  for (const e of expected) {
    const raw = e[nameKey] as string | undefined;
    if (raw) expectedNames.set(normalizeName(raw), raw);
  }
  const actualNames = new Map<string, string>();
  for (const a of actual) {
    const raw = a[nameKey] as string | undefined;
    if (raw) actualNames.set(normalizeName(raw), raw);
  }

  const missed: string[] = [];
  let found = 0;
  for (const [key, raw] of expectedNames) {
    if (actualNames.has(key)) found++;
    else missed.push(raw);
  }

  const hallucinated: string[] = [];
  for (const [key, raw] of actualNames) {
    if (!expectedNames.has(key)) hallucinated.push(raw);
  }

  const expectedCount = expectedNames.size;
  return {
    expected: expectedCount,
    found,
    missed,
    hallucinated,
    recall: expectedCount === 0 ? 1 : found / expectedCount,
  };
}

/** Compare a festival's scalar fields against the golden festival. */
export function scoreFestivalFields(
  expected: BatchItem | undefined,
  actual: BatchItem | undefined,
): FestivalFieldScore[] {
  if (!expected) return [];
  const fields = ["name", "series_slug", "edition_year", "start_date", "end_date"] as const;
  return fields.map((field) => {
    const exp = expected[field];
    const act = actual?.[field];
    const correct =
      act !== undefined &&
      String(act).trim().toLowerCase() === String(exp).trim().toLowerCase();
    return { field, expected: exp, actual: act, correct };
  });
}

/** Fraction of golden lineup artists whose billing_tier matches the model's. */
export function scoreBillingTiers(
  expectedFestival: BatchItem | undefined,
  actualFestival: BatchItem | undefined,
): { matched: number; comparable: number; rate: number } {
  const expArtists = expectedFestival?.artists ?? [];
  const actArtists = actualFestival?.artists ?? [];
  const actByName = new Map<string, string | undefined>();
  for (const a of actArtists) {
    if (a.name) actByName.set(normalizeName(a.name), a.billing_tier);
  }

  let matched = 0;
  let comparable = 0;
  for (const e of expArtists) {
    if (!e.name || e.billing_tier === undefined) continue;
    const key = normalizeName(e.name);
    if (!actByName.has(key)) continue; // artist missing entirely — counted by recall, not here
    comparable++;
    if (actByName.get(key) === e.billing_tier) matched++;
  }
  return { matched, comparable, rate: comparable === 0 ? 1 : matched / comparable };
}

/**
 * One stated time as the fact it names rather than as it was typed: the clock
 * "HH:MM" when the time is readable, "?" plus the raw text when the source
 * stated something unreadable, and "" when nothing was stated.
 *
 * Read through the same parser the ingest path uses, so a model that writes
 * "7:30 PM" for an image reading "7:30PM" agrees with the golden: both reach
 * `shows.doors_at` as the same instant, and a spacing difference is not an
 * extraction error. A stated-but-unreadable value stays distinct from an absent
 * one because those are different claims about the source.
 */
function scheduleKeyPart(value: unknown): string {
  if (typeof value !== "string" || value.trim() === "") return "";
  const clock = parseClockTime(value);
  if (clock === null) return `?${value.trim().toLowerCase()}`;
  return `${String(clock.hour).padStart(2, "0")}:${String(clock.minute).padStart(2, "0")}`;
}

/** The pair of times one show states, as a single comparable key. */
function scheduleKey(show: BatchItem): string {
  return `${scheduleKeyPart(show.doors_at)}|${scheduleKeyPart(show.music_at)}`;
}

/**
 * Compare the schedules the golden shows state against the model's, as
 * MULTISETS rather than per show.
 *
 * The two failure modes these fixtures exist to catch are both answerable
 * without deciding which extracted show is which: a labelled door/show time the
 * model dropped, and a time the model invented from a listing that labelled
 * none. Matching show-to-show would need a key built from the event date or the
 * bill, which would make a year the source never printed, or one misread band
 * name, read as a missing TIME. Multiset comparison keeps this metric about the
 * clocks.
 *
 * A golden show whose listing states no time contributes the empty schedule, so
 * a model that invents one for it scores zero here and the invented value is
 * named in `invented`.
 *
 * The denominator is every golden show, which folds one MISSING show into this
 * metric as a missing schedule. That is the price of not keying on the show, and
 * it is bounded: `artists` already reports what was dropped, and `invented`
 * separates "made a time up" from "did not produce the show at all".
 */
export function scoreShowTimes(expected: BatchItem[], actual: BatchItem[]): ShowTimesScore {
  const expectedKeys = itemsOfType(expected, "show").map(scheduleKey);
  const remaining = new Map<string, number>();
  for (const key of expectedKeys) remaining.set(key, (remaining.get(key) ?? 0) + 1);

  const invented: string[] = [];
  let found = 0;
  for (const key of itemsOfType(actual, "show").map(scheduleKey)) {
    const left = remaining.get(key) ?? 0;
    if (left > 0) {
      remaining.set(key, left - 1);
      found++;
    } else {
      invented.push(key);
    }
  }

  const missed: string[] = [];
  for (const [key, count] of remaining) {
    for (let i = 0; i < count; i++) missed.push(key);
  }

  return {
    expected: expectedKeys.length,
    found,
    missed,
    invented,
    recall: expectedKeys.length === 0 ? 1 : found / expectedKeys.length,
  };
}

/**
 * Score a model's extraction against the golden batch.
 *
 * `overall` weights artist recall most heavily (it is the dominant correctness
 * signal for a lineup) and folds in venue recall, festival-field correctness,
 * and billing-tier agreement. Hallucinations apply a proportional penalty to the
 * artist component so a model that invents artists cannot score a perfect recall.
 */
export function scoreExtraction(expected: BatchItem[], actual: BatchItem[]): ExtractionScore {
  const artists = scoreEntitySet(itemsOfType(expected, "artist"), itemsOfType(actual, "artist"));
  const venues = scoreEntitySet(itemsOfType(expected, "venue"), itemsOfType(actual, "venue"));

  const expFestival = itemsOfType(expected, "festival")[0];
  const actFestival = itemsOfType(actual, "festival")[0];
  const festivalFields = scoreFestivalFields(expFestival, actFestival);
  const billingTierAgreement = scoreBillingTiers(expFestival, actFestival);

  // Artist component: recall minus a hallucination penalty proportional to
  // false positives relative to the expected count (capped so it can't go below 0).
  const hallucPenalty =
    artists.expected === 0 ? 0 : Math.min(artists.recall, artists.hallucinated.length / artists.expected);
  const artistComponent = Math.max(0, artists.recall - hallucPenalty);

  const venueComponent = venues.recall;
  const festivalComponent =
    festivalFields.length === 0
      ? 1
      : festivalFields.filter((f) => f.correct).length / festivalFields.length;
  const billingComponent = billingTierAgreement.rate;
  const showTimes = scoreShowTimes(expected, actual);

  // Weights: artists dominate; venue/festival/billing are secondary signals.
  // These four sum to 1. Show times join at 0.15 only for a fixture whose golden
  // HAS shows, renormalized rather than reserved, so a fixture with none scores
  // exactly what these four give it.
  const base =
    0.55 * artistComponent +
    0.1 * venueComponent +
    0.2 * festivalComponent +
    0.15 * billingComponent;
  const overall =
    showTimes.expected === 0 ? base : (base + 0.15 * showTimes.recall) / 1.15;

  return { artists, venues, showTimes, festivalFields, billingTierAgreement, overall };
}

/** Human-readable one-screen summary of an ExtractionScore. */
export function formatScore(score: ExtractionScore): string {
  const lines: string[] = [];
  const a = score.artists;
  lines.push(
    `Artists: ${a.found}/${a.expected} found (recall ${(a.recall * 100).toFixed(1)}%), ` +
      `${a.missed.length} missed, ${a.hallucinated.length} hallucinated`,
  );
  if (a.missed.length) lines.push(`  missed: ${a.missed.join(", ")}`);
  if (a.hallucinated.length) lines.push(`  hallucinated: ${a.hallucinated.join(", ")}`);

  const v = score.venues;
  lines.push(`Venues: ${v.found}/${v.expected} found (recall ${(v.recall * 100).toFixed(1)}%)`);
  if (v.missed.length) lines.push(`  missed: ${v.missed.join(", ")}`);
  if (v.hallucinated.length) lines.push(`  hallucinated: ${v.hallucinated.join(", ")}`);

  const t = score.showTimes;
  if (t.expected > 0) {
    lines.push(
      `Show times: ${t.found}/${t.expected} schedules matched (${(t.recall * 100).toFixed(1)}%)`,
    );
    if (t.missed.length) lines.push(`  missed: ${t.missed.join(", ")}`);
    if (t.invented.length) lines.push(`  invented: ${t.invented.join(", ")}`);
  }

  lines.push("Festival fields:");
  for (const f of score.festivalFields) {
    const mark = f.correct ? "ok " : "MISS";
    lines.push(`  [${mark}] ${f.field}: expected ${JSON.stringify(f.expected)}, got ${JSON.stringify(f.actual)}`);
  }

  const b = score.billingTierAgreement;
  lines.push(
    `Billing-tier agreement: ${b.matched}/${b.comparable} (${(b.rate * 100).toFixed(1)}%)`,
  );
  lines.push(`Overall score: ${(score.overall * 100).toFixed(1)}%`);
  return lines.join("\n");
}

/**
 * Extract a JSON array from a model response that may be wrapped in markdown
 * fences or surrounded by prose. Returns the parsed array, or throws.
 */
export function parseModelBatch(output: string): BatchItem[] {
  let text = output.trim();
  // Strip ```json ... ``` or ``` ... ``` fences.
  const fenceMatch = text.match(/```(?:json)?\s*([\s\S]*?)```/);
  if (fenceMatch) text = fenceMatch[1].trim();
  // If there is leading/trailing prose, grab the outermost array.
  if (!text.startsWith("[")) {
    const start = text.indexOf("[");
    const end = text.lastIndexOf("]");
    if (start !== -1 && end !== -1 && end > start) {
      text = text.slice(start, end + 1);
    }
  }
  const parsed = JSON.parse(text);
  if (!Array.isArray(parsed)) {
    throw new Error("model output is not a JSON array");
  }
  return parsed as BatchItem[];
}
