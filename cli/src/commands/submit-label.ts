import { APIClient } from "../lib/api";
import type { EnvironmentConfig } from "../lib/types";
import { validateLabel } from "../lib/schemas";
import { checkDuplicate, type DuplicateCheckResult } from "../lib/duplicates";
import { TagResolver, formatTagsPreview, formatFuzzyWarning } from "../lib/tags";
import type { TagInput, ResolvedTag } from "../lib/tags";
import { expandInlineRosters } from "../lib/roster";
import * as display from "../lib/display";

interface LabelInput {
  name: string;
  city?: string;
  state?: string;
  country?: string;
  website?: string;
  description?: string;
  /** Canonical Bandcamp field (matches the backend `bandcamp` API field). */
  bandcamp?: string;
  /** Legacy alias for `bandcamp`; normalized away before submit. */
  bandcamp_url?: string;
  /** Inline roster — expanded into artist items upstream; never sent to the label API. */
  artists?: unknown[];
  [key: string]: unknown;
}

/**
 * Normalize the legacy `bandcamp_url` alias onto the canonical `bandcamp` field.
 * The backend label API only accepts `bandcamp` (see catalog/label.go), so a
 * stray `bandcamp_url` would otherwise be silently dropped.
 */
function normalizeBandcamp(label: LabelInput): void {
  if (
    typeof label.bandcamp_url === "string" &&
    label.bandcamp_url &&
    !(typeof label.bandcamp === "string" && label.bandcamp)
  ) {
    label.bandcamp = label.bandcamp_url;
  }
  delete label.bandcamp_url;
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

    const label = item as LabelInput;
    normalizeBandcamp(label);
    validated.push({ label, index: i });
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

  // Phase 2b: Resolve tags for all labels
  const tagResolver = new TagResolver(client);
  const resolvedTags: ResolvedTag[][] = [];
  for (const { label } of plans) {
    const tags = TagResolver.parseTags(label.tags as TagInput[] | undefined);
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

  for (let planIdx = 0; planIdx < plans.length; planIdx++) {
    const { label, dupResult } = plans[planIdx];
    switch (dupResult.action) {
      case "create": {
        display.header(`CREATE: ${label.name}`);
        if (label.city) display.kv("City", label.city);
        if (label.state) display.kv("State", label.state);
        if (label.country) display.kv("Country", label.country);
        if (label.website) display.kv("Website", label.website);
        if (label.bandcamp) display.kv("Bandcamp", label.bandcamp);
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

    // Show tags if any
    if (resolvedTags[planIdx].length > 0) {
      display.kv("tags", formatTagsPreview(resolvedTags[planIdx]));
      for (const tag of resolvedTags[planIdx]) {
        const warning = formatFuzzyWarning(tag);
        if (warning) display.warn(warning);
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

  for (let planIdx = 0; planIdx < plans.length; planIdx++) {
    const { label, dupResult } = plans[planIdx];
    const parsedTags = TagResolver.parseTags(label.tags as TagInput[] | undefined);

    try {
      switch (dupResult.action) {
        case "create": {
          // Build POST payload with only API-accepted fields.
          // Strip tags (applied separately), entity_type (batch routing only),
          // label_name (not part of label API).
          const labelApiFields = [
            "name", "city", "state", "country", "website",
            "description", "bandcamp",
            "founded_year", "status",
            "instagram", "facebook", "twitter", "youtube",
            "spotify", "soundcloud",
          ];
          const labelPayload: Record<string, unknown> = {};
          for (const field of labelApiFields) {
            if (label[field] !== undefined) {
              labelPayload[field] = label[field];
            }
          }

          const response = await client.post<{
            label?: { id: number };
            id?: number;
          }>("/labels", labelPayload);
          const labelId = response.label?.id ?? response.id;
          display.success(`Created: ${label.name}`);
          // Apply tags if any
          if (labelId && parsedTags.length > 0) {
            const tagResult = await tagResolver.applyToEntity("label", labelId, parsedTags);
            if (tagResult.applied > 0) {
              display.info(`  Applied ${tagResult.applied} tag(s)`);
            }
          }
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
          // Apply tags if any
          if (dupResult.existingId && parsedTags.length > 0) {
            const tagResult = await tagResolver.applyToEntity("label", dupResult.existingId, parsedTags);
            if (tagResult.applied > 0) {
              display.info(`  Applied ${tagResult.applied} tag(s)`);
            }
          }
          result.updated++;
          break;
        }

        case "skip": {
          // Still apply tags even on skip
          if (dupResult.existingId && parsedTags.length > 0) {
            const tagResult = await tagResolver.applyToEntity("label", dupResult.existingId, parsedTags);
            if (tagResult.applied > 0) {
              display.info(`  Applied ${tagResult.applied} tag(s)`);
            }
          }
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
  const confirm = opts.confirm ?? false;

  let items: unknown[];
  try {
    items = await parseInput(json);
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    display.error(`Invalid JSON input: ${message}`);
    process.exit(1);
  }

  // Support inline rosters: a label item may carry an `artists` array. Tag each
  // input as a label, expand rosters into label + artist items, then process
  // labels first so each roster artist can resolve and link to its label.
  const tagged = (items as Record<string, unknown>[]).map((i) => ({
    ...(i as Record<string, unknown>),
    entity_type: "label",
  }));
  const { items: expanded, expandedLabels, expandedArtists } = expandInlineRosters(tagged);
  if (expandedLabels > 0) {
    display.info(
      `Expanded ${expandedLabels} label roster(s) into ${expandedArtists} artist item(s).`,
    );
  }

  const labelItems = expanded
    .filter((i) => i.entity_type === "label")
    .map(({ entity_type, ...rest }) => rest);
  const artistItems = expanded
    .filter((i) => i.entity_type === "artist")
    .map(({ entity_type, ...rest }) => rest);

  display.info(
    `Processing ${labelItems.length} label${labelItems.length !== 1 ? "s" : ""}...`,
  );
  const result = await submitLabels(labelItems, client, confirm);

  let artistErrors = 0;
  if (artistItems.length > 0) {
    display.info(`Processing ${artistItems.length} roster artist(s)...`);
    const { submitArtists } = await import("./submit-artist");
    const artistResults = await submitArtists(client, artistItems, { confirm });
    artistErrors = artistResults.filter((r) => r.action === "error").length;
  }

  if (result.errors > 0 || artistErrors > 0) {
    process.exit(1);
  }
}
