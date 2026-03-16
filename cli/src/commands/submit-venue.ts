import { APIClient } from "../lib/api";
import type { EnvironmentConfig } from "../lib/types";
import { validateVenue } from "../lib/schemas";
import { checkDuplicate, type DuplicateCheckResult } from "../lib/duplicates";
import { TagResolver, formatTagsPreview, formatFuzzyWarning } from "../lib/tags";
import type { TagInput, ResolvedTag } from "../lib/tags";
import * as display from "../lib/display";

interface VenueInput {
  name: string;
  city: string;
  state: string;
  country?: string;
  address?: string;
  zip_code?: string;
  website?: string;
  capacity?: number;
  description?: string;
}

export interface SubmitVenuesResult {
  creates: number;
  updates: number;
  skips: number;
  errors: number;
  results: Array<{
    venue: VenueInput;
    action: "create" | "update" | "skip" | "error";
    message: string;
  }>;
}

/**
 * Parse venue JSON from a CLI argument or stdin.
 * Accepts a single venue object or an array of venue objects.
 */
export async function parseVenueInput(
  jsonArg: string | undefined,
): Promise<Record<string, unknown>[]> {
  let raw: string;

  if (jsonArg) {
    raw = jsonArg;
  } else {
    // Read from stdin
    raw = await readStdin();
  }

  if (!raw.trim()) {
    throw new Error("No JSON input provided. Pass JSON as argument or pipe via stdin.");
  }

  const parsed = JSON.parse(raw);

  if (Array.isArray(parsed)) {
    return parsed;
  }

  return [parsed];
}

async function readStdin(): Promise<string> {
  const chunks: string[] = [];
  const reader = Bun.stdin.stream().getReader();

  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      chunks.push(new TextDecoder().decode(value));
    }
  } finally {
    reader.releaseLock();
  }

  return chunks.join("");
}

/**
 * Submit venues for creation/update.
 *
 * Validates each venue, checks for duplicates, previews changes,
 * and optionally executes API calls when confirm is true.
 */
export async function submitVenues(
  client: APIClient,
  venues: Record<string, unknown>[],
  confirm: boolean,
): Promise<SubmitVenuesResult> {
  const result: SubmitVenuesResult = {
    creates: 0,
    updates: 0,
    skips: 0,
    errors: 0,
    results: [],
  };

  // Step 1: Validate all venues first
  const validVenues: Array<{ venue: Record<string, unknown>; index: number }> = [];

  for (let i = 0; i < venues.length; i++) {
    const venue = venues[i];
    const validation = validateVenue(venue);

    if (!validation.valid) {
      const errorMsg = validation.errors
        .map((e) => `${e.field}: ${e.message}`)
        .join(", ");
      display.error(
        `Venue ${i + 1}: validation failed — ${errorMsg}`,
      );
      result.errors++;
      result.results.push({
        venue: venue as unknown as VenueInput,
        action: "error",
        message: `Validation failed: ${errorMsg}`,
      });
      continue;
    }

    validVenues.push({ venue, index: i });
  }

  if (validVenues.length === 0) {
    return result;
  }

  // Step 2: Check for duplicates and classify actions
  const plans: Array<{
    venue: Record<string, unknown>;
    index: number;
    dupResult: DuplicateCheckResult;
  }> = [];

  for (const { venue, index } of validVenues) {
    const dupResult = await checkDuplicate(client, "venue", venue);
    plans.push({ venue, index, dupResult });
  }

  // Step 2b: Resolve tags for all venues
  const tagResolver = new TagResolver(client);
  const resolvedTags: ResolvedTag[][] = [];
  for (const { venue } of validVenues) {
    const tags = TagResolver.parseTags(venue.tags as TagInput[] | undefined);
    if (tags.length > 0) {
      resolvedTags.push(await tagResolver.resolveAll(tags));
    } else {
      resolvedTags.push([]);
    }
  }

  // Step 3: Display preview
  display.header("Venue Submit Preview");

  for (let planIdx = 0; planIdx < plans.length; planIdx++) {
    const { venue, index, dupResult } = plans[planIdx];
    const venueName = String(venue.name);
    const venueLocation = `${venue.city}, ${venue.state}`;

    if (dupResult.action === "create") {
      display.info(`#${index + 1} CREATE: ${venueName} (${venueLocation})`);
    } else if (dupResult.action === "update") {
      display.info(
        `#${index + 1} UPDATE: ${venueName} → existing "${dupResult.existingName}" (ID ${dupResult.existingId})`,
      );
      for (const field of dupResult.fields) {
        if (field.status === "new_info") {
          display.fieldDiff(field.field, field.existing, field.proposed);
        }
      }
    } else {
      display.info(
        `#${index + 1} SKIP: ${venueName} — already exists as "${dupResult.existingName}" (ID ${dupResult.existingId})`,
      );
    }

    // Show tags if any
    if (resolvedTags[planIdx].length > 0) {
      display.kv("tags", formatTagsPreview(resolvedTags[planIdx]));
      for (const tag of resolvedTags[planIdx]) {
        const warning = formatFuzzyWarning(tag);
        if (warning) display.warn(warning);
      }
    }
  }

  // Step 4: Summary
  const createCount = plans.filter((p) => p.dupResult.action === "create").length;
  const updateCount = plans.filter((p) => p.dupResult.action === "update").length;
  const skipCount = plans.filter((p) => p.dupResult.action === "skip").length;

  display.summary(createCount, updateCount, skipCount);

  // Step 5: Execute if --confirm
  if (!confirm) {
    display.warn("Dry run — pass --confirm to execute.");
    result.creates = createCount;
    result.updates = updateCount;
    result.skips = skipCount;

    for (const { venue, dupResult } of plans) {
      result.results.push({
        venue: venue as unknown as VenueInput,
        action: dupResult.action,
        message: `Dry run: would ${dupResult.action}`,
      });
    }

    return result;
  }

  // Execute API calls
  for (let planIdx = 0; planIdx < plans.length; planIdx++) {
    const { venue, index, dupResult } = plans[planIdx];
    const venueName = String(venue.name);
    const parsedTags = TagResolver.parseTags(venue.tags as TagInput[] | undefined);

    try {
      if (dupResult.action === "create") {
        const response = await client.post<{
          venue?: { id: number };
          id?: number;
        }>("/admin/venues", venue);
        const venueId = response.venue?.id ?? response.id;
        display.success(`Created venue: ${venueName}`);
        // Apply tags if any
        if (venueId && parsedTags.length > 0) {
          const tagResult = await tagResolver.applyToEntity("venue", venueId, parsedTags);
          if (tagResult.applied > 0) {
            display.info(`  Applied ${tagResult.applied} tag(s)`);
          }
        }
        result.creates++;
        result.results.push({
          venue: venue as unknown as VenueInput,
          action: "create",
          message: "Created successfully",
        });
      } else if (dupResult.action === "update") {
        // Build update body with only new_info fields
        const updateBody: Record<string, unknown> = {};
        for (const field of dupResult.fields) {
          if (field.status === "new_info") {
            updateBody[field.field] = venue[field.field];
          }
        }

        if (Object.keys(updateBody).length > 0) {
          await client.put(
            `/venues/${dupResult.existingId}`,
            updateBody,
          );
          display.success(
            `Updated venue: ${venueName} (ID ${dupResult.existingId})`,
          );
        }
        // Apply tags if any
        if (dupResult.existingId && parsedTags.length > 0) {
          const tagResult = await tagResolver.applyToEntity("venue", dupResult.existingId, parsedTags);
          if (tagResult.applied > 0) {
            display.info(`  Applied ${tagResult.applied} tag(s)`);
          }
        }
        result.updates++;
        result.results.push({
          venue: venue as unknown as VenueInput,
          action: "update",
          message: `Updated (ID ${dupResult.existingId})`,
        });
      } else {
        display.info(`Skipped venue: ${venueName} (already exists)`);
        // Still apply tags even on skip
        if (dupResult.existingId && parsedTags.length > 0) {
          const tagResult = await tagResolver.applyToEntity("venue", dupResult.existingId, parsedTags);
          if (tagResult.applied > 0) {
            display.info(`  Applied ${tagResult.applied} tag(s)`);
          }
        }
        result.skips++;
        result.results.push({
          venue: venue as unknown as VenueInput,
          action: "skip",
          message: `Already exists (ID ${dupResult.existingId})`,
        });
      }
    } catch (err) {
      const message =
        err instanceof Error ? err.message : "Unknown error";
      display.error(`Failed to ${dupResult.action} venue #${index + 1} "${venueName}": ${message}`);
      result.errors++;
      result.results.push({
        venue: venue as unknown as VenueInput,
        action: "error",
        message: `API error: ${message}`,
      });
    }
  }

  return result;
}

/** CLI entry point for `ph submit venue`. */
export async function runSubmitVenue(
  json: string | undefined,
  opts: { confirm?: boolean },
  env: EnvironmentConfig,
): Promise<void> {
  const client = new APIClient(env);

  let venues: Record<string, unknown>[];
  try {
    venues = await parseVenueInput(json);
  } catch (err) {
    const message =
      err instanceof Error ? err.message : "Invalid JSON input";
    display.error(message);
    process.exit(1);
  }

  if (venues.length === 0) {
    display.warn("No venues in input.");
    process.exit(0);
  }

  display.info(`Processing ${venues.length} venue${venues.length !== 1 ? "s" : ""}...`);

  const result = await submitVenues(client, venues, opts.confirm ?? false);

  if (result.errors > 0) {
    process.exit(1);
  }
}
