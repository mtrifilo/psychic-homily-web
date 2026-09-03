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
  artists?: Array<{
    name: string;
    billing_tier?: string;
    is_headliner?: boolean;
    set_type?: string;
  }>;
  venues?: Array<{ name?: string; city?: string; state?: string; is_primary?: boolean }>;
  price?: number | string;
  door_price?: number | string;
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

/** matched / comparable agreement on one family of fields, in [0, 1]. */
export interface FieldAgreement {
  matched: number;
  comparable: number;
  rate: number;
  /** One line per disagreement, for the eval report. */
  mismatches: string[];
}

export interface ShowFieldScore {
  /** Golden shows the model produced at all (matched on date + venue). */
  shows: { expected: number; matched: number; missed: string[] };
  /**
   * `price` and `door_price` compared INCLUDING absence: a show the source
   * states one price for must carry no `door_price` at all.
   */
  prices: FieldAgreement;
  /**
   * `set_type` compared INCLUDING absence, over the acts the model produced.
   * A slot the source never stated must come back unstated.
   */
  billRoles: FieldAgreement;
}

export interface ExtractionScore {
  artists: EntityScore;
  venues: EntityScore;
  /** Per-festival field correctness (name, dates, slug, year). */
  festivalFields: FestivalFieldScore[];
  /** Fraction of golden lineup artists whose billing_tier matches in the model output. */
  billingTierAgreement: { matched: number; comparable: number; rate: number };
  /** Show price + bill-role agreement, both scored on absence as well as value. */
  showFields: ShowFieldScore;
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
 * A show's identity for matching a model's shows against the golden ones:
 * the date it happens and the venue it happens at. A tour post emits one show
 * per date, so the date alone is nearly always enough; the venue disambiguates
 * two shows on the same night.
 */
function showKey(show: BatchItem): string {
  const first = show.venues?.[0];
  const venue = first?.name ? normalizeName(first.name) : "";
  return `${String(show.event_date ?? "").trim()}|${venue}`;
}

/**
 * The value a show states for a price field, or `null` when it states none.
 *
 * Absence is a value here, and the point of the metric: the extraction rules
 * forbid deriving `door_price` from `price`, so a show whose source names one
 * price must come back with no `door_price` key at all. `null`, `undefined` and
 * `""` are all the same silence.
 *
 * A present value that is not a number yields `NaN`, which equals nothing —
 * including itself — so an unreadable price is scored as a disagreement rather
 * than quietly matching an absent one.
 */
function statedPrice(show: BatchItem, field: "price" | "door_price"): number | null {
  const raw = show[field];
  if (raw === undefined || raw === null || raw === "") return null;
  if (typeof raw === "number") return raw;
  const parsed = Number(String(raw).replace(/[$,\s]/g, ""));
  return Number.isFinite(parsed) ? parsed : NaN;
}

/**
 * The bill slot an act states, or `null` when it states none.
 *
 * The backend's own resolution ladder: a curated `set_type` wins, the legacy
 * `is_headliner` flag decides only in its absence, and anything else is silence.
 * Reading `is_headliner` here is what makes the metric catch the failure the
 * rules exist to prevent — a model that infers the headliner from the top of
 * the poster reports a slot the source never stated.
 *
 * `performer` is silence, not a role: it is the value every uncurated row
 * holds, so a model that spells "slot unknown" as `performer` and one that
 * omits the key are saying the same thing and score the same.
 */
function statedSlot(act: { set_type?: string; is_headliner?: boolean }): string | null {
  const raw = act.set_type;
  if (typeof raw === "string") {
    const trimmed = raw.trim();
    if (trimmed !== "" && trimmed !== "performer") return trimmed;
  }
  return act.is_headliner === true ? "headliner" : null;
}

function agreement(matched: number, comparable: number, mismatches: string[]): FieldAgreement {
  return { matched, comparable, rate: comparable === 0 ? 1 : matched / comparable, mismatches };
}

/**
 * Score the show fields the extraction rules govern by an "only when stated"
 * rule: the split advance/door price, and the curated bill role.
 *
 * Scored over the golden shows the model actually produced. A golden show the
 * model missed entirely is reported as missed and contributes nothing to the
 * field rates, mirroring how billing-tier agreement leaves a missing artist to
 * the recall metric.
 */
export function scoreShowFields(expected: BatchItem[], actual: BatchItem[]): ShowFieldScore {
  const expectedShows = itemsOfType(expected, "show");
  const actualByKey = new Map<string, BatchItem>();
  for (const show of itemsOfType(actual, "show")) {
    actualByKey.set(showKey(show), show);
  }

  const missed: string[] = [];
  let matchedShows = 0;
  let priceMatched = 0;
  let priceComparable = 0;
  const priceMismatches: string[] = [];
  let roleMatched = 0;
  let roleComparable = 0;
  const roleMismatches: string[] = [];

  for (const exp of expectedShows) {
    const key = showKey(exp);
    const act = actualByKey.get(key);
    if (!act) {
      missed.push(key);
      continue;
    }
    matchedShows++;

    for (const field of ["price", "door_price"] as const) {
      priceComparable++;
      const e = statedPrice(exp, field);
      const a = statedPrice(act, field);
      if (e === a) priceMatched++;
      else priceMismatches.push(`${key} ${field}: expected ${e}, got ${a}`);
    }

    const actByName = new Map<string, { set_type?: string; is_headliner?: boolean }>();
    for (const a of act.artists ?? []) {
      if (a.name) actByName.set(normalizeName(a.name), a);
    }
    for (const e of exp.artists ?? []) {
      if (!e.name) continue;
      const a = actByName.get(normalizeName(e.name));
      // Missing act: counted by artist recall, not here.
      if (!a) continue;
      roleComparable++;
      const expSlot = statedSlot(e);
      const actSlot = statedSlot(a);
      if (expSlot === actSlot) roleMatched++;
      else roleMismatches.push(`${key} ${e.name}: expected ${expSlot}, got ${actSlot}`);
    }
  }

  return {
    shows: { expected: expectedShows.length, matched: matchedShows, missed },
    prices: agreement(priceMatched, priceComparable, priceMismatches),
    billRoles: agreement(roleMatched, roleComparable, roleMismatches),
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

  // Weights: artists dominate; venue/festival/billing are secondary signals.
  //
  // Show-field agreement is reported beside `overall` rather than folded into
  // it, so a score stays comparable with the one a fixture recorded before the
  // metric existed. A fixture with no golden shows would otherwise gain a
  // vacuous perfect component and drift upward.
  const overall =
    0.55 * artistComponent +
    0.1 * venueComponent +
    0.2 * festivalComponent +
    0.15 * billingComponent;

  return {
    artists,
    venues,
    festivalFields,
    billingTierAgreement,
    showFields: scoreShowFields(expected, actual),
    overall,
  };
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

  lines.push("Festival fields:");
  for (const f of score.festivalFields) {
    const mark = f.correct ? "ok " : "MISS";
    lines.push(`  [${mark}] ${f.field}: expected ${JSON.stringify(f.expected)}, got ${JSON.stringify(f.actual)}`);
  }

  const b = score.billingTierAgreement;
  lines.push(
    `Billing-tier agreement: ${b.matched}/${b.comparable} (${(b.rate * 100).toFixed(1)}%)`,
  );

  const s = score.showFields;
  if (s.shows.expected > 0) {
    lines.push(`Shows: ${s.shows.matched}/${s.shows.expected} matched`);
    if (s.shows.missed.length) lines.push(`  missed: ${s.shows.missed.join(", ")}`);
    lines.push(
      `Show prices (absence included): ${s.prices.matched}/${s.prices.comparable} ` +
        `(${(s.prices.rate * 100).toFixed(1)}%)`,
    );
    for (const m of s.prices.mismatches) lines.push(`  ${m}`);
    lines.push(
      `Bill roles (absence included): ${s.billRoles.matched}/${s.billRoles.comparable} ` +
        `(${(s.billRoles.rate * 100).toFixed(1)}%)`,
    );
    for (const m of s.billRoles.mismatches) lines.push(`  ${m}`);
  }

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
