import type { APIClient } from "../lib/api";
import type { EnvironmentConfig } from "../lib/types";
import type { DuplicateCheckResult, FieldComparison } from "../lib/duplicates";
import { checkDuplicate } from "../lib/duplicates";
import { validateArtist } from "../lib/schemas";
import { TagResolver, formatTagsPreview, formatFuzzyWarning } from "../lib/tags";
import type { TagInput, ResolvedTag } from "../lib/tags";
import { resolveAndLinkArtistLabel } from "../lib/labels";
import * as display from "../lib/display";
import { green, yellow, gray, dim } from "../lib/ansi";

export interface SubmitArtistResult {
  name: string;
  action: "created" | "updated" | "skipped" | "error";
  id?: number;
  error?: string;
}

export interface SubmitArtistsOptions {
  confirm?: boolean;
  force?: boolean;
}

/**
 * Parse JSON input into an array of artist objects.
 * Accepts a single object or an array.
 */
export function parseArtistInput(input: string): Record<string, unknown>[] {
  const parsed = JSON.parse(input);

  if (Array.isArray(parsed)) {
    return parsed;
  }

  if (parsed && typeof parsed === "object") {
    return [parsed];
  }

  throw new Error("Input must be a JSON object or array of objects");
}

/**
 * Read JSON from stdin (for piped input).
 */
async function readStdin(): Promise<string> {
  const chunks: string[] = [];
  const reader = Bun.stdin.stream().getReader();

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    chunks.push(new TextDecoder().decode(value));
  }

  return chunks.join("");
}

/**
 * Display the preview for a single artist based on duplicate check results.
 */
export function displayArtistPreview(
  artist: Record<string, unknown>,
  dupResult: DuplicateCheckResult,
  index: number,
): void {
  const name = String(artist.name || "(unnamed)");

  switch (dupResult.action) {
    case "create":
      display.header(`[${index + 1}] CREATE: ${name}`);
      // Show the fields that will be set
      for (const [key, val] of Object.entries(artist)) {
        if (val !== undefined && val !== null && val !== "") {
          display.kv(key, String(val));
        }
      }
      break;

    case "update":
      display.header(
        `[${index + 1}] UPDATE: ${dupResult.existingName || name} (ID ${dupResult.existingId})`,
      );
      display.info(
        `Match: ${dupResult.match} (confidence: ${(dupResult.confidence * 100).toFixed(0)}%)`,
      );
      for (const field of dupResult.fields) {
        display.fieldDiff(field.field, field.existing, field.proposed);
      }
      break;

    case "skip":
      display.header(`[${index + 1}] SKIP: ${dupResult.existingName || name}`);
      display.info(
        `Already exists (ID ${dupResult.existingId}), no new info to add.`,
      );
      break;
  }
}

/**
 * Build the PATCH body for an update: only include fields with new_info status.
 */
export function buildUpdateBody(
  fields: FieldComparison[],
): Record<string, string> {
  const body: Record<string, string> = {};
  for (const field of fields) {
    if (field.status === "new_info") {
      body[field.field] = field.proposed;
    }
  }
  return body;
}

/**
 * Core submit artists logic. Exported for testing.
 */
export async function submitArtists(
  client: APIClient,
  artists: Record<string, unknown>[],
  options: SubmitArtistsOptions,
): Promise<SubmitArtistResult[]> {
  const results: SubmitArtistResult[] = [];

  // Phase 1: Validate all artists
  const validationErrors: { index: number; name: string; errors: string[] }[] =
    [];

  for (let i = 0; i < artists.length; i++) {
    const artist = artists[i];
    const validation = validateArtist(artist);
    if (!validation.valid) {
      validationErrors.push({
        index: i,
        name: String(artist.name || `(item ${i + 1})`),
        errors: validation.errors.map((e) => `${e.field}: ${e.message}`),
      });
    }
  }

  if (validationErrors.length > 0) {
    for (const ve of validationErrors) {
      display.error(`Validation failed for "${ve.name}":`);
      for (const err of ve.errors) {
        display.kv("  ", err);
      }
    }
    return validationErrors.map((ve) => ({
      name: ve.name,
      action: "error" as const,
      error: ve.errors.join("; "),
    }));
  }

  // Phase 2: Check duplicates for all artists (skip with --force)
  const dupResults: DuplicateCheckResult[] = [];
  if (options.force) {
    display.info("Skipping duplicate check (--force)");
    for (const artist of artists) {
      dupResults.push({ action: "create" as const, match: "none" as const, fields: [], confidence: 0 });
    }
  } else {
    display.info("Checking for duplicates...");
    for (let i = 0; i < artists.length; i++) {
      const result = await checkDuplicate(client, "artist", artists[i]);
      dupResults.push(result);
    }
  }

  // Phase 2b: Resolve tags for all artists
  const tagResolver = new TagResolver(client);
  const resolvedTags: ResolvedTag[][] = [];
  for (const artist of artists) {
    const tags = TagResolver.parseTags(artist.tags as TagInput[] | undefined);
    if (tags.length > 0) {
      resolvedTags.push(await tagResolver.resolveAll(tags));
    } else {
      resolvedTags.push([]);
    }
  }

  // Phase 3: Display preview
  let creates = 0;
  let updates = 0;
  let skips = 0;

  for (let i = 0; i < artists.length; i++) {
    displayArtistPreview(artists[i], dupResults[i], i);

    // Show label if specified
    const labelField = artists[i].label;
    if (typeof labelField === "string" && labelField) {
      display.kv("label", `${labelField} (will link after create/resolve)`);
    }

    // Show tags if any
    if (resolvedTags[i].length > 0) {
      display.kv("tags", formatTagsPreview(resolvedTags[i]));
      for (const tag of resolvedTags[i]) {
        const warning = formatFuzzyWarning(tag);
        if (warning) display.warn(warning);
      }
    }

    switch (dupResults[i].action) {
      case "create":
        creates++;
        break;
      case "update":
        updates++;
        break;
      case "skip":
        skips++;
        break;
    }
  }

  display.summary(creates, updates, skips);

  // Phase 4: Execute if --confirm
  if (!options.confirm) {
    display.info("Dry run. Use --confirm to execute.");
    return artists.map((artist, i) => ({
      name: String(artist.name || ""),
      action:
        dupResults[i].action === "create"
          ? ("created" as const)
          : dupResults[i].action === "update"
            ? ("updated" as const)
            : ("skipped" as const),
      id: dupResults[i].existingId,
    }));
  }

  display.info("Executing...");

  for (let i = 0; i < artists.length; i++) {
    const artist = artists[i];
    const dup = dupResults[i];
    const name = String(artist.name || "");

    // Extract label field (not an artist API field — used for linking)
    const labelName = typeof artist.label === "string" ? artist.label : undefined;

    try {
      switch (dup.action) {
        case "create": {
          // Build POST payload with only API-accepted fields.
          // Strip tags (applied separately), label (linked separately),
          // entity_type (batch routing only), and any other non-API fields.
          const artistApiFields = [
            "name", "city", "state", "country",
            "instagram", "facebook", "twitter", "youtube",
            "spotify", "soundcloud", "bandcamp", "website",
            "description",
          ];
          const artistPayload: Record<string, unknown> = {};
          for (const field of artistApiFields) {
            if (artist[field] !== undefined) {
              artistPayload[field] = artist[field];
            }
          }

          const response = await client.post<{
            artist?: { id: number; name: string };
            id?: number;
          }>("/admin/artists", artistPayload);
          const id = response.artist?.id ?? response.id;
          display.success(`Created "${name}" (ID ${id})`);
          // Apply tags if any
          if (id && resolvedTags[i].length > 0) {
            const parsedTags = TagResolver.parseTags(artist.tags as TagInput[] | undefined);
            const tagResult = await tagResolver.applyToEntity("artist", id, parsedTags);
            if (tagResult.applied > 0) {
              display.info(`  Applied ${tagResult.applied} tag(s)`);
            }
          }
          // Link artist to label if specified
          if (id && labelName) {
            await resolveAndLinkArtistLabel(client, labelName, id);
          }
          results.push({ name, action: "created", id });
          break;
        }

        case "update": {
          const updateBody = buildUpdateBody(dup.fields);
          if (Object.keys(updateBody).length === 0) {
            display.info(`No new fields to update for "${name}", skipping.`);
            // Still link to label even when no field updates
            if (dup.existingId && labelName) {
              await resolveAndLinkArtistLabel(client, labelName, dup.existingId);
            }
            results.push({
              name,
              action: "skipped",
              id: dup.existingId,
            });
            break;
          }
          await client.patch(
            `/admin/artists/${dup.existingId}`,
            updateBody,
          );
          display.success(
            `Updated "${dup.existingName || name}" (ID ${dup.existingId})`,
          );
          // Apply tags if any
          if (dup.existingId && resolvedTags[i].length > 0) {
            const parsedTags = TagResolver.parseTags(artist.tags as TagInput[] | undefined);
            const tagResult = await tagResolver.applyToEntity("artist", dup.existingId, parsedTags);
            if (tagResult.applied > 0) {
              display.info(`  Applied ${tagResult.applied} tag(s)`);
            }
          }
          // Link artist to label if specified
          if (dup.existingId && labelName) {
            await resolveAndLinkArtistLabel(client, labelName, dup.existingId);
          }
          results.push({
            name,
            action: "updated",
            id: dup.existingId,
          });
          break;
        }

        case "skip": {
          display.info(
            `Skipped "${dup.existingName || name}" (ID ${dup.existingId}) — no new info.`,
          );
          // Still apply tags even on skip
          if (dup.existingId && resolvedTags[i].length > 0) {
            const parsedTags = TagResolver.parseTags(artist.tags as TagInput[] | undefined);
            const tagResult = await tagResolver.applyToEntity("artist", dup.existingId, parsedTags);
            if (tagResult.applied > 0) {
              display.info(`  Applied ${tagResult.applied} tag(s)`);
            }
          }
          // Link artist to label even on skip
          if (dup.existingId && labelName) {
            await resolveAndLinkArtistLabel(client, labelName, dup.existingId);
          }
          results.push({
            name,
            action: "skipped",
            id: dup.existingId,
          });
          break;
        }
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : "Unknown error";
      display.error(`Failed to process "${name}": ${message}`);
      results.push({ name, action: "error", error: message });
    }
  }

  return results;
}

/**
 * CLI entry point for `ph submit artist`.
 */
export async function runSubmitArtist(
  json: string | undefined,
  env: EnvironmentConfig,
  options: SubmitArtistsOptions,
): Promise<void> {
  // Read JSON from argument or stdin
  let input: string;

  if (json) {
    input = json;
  } else if (!process.stdin.isTTY) {
    input = await readStdin();
  } else {
    display.error(
      'No JSON provided. Pass as argument or pipe via stdin.\n\n' +
        '  Usage:\n' +
        '    ph submit artist \'{"name": "Artist Name"}\'\n' +
        '    echo \'{"name": "Artist Name"}\' | ph submit artist\n',
    );
    process.exit(1);
  }

  input = input.trim();
  if (!input) {
    display.error("Empty input. Provide JSON data.");
    process.exit(1);
  }

  // Parse JSON
  let artists: Record<string, unknown>[];
  try {
    artists = parseArtistInput(input);
  } catch {
    display.error("Invalid JSON input. Expected a JSON object or array.");
    process.exit(1);
  }

  if (artists.length === 0) {
    display.warn("No artists to process (empty array).");
    return;
  }

  display.info(`Processing ${artists.length} artist${artists.length !== 1 ? "s" : ""}...`);

  const { APIClient } = await import("../lib/api");
  const client = new APIClient(env);

  const results = await submitArtists(client, artists, options);

  // Check for any errors and set exit code
  const hasErrors = results.some((r) => r.action === "error");
  if (hasErrors) {
    process.exit(1);
  }
}
