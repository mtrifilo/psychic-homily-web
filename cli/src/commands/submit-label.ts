import { APIClient } from "../lib/api";
import type { EnvironmentConfig } from "../lib/types";
import { validateLabel } from "../lib/schemas";
import { checkDuplicate, type DuplicateCheckResult } from "../lib/duplicates";
import * as display from "../lib/display";

interface LabelInput {
  name: string;
  city?: string;
  state?: string;
  country?: string;
  website?: string;
  description?: string;
  bandcamp_url?: string;
  [key: string]: unknown;
}

interface SubmitResult {
  created: number;
  updated: number;
  skipped: number;
  errors: number;
}

/**
 * Parse JSON input, accepting either a single label object or an array.
 * Reads from the argument string or from stdin if no argument provided.
 */
export async function parseInput(jsonArg?: string): Promise<unknown[]> {
  let raw: string;

  if (jsonArg) {
    raw = jsonArg;
  } else {
    // Read from stdin
    const chunks: Uint8Array[] = [];
    for await (const chunk of process.stdin) {
      chunks.push(chunk as Uint8Array);
    }
    raw = Buffer.concat(chunks).toString("utf-8").trim();
  }

  if (!raw) {
    throw new Error("No JSON input provided. Pass JSON as argument or pipe via stdin.");
  }

  const parsed = JSON.parse(raw);
  return Array.isArray(parsed) ? parsed : [parsed];
}

/**
 * Build the update payload containing only fields with new_info status.
 */
function buildUpdatePayload(
  dupResult: DuplicateCheckResult,
  proposed: LabelInput,
): Record<string, unknown> {
  const payload: Record<string, unknown> = {};

  for (const field of dupResult.fields) {
    if (field.status === "new_info") {
      payload[field.field] = proposed[field.field];
    }
  }

  return payload;
}

/**
 * Core submit function. Validates, deduplicates, previews, and optionally submits labels.
 *
 * @param items - Parsed label objects
 * @param client - API client instance
 * @param confirm - If true, actually submit; if false, dry-run only
 * @returns Summary of results
 */
export async function submitLabels(
  items: unknown[],
  client: APIClient,
  confirm: boolean,
): Promise<SubmitResult> {
  const result: SubmitResult = { created: 0, updated: 0, skipped: 0, errors: 0 };

  // Phase 1: Validate all items
  const validated: { label: LabelInput; index: number }[] = [];

  for (let i = 0; i < items.length; i++) {
    const item = items[i];
    const validation = validateLabel(item);

    if (!validation.valid) {
      display.error(
        `Label #${i + 1}: Validation failed — ${validation.errors.map((e) => e.message).join(", ")}`,
      );
      result.errors++;
      continue;
    }

    validated.push({ label: item as LabelInput, index: i });
  }

  if (validated.length === 0) {
    display.warn("No valid labels to process.");
    return result;
  }

  // Phase 2: Check duplicates and classify actions
  const plans: {
    label: LabelInput;
    dupResult: DuplicateCheckResult;
    index: number;
  }[] = [];

  for (const { label, index } of validated) {
    const dupResult = await checkDuplicate(
      client,
      "label",
      label as Record<string, unknown>,
    );
    plans.push({ label, dupResult, index });
  }

  // Phase 3: Display preview
  let creates = 0;
  let updates = 0;
  let skips = 0;

  for (const { label, dupResult } of plans) {
    switch (dupResult.action) {
      case "create": {
        display.header(`CREATE: ${label.name}`);
        if (label.city) display.kv("City", label.city);
        if (label.state) display.kv("State", label.state);
        if (label.country) display.kv("Country", label.country);
        if (label.website) display.kv("Website", label.website);
        if (label.bandcamp_url) display.kv("Bandcamp", label.bandcamp_url);
        if (label.description) display.kv("Description", label.description);
        creates++;
        break;
      }

      case "update": {
        display.header(
          `UPDATE: ${label.name} → ${dupResult.existingName} (#${dupResult.existingId})`,
        );
        for (const field of dupResult.fields) {
          display.fieldDiff(field.field, field.existing, field.proposed);
        }
        updates++;
        break;
      }

      case "skip": {
        display.info(
          `SKIP: ${label.name} — already exists as "${dupResult.existingName}" (#${dupResult.existingId})`,
        );
        skips++;
        break;
      }
    }
  }

  display.summary(creates, updates, skips);

  // Phase 4: Execute if --confirm
  if (!confirm) {
    display.warn('Dry-run mode. Pass --confirm to submit.');
    result.created = creates;
    result.updated = updates;
    result.skipped = skips;
    return result;
  }

  for (const { label, dupResult } of plans) {
    try {
      switch (dupResult.action) {
        case "create": {
          await client.post("/labels", label);
          display.success(`Created: ${label.name}`);
          result.created++;
          break;
        }

        case "update": {
          const payload = buildUpdatePayload(dupResult, label);
          if (Object.keys(payload).length > 0) {
            await client.put(`/labels/${dupResult.existingId}`, payload);
            display.success(
              `Updated: ${dupResult.existingName} (#${dupResult.existingId})`,
            );
          } else {
            display.info(
              `No new fields to update for ${dupResult.existingName}`,
            );
          }
          result.updated++;
          break;
        }

        case "skip": {
          result.skipped++;
          break;
        }
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      display.error(`Failed to submit ${label.name}: ${message}`);
      result.errors++;
    }
  }

  return result;
}

/**
 * CLI entry point for `ph submit label`.
 */
export async function runSubmitLabel(
  json: string | undefined,
  opts: { confirm?: boolean },
  env: EnvironmentConfig,
): Promise<void> {
  const client = new APIClient(env);

  let items: unknown[];
  try {
    items = await parseInput(json);
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    display.error(`Invalid JSON input: ${message}`);
    process.exit(1);
  }

  display.info(`Processing ${items.length} label${items.length !== 1 ? "s" : ""}...`);

  const result = await submitLabels(items, client, opts.confirm ?? false);

  if (result.errors > 0) {
    process.exit(1);
  }
}
