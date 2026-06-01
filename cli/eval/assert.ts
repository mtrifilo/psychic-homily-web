/**
 * Promptfoo custom assertion for the /ingest extraction eval.
 *
 * Thin adapter: parses the model output, scores it against the golden batch
 * (the `expected_json` test var, loaded by promptfoo from `file://...expected.json`)
 * using the pure logic in scoring.ts, validates JSON-schema shape, and returns a
 * promptfoo GradingResult with per-metric namedScores + componentResults.
 *
 * Reported as `assert: { type: javascript, value: file://assert.ts }`.
 */
import Ajv from "ajv";
import { readFileSync } from "node:fs";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import {
  scoreExtraction,
  parseModelBatch,
  formatScore,
  type BatchItem,
} from "./scoring.ts";

const HERE = dirname(fileURLToPath(import.meta.url));

interface GradingResult {
  pass: boolean;
  score: number;
  reason: string;
  componentResults?: GradingResult[];
  namedScores?: Record<string, number>;
}

interface AssertContext {
  vars: { expected_json?: BatchItem[] | string; [k: string]: unknown };
}

let schemaValidator: ((data: unknown) => boolean) | null = null;
function getSchemaValidator() {
  if (schemaValidator) return schemaValidator;
  const schema = JSON.parse(readFileSync(join(HERE, "batch-schema.json"), "utf-8"));
  const ajv = new Ajv({ allErrors: false, strict: false });
  const validate = ajv.compile(schema);
  schemaValidator = (data: unknown) => validate(data) as boolean;
  return schemaValidator;
}

export default function assert(output: string, context: AssertContext): GradingResult {
  // promptfoo auto-parses JSON output to an object; re-stringify so our
  // fence-tolerant parser sees a consistent string input.
  const raw = typeof output === "string" ? output : JSON.stringify(output);

  let actual: BatchItem[];
  try {
    actual = parseModelBatch(raw);
  } catch (err) {
    return {
      pass: false,
      score: 0,
      reason: `Model output did not parse as a JSON batch array: ${(err as Error).message}`,
    };
  }

  const expectedVar = context.vars.expected_json;
  let expected: BatchItem[];
  try {
    expected = typeof expectedVar === "string" ? JSON.parse(expectedVar) : (expectedVar ?? []);
  } catch (err) {
    // A misconfigured fixture (expected_json not valid JSON) is a harness error,
    // not a model failure — surface it clearly rather than crashing the run.
    return {
      pass: false,
      score: 0,
      reason: `expected_json is not valid JSON — check the fixture wiring: ${(err as Error).message}`,
    };
  }
  if (!Array.isArray(expected) || expected.length === 0) {
    return { pass: false, score: 0, reason: "No expected_json array provided to assertion" };
  }

  const schemaValid = getSchemaValidator()(actual);
  const score = scoreExtraction(expected, actual);

  const componentResults: GradingResult[] = [
    {
      pass: schemaValid,
      score: schemaValid ? 1 : 0,
      reason: schemaValid ? "Output conforms to batch-schema.json" : "Output violates batch-schema.json",
      namedScores: { schema_valid: schemaValid ? 1 : 0 },
    },
    {
      pass: score.artists.recall >= 0.8,
      score: score.artists.recall,
      reason: `Artists ${score.artists.found}/${score.artists.expected} (${score.artists.missed.length} missed, ${score.artists.hallucinated.length} hallucinated)`,
      namedScores: {
        artist_recall: score.artists.recall,
        artists_missed: score.artists.missed.length,
        artists_hallucinated: score.artists.hallucinated.length,
      },
    },
    {
      pass: score.venues.recall >= 1,
      score: score.venues.recall,
      reason: `Venues ${score.venues.found}/${score.venues.expected}`,
      namedScores: { venue_recall: score.venues.recall },
    },
    {
      pass: score.festivalFields.every((f) => f.correct),
      score:
        score.festivalFields.length === 0
          ? 1
          : score.festivalFields.filter((f) => f.correct).length / score.festivalFields.length,
      reason: `Festival fields: ${score.festivalFields.filter((f) => f.correct).length}/${score.festivalFields.length} correct`,
      namedScores: Object.fromEntries(
        score.festivalFields.map((f) => [`festival_${f.field}`, f.correct ? 1 : 0]),
      ),
    },
    {
      pass: score.billingTierAgreement.rate >= 0.8,
      score: score.billingTierAgreement.rate,
      reason: `Billing-tier agreement ${score.billingTierAgreement.matched}/${score.billingTierAgreement.comparable}`,
      namedScores: { billing_tier_agreement: score.billingTierAgreement.rate },
    },
  ];

  // No hard pass/fail gate (per PSY-935 — thresholds are a later user decision).
  // We surface the score and always "pass" the run so the eval reports numbers
  // rather than failing a build. Schema invalidity is the one fatal condition.
  return {
    pass: schemaValid,
    score: score.overall,
    reason: formatScore(score),
    componentResults,
    namedScores: {
      overall: score.overall,
      artist_recall: score.artists.recall,
      venue_recall: score.venues.recall,
      billing_tier_agreement: score.billingTierAgreement.rate,
      schema_valid: schemaValid ? 1 : 0,
    },
  };
}
