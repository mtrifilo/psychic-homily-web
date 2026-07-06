import { APIClient } from "../lib/api";
import type { EnvironmentConfig } from "../lib/types";
import * as display from "../lib/display";
import { rematchRadioPlaysChunked, rematchRadioPlays, dedupeArtistNames, aggregateRematchResults } from "../lib/radio";
import { expandInlineRosters, type RosterItem } from "../lib/roster";
import { green, yellow, gray, dim } from "../lib/ansi";

// -- Types --

/** Valid entity types for batch processing. */
const ENTITY_TYPES = ["label", "artist", "release", "venue", "festival", "show"] as const;
type BatchEntityType = (typeof ENTITY_TYPES)[number];

/** Processing order ensures dependencies are created before dependents. */
const PROCESSING_ORDER: BatchEntityType[] = [
  "label",
  "artist",
  "release",
  "venue",
  "festival",
  "show",
];

interface BatchItem {
  entity_type: string;
  [key: string]: unknown;
}

interface BatchGroupResult {
  type: BatchEntityType;
  total: number;
  created: number;
  updated: number;
  skipped: number;
  errors: number;
}

export interface BatchResult {
  groups: BatchGroupResult[];
  totalProcessed: number;
  totalCreated: number;
  totalUpdated: number;
  totalSkipped: number;
  totalErrors: number;
}

// -- Parsing & Validation --

/**
 * Parse and validate a batch JSON file.
 * Returns the parsed items or throws on invalid input.
 */
export function parseBatchInput(content: string): BatchItem[] {
  let parsed: unknown;
  try {
    parsed = JSON.parse(content);
  } catch {
    throw new Error("Invalid JSON: could not parse file contents");
  }

  if (!Array.isArray(parsed)) {
    throw new Error("Batch file must contain a JSON array");
  }

  return parsed as BatchItem[];
}

/**
 * Validate that all batch items have a valid entity_type field.
 * Returns validation errors (empty array = all valid).
 */
export function validateBatchItems(
  items: BatchItem[],
): { index: number; error: string }[] {
  const errors: { index: number; error: string }[] = [];

  for (let i = 0; i < items.length; i++) {
    const item = items[i];

    if (!item || typeof item !== "object") {
      errors.push({ index: i, error: "Item is not an object" });
      continue;
    }

    if (!item.entity_type) {
      errors.push({ index: i, error: "Missing required field: entity_type" });
      continue;
    }

    if (typeof item.entity_type !== "string") {
      errors.push({ index: i, error: "entity_type must be a string" });
      continue;
    }

    if (!ENTITY_TYPES.includes(item.entity_type as BatchEntityType)) {
      errors.push({
        index: i,
        error: `Invalid entity_type "${item.entity_type}". Must be one of: ${ENTITY_TYPES.join(", ")}`,
      });
    }
  }

  return errors;
}

/**
 * Group batch items by entity type, preserving order within each group.
 */
export function groupByType(
  items: BatchItem[],
): Map<BatchEntityType, Record<string, unknown>[]> {
  const groups = new Map<BatchEntityType, Record<string, unknown>[]>();

  for (const type of PROCESSING_ORDER) {
    groups.set(type, []);
  }

  for (const item of items) {
    const type = item.entity_type as BatchEntityType;
    const { entity_type, ...rest } = item;
    groups.get(type)!.push(rest);
  }

  return groups;
}

/** Artist/label names from a batch file — scoped post-confirm radio rematch. */
export function batchRematchNames(items: BatchItem[]): {
  artistNames: string[];
  labelNames: string[];
} {
  const artists: string[] = [];
  const labels: string[] = [];
  for (const item of items) {
    if (typeof item.name !== "string" || !item.name.trim()) continue;
    if (item.entity_type === "artist") artists.push(item.name);
    if (item.entity_type === "label") labels.push(item.name);
  }
  return {
    artistNames: dedupeArtistNames(artists),
    labelNames: dedupeArtistNames(labels),
  };
}

// -- Core batch logic --

/**
 * Process a batch of mixed entities in dependency order.
 * Reuses the core submit functions from each entity module.
 */
export async function processBatch(
  items: BatchItem[],
  env: EnvironmentConfig,
  confirm: boolean,
): Promise<BatchResult> {
  const groups = groupByType(items);
  const client = new APIClient(env);
  const result: BatchResult = {
    groups: [],
    totalProcessed: 0,
    totalCreated: 0,
    totalUpdated: 0,
    totalSkipped: 0,
    totalErrors: 0,
  };

  for (const type of PROCESSING_ORDER) {
    const groupItems = groups.get(type)!;
    if (groupItems.length === 0) continue;

    display.header(`Processing ${groupItems.length} ${type}(s)...`);

    const groupResult = await processGroup(client, type, groupItems, env, confirm);
    result.groups.push(groupResult);
    result.totalProcessed += groupResult.total;
    result.totalCreated += groupResult.created;
    result.totalUpdated += groupResult.updated;
    result.totalSkipped += groupResult.skipped;
    result.totalErrors += groupResult.errors;
  }

  // Final summary
  display.header("Batch Summary");
  for (const group of result.groups) {
    const parts: string[] = [];
    if (group.created > 0) parts.push(green(`${group.created} created`));
    if (group.updated > 0) parts.push(yellow(`${group.updated} updated`));
    if (group.skipped > 0) parts.push(gray(`${group.skipped} skipped`));
    if (group.errors > 0) parts.push(`${group.errors} error(s)`);
    display.kv(
      `${group.type}s (${group.total})`,
      parts.join(", ") || dim("none"),
    );
  }
  display.summary(result.totalCreated, result.totalUpdated, result.totalSkipped);

  if (!confirm) {
    display.warn("Dry run. Use --confirm to execute.");
  } else if (result.totalCreated > 0 || result.totalUpdated > 0) {
    try {
      const { artistNames, labelNames } = batchRematchNames(items);
      const artistResult =
        artistNames.length > 0
          ? await rematchRadioPlaysChunked(client, { artistNames })
          : { namesProcessed: 0, total: 0, matched: 0, unmatched: 0 };
      const labelResults = [];
      for (const labelName of labelNames) {
        labelResults.push(await rematchRadioPlays(client, { labelName }));
      }
      const labelAgg = aggregateRematchResults(labelResults);
      const matched = artistResult.matched + labelAgg.matched;
      const total = artistResult.total + labelAgg.total;
      const unmatched = artistResult.unmatched + labelAgg.unmatched;
      const namesProcessed = artistResult.namesProcessed + labelNames.length;
      if (matched > 0) {
        display.info(
          `Radio rematch linked ${matched} play(s) across ${namesProcessed} batch name(s) ` +
            `(${unmatched} still unmatched of ${total} scanned).`,
        );
      } else if (namesProcessed > 0) {
        display.info(
          `Radio rematch: no new play links across ${namesProcessed} batch name(s) ` +
            `(${total} unmatched plays scanned).`,
        );
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      display.warn(`Radio rematch failed (non-fatal): ${message}`);
    }
  }

  return result;
}

/**
 * Process a single entity type group using the corresponding submit module.
 */
async function processGroup(
  client: APIClient,
  type: BatchEntityType,
  items: Record<string, unknown>[],
  env: EnvironmentConfig,
  confirm: boolean,
): Promise<BatchGroupResult> {
  const groupResult: BatchGroupResult = {
    type,
    total: items.length,
    created: 0,
    updated: 0,
    skipped: 0,
    errors: 0,
  };

  switch (type) {
    case "artist": {
      const { submitArtists } = await import("./submit-artist");
      const results = await submitArtists(client, items, { confirm });
      for (const r of results) {
        switch (r.action) {
          case "created":
            groupResult.created++;
            break;
          case "updated":
            groupResult.updated++;
            break;
          case "skipped":
            groupResult.skipped++;
            break;
          case "error":
            groupResult.errors++;
            break;
        }
      }
      break;
    }

    case "venue": {
      const { submitVenues } = await import("./submit-venue");
      const result = await submitVenues(client, items, confirm);
      groupResult.created = result.creates;
      groupResult.updated = result.updates;
      groupResult.skipped = result.skips;
      groupResult.errors = result.errors;
      break;
    }

    case "show": {
      const { submitShows } = await import("./submit-show");
      const jsonStr = JSON.stringify(items);
      const result = await submitShows(client, jsonStr, confirm);
      groupResult.created = result.created;
      groupResult.skipped = result.skipped;
      groupResult.errors = result.failed;
      break;
    }

    case "release": {
      const { submitReleases } = await import("./submit-release");
      const jsonStr = JSON.stringify(items);
      const result = await submitReleases(jsonStr, env, confirm);
      groupResult.created = result.created;
      groupResult.updated = result.updated;
      groupResult.skipped = result.skipped;
      groupResult.errors = result.errors;
      break;
    }

    case "label": {
      const { submitLabels } = await import("./submit-label");
      const result = await submitLabels(items, client, confirm);
      groupResult.created = result.created;
      groupResult.updated = result.updated;
      groupResult.skipped = result.skipped;
      groupResult.errors = result.errors;
      break;
    }

    case "festival": {
      const { submitFestivals } = await import("./submit-festival");
      const results = await submitFestivals(
        items as unknown as Parameters<typeof submitFestivals>[0],
        env,
        confirm,
      );
      for (const r of results) {
        switch (r.action) {
          case "created":
            groupResult.created++;
            break;
          case "updated":
            groupResult.updated++;
            break;
          case "skipped":
            groupResult.skipped++;
            break;
          case "error":
            groupResult.errors++;
            break;
        }
      }
      break;
    }
  }

  return groupResult;
}

// -- CLI entry point --

/**
 * CLI entry point for `ph batch <file>`.
 */
export async function runBatch(
  filePath: string,
  env: EnvironmentConfig,
  confirm: boolean,
): Promise<void> {
  // Read file
  const file = Bun.file(filePath);
  if (!(await file.exists())) {
    display.error(`File not found: ${filePath}`);
    process.exit(1);
  }

  const content = await file.text();
  if (!content.trim()) {
    display.error("File is empty.");
    process.exit(1);
  }

  // Parse
  let items: BatchItem[];
  try {
    items = parseBatchInput(content);
  } catch (err) {
    const message = err instanceof Error ? err.message : "Invalid JSON";
    display.error(message);
    process.exit(1);
  }

  if (items.length === 0) {
    display.warn("Empty array — nothing to process.");
    return;
  }

  // Expand any inline label rosters ({entity_type:"label", artists:[...]}) into
  // flat label + artist items before validation/grouping. The existing
  // label-before-artist processing order then links each roster artist.
  const expansion = expandInlineRosters(items as RosterItem[]);
  items = expansion.items as BatchItem[];
  if (expansion.expandedLabels > 0) {
    display.info(
      `Expanded ${expansion.expandedLabels} label roster(s) into ${expansion.expandedArtists} artist item(s).`,
    );
  }

  // Validate
  const validationErrors = validateBatchItems(items);
  if (validationErrors.length > 0) {
    for (const ve of validationErrors) {
      display.error(`Item #${ve.index + 1}: ${ve.error}`);
    }
    process.exit(1);
  }

  display.info(`Batch file contains ${items.length} item(s).`);

  // Show what will be processed in which order
  const groups = groupByType(items);
  const activeGroups: string[] = [];
  for (const type of PROCESSING_ORDER) {
    const count = groups.get(type)!.length;
    if (count > 0) {
      activeGroups.push(`${count} ${type}(s)`);
    }
  }
  display.info(`Processing order: ${activeGroups.join(" -> ")}`);

  // Process
  const result = await processBatch(items, env, confirm);

  if (result.totalErrors > 0) {
    process.exit(1);
  }
}
