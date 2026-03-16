import { APIClient } from "../lib/api";
import type { EnvironmentConfig } from "../lib/types";
import { validateRelease } from "../lib/schemas";
import {
  checkDuplicate,
  type DuplicateCheckResult,
  type FieldComparison,
} from "../lib/duplicates";
import { TagResolver, formatTagsPreview, formatFuzzyWarning } from "../lib/tags";
import type { TagInput, ResolvedTag } from "../lib/tags";
import * as display from "../lib/display";
import { dim } from "../lib/ansi";

// -- Types --

interface ReleaseArtistInput {
  name: string;
  role?: string;
}

interface ReleaseLinkInput {
  platform: string;
  url: string;
}

interface ReleaseInput {
  title: string;
  release_type?: string;
  release_year?: number;
  release_date?: string;
  cover_art_url?: string;
  description?: string;
  artists: ReleaseArtistInput[];
  labels?: string[];
  external_links?: ReleaseLinkInput[];
  tags?: TagInput[];
}

interface ResolvedArtist {
  artist_id: number;
  name: string;
  role: string;
}

interface ReleaseAction {
  release: ReleaseInput;
  action: "create" | "update" | "skip";
  dupCheck: DuplicateCheckResult;
  resolvedArtists: ResolvedArtist[];
  unresolvedArtists: string[];
  validationErrors?: string[];
}

export interface SubmitResult {
  created: number;
  updated: number;
  skipped: number;
  errors: number;
  actions: ReleaseAction[];
}

// -- Core logic --

/** Parse release JSON input (single object or array). */
export function parseReleaseInput(json: string): ReleaseInput[] {
  const parsed = JSON.parse(json);

  if (Array.isArray(parsed)) {
    return parsed;
  }

  return [parsed];
}

/** Normalize the artists field: accept array of strings or array of objects. */
function normalizeArtists(
  artists: unknown,
): ReleaseArtistInput[] {
  if (!Array.isArray(artists)) return [];

  return artists.map((a) => {
    if (typeof a === "string") {
      return { name: a, role: "main" };
    }
    if (a && typeof a === "object" && "name" in a) {
      return { name: String(a.name), role: String(a.role || "main") };
    }
    return { name: String(a), role: "main" };
  });
}

/** Resolve an artist name to an ID via the search API. */
async function resolveArtist(
  client: APIClient,
  name: string,
): Promise<{ id: number; name: string } | null> {
  try {
    const result = await client.get<{
      artists: Array<{ id: number; name: string; slug: string }>;
    }>("/artists/search", { q: name });

    if (!result.artists?.length) return null;

    // Find exact match first (case-insensitive)
    const normalizedName = name.toLowerCase().trim();
    const exact = result.artists.find(
      (a) => a.name.toLowerCase().trim() === normalizedName,
    );
    if (exact) return { id: exact.id, name: exact.name };

    // Fall back to first result if it's a close match
    const first = result.artists[0];
    if (first.name.toLowerCase().includes(normalizedName) ||
        normalizedName.includes(first.name.toLowerCase())) {
      return { id: first.id, name: first.name };
    }

    return null;
  } catch {
    return null;
  }
}

/** Plan release actions: validate, check duplicates, resolve artists. */
export async function planReleases(
  client: APIClient,
  releases: ReleaseInput[],
): Promise<ReleaseAction[]> {
  const actions: ReleaseAction[] = [];

  for (const release of releases) {
    // Normalize artists field
    release.artists = normalizeArtists(release.artists);

    // Validate
    const validation = validateRelease(release);
    if (!validation.valid) {
      actions.push({
        release,
        action: "skip",
        dupCheck: { action: "skip", match: "none", fields: [], confidence: 0 },
        resolvedArtists: [],
        unresolvedArtists: [],
        validationErrors: validation.errors.map(
          (e) => `${e.field}: ${e.message}`,
        ),
      });
      continue;
    }

    // Check for duplicates using release title
    const dupCheck = await checkDuplicate(
      client,
      "release",
      release as unknown as Record<string, unknown>,
    );

    // Resolve artists
    const resolvedArtists: ResolvedArtist[] = [];
    const unresolvedArtists: string[] = [];

    for (const artist of release.artists) {
      const resolved = await resolveArtist(client, artist.name);
      if (resolved) {
        resolvedArtists.push({
          artist_id: resolved.id,
          name: resolved.name,
          role: artist.role || "main",
        });
      } else {
        unresolvedArtists.push(artist.name);
      }
    }

    // Log labels as informational
    if (release.labels?.length) {
      for (const label of release.labels) {
        display.info(
          `Label "${label}" noted ${dim("(label linkage is via artist associations)")}`,
        );
      }
    }

    actions.push({
      release,
      action: dupCheck.action,
      dupCheck,
      resolvedArtists,
      unresolvedArtists,
    });
  }

  return actions;
}

/** Display the planned actions as a preview. */
export function displayPreview(actions: ReleaseAction[], resolvedTags?: ResolvedTag[][]): void {
  let creates = 0;
  let updates = 0;
  let skips = 0;

  for (let i = 0; i < actions.length; i++) {
    const action = actions[i];
    const title = action.release.title || "(untitled)";

    if (action.validationErrors?.length) {
      display.error(`${title} - validation failed`);
      for (const err of action.validationErrors) {
        display.kv("  Error", err);
      }
      skips++;
      continue;
    }

    switch (action.action) {
      case "create": {
        display.success(`CREATE: ${title}`);
        if (action.release.release_type) {
          display.kv("Type", action.release.release_type);
        }
        if (action.release.release_year) {
          display.kv("Year", String(action.release.release_year));
        }
        if (action.resolvedArtists.length > 0) {
          display.kv(
            "Artists",
            action.resolvedArtists
              .map((a) => `${a.name} (ID: ${a.artist_id}, ${a.role})`)
              .join(", "),
          );
        }
        if (action.unresolvedArtists.length > 0) {
          display.warn(
            `Unresolved artists: ${action.unresolvedArtists.join(", ")}`,
          );
        }
        if (action.release.external_links?.length) {
          display.kv(
            "Links",
            action.release.external_links
              .map((l) => `${l.platform}: ${l.url}`)
              .join(", "),
          );
        }
        creates++;
        break;
      }
      case "update": {
        display.info(
          `UPDATE: ${title} (matched: "${action.dupCheck.existingName}", ID: ${action.dupCheck.existingId})`,
        );
        const newFields = action.dupCheck.fields.filter(
          (f) => f.status === "new_info",
        );
        for (const field of newFields) {
          display.fieldDiff(field.field, field.existing, field.proposed);
        }
        if (action.unresolvedArtists.length > 0) {
          display.warn(
            `Unresolved artists: ${action.unresolvedArtists.join(", ")}`,
          );
        }
        updates++;
        break;
      }
      case "skip": {
        display.kv(
          `SKIP`,
          `${title} ${dim(`(matches "${action.dupCheck.existingName}", ID: ${action.dupCheck.existingId})`)}`,
        );
        skips++;
        break;
      }
    }

    // Show tags if any
    if (resolvedTags && resolvedTags[i].length > 0) {
      display.kv("tags", formatTagsPreview(resolvedTags[i]));
      for (const tag of resolvedTags[i]) {
        const warning = formatFuzzyWarning(tag);
        if (warning) display.warn(warning);
      }
    }
  }

  display.summary(creates, updates, skips);
}

/** Execute the planned actions (create/update releases). */
async function executeActions(
  client: APIClient,
  actions: ReleaseAction[],
  tagResolver?: TagResolver,
): Promise<SubmitResult> {
  let created = 0;
  let updated = 0;
  let skipped = 0;
  let errors = 0;

  for (const action of actions) {
    if (action.validationErrors?.length) {
      skipped++;
      continue;
    }

    const parsedTags = tagResolver
      ? TagResolver.parseTags(action.release.tags as TagInput[] | undefined)
      : [];

    if (action.action === "skip") {
      // Still apply tags even on skip
      if (tagResolver && action.dupCheck.existingId && parsedTags.length > 0) {
        const tagResult = await tagResolver.applyToEntity("release", action.dupCheck.existingId, parsedTags);
        if (tagResult.applied > 0) {
          display.info(`  Applied ${tagResult.applied} tag(s)`);
        }
      }
      skipped++;
      continue;
    }

    if (action.action === "create") {
      // Need at least one resolved artist
      if (action.resolvedArtists.length === 0) {
        display.error(
          `Cannot create "${action.release.title}": no resolved artists`,
        );
        errors++;
        continue;
      }

      try {
        const body: Record<string, unknown> = {
          title: action.release.title,
          artists: action.resolvedArtists.map((a) => ({
            artist_id: a.artist_id,
            role: a.role,
          })),
        };

        if (action.release.release_type) {
          body.release_type = action.release.release_type;
        }
        if (action.release.release_year != null) {
          body.release_year = action.release.release_year;
        }
        if (action.release.release_date) {
          body.release_date = action.release.release_date;
        }
        if (action.release.cover_art_url) {
          body.cover_art_url = action.release.cover_art_url;
        }
        if (action.release.description) {
          body.description = action.release.description;
        }
        if (action.release.external_links?.length) {
          body.external_links = action.release.external_links;
        }

        const result = await client.post<{ id?: number; release?: { id: number } }>("/releases", body);
        const releaseId = result.release?.id ?? result.id;
        display.success(`Created: ${action.release.title}`);
        // Apply tags if any
        if (tagResolver && releaseId && parsedTags.length > 0) {
          const tagResult = await tagResolver.applyToEntity("release", releaseId, parsedTags);
          if (tagResult.applied > 0) {
            display.info(`  Applied ${tagResult.applied} tag(s)`);
          }
        }
        created++;
      } catch (err) {
        const message =
          err instanceof Error ? err.message : "Unknown error";
        display.error(
          `Failed to create "${action.release.title}": ${message}`,
        );
        errors++;
      }
    }

    if (action.action === "update") {
      // Only send fields that are new_info
      const newFields = action.dupCheck.fields.filter(
        (f) => f.status === "new_info",
      );

      if (newFields.length === 0) {
        // Still apply tags even when no field updates
        if (tagResolver && action.dupCheck.existingId && parsedTags.length > 0) {
          const tagResult = await tagResolver.applyToEntity("release", action.dupCheck.existingId, parsedTags);
          if (tagResult.applied > 0) {
            display.info(`  Applied ${tagResult.applied} tag(s)`);
          }
        }
        skipped++;
        continue;
      }

      const body: Record<string, unknown> = {};
      for (const field of newFields) {
        body[field.field] = field.proposed;
      }

      // Convert release_year back to number if present
      if (body.release_year && typeof body.release_year === "string") {
        body.release_year = parseInt(body.release_year, 10);
      }

      try {
        await client.put(
          `/releases/${action.dupCheck.existingId}`,
          body,
        );
        display.success(
          `Updated: ${action.release.title} (ID: ${action.dupCheck.existingId})`,
        );
        // Apply tags if any
        if (tagResolver && action.dupCheck.existingId && parsedTags.length > 0) {
          const tagResult = await tagResolver.applyToEntity("release", action.dupCheck.existingId, parsedTags);
          if (tagResult.applied > 0) {
            display.info(`  Applied ${tagResult.applied} tag(s)`);
          }
        }
        updated++;
      } catch (err) {
        const message =
          err instanceof Error ? err.message : "Unknown error";
        display.error(
          `Failed to update "${action.release.title}": ${message}`,
        );
        errors++;
      }
    }
  }

  return { created, updated, skipped, errors, actions };
}

/** Main entry point for the submit release command. */
export async function submitReleases(
  jsonInput: string,
  env: EnvironmentConfig,
  confirm: boolean,
): Promise<SubmitResult> {
  // Parse input
  let releases: ReleaseInput[];
  try {
    releases = parseReleaseInput(jsonInput);
  } catch {
    display.error("Invalid JSON input");
    return { created: 0, updated: 0, skipped: 0, errors: 1, actions: [] };
  }

  if (releases.length === 0) {
    display.warn("No releases to process.");
    return { created: 0, updated: 0, skipped: 0, errors: 0, actions: [] };
  }

  display.header(`Processing ${releases.length} release${releases.length !== 1 ? "s" : ""}...`);

  const client = new APIClient(env);

  // Plan
  const actions = await planReleases(client, releases);

  // Resolve tags for all releases
  const tagResolver = new TagResolver(client);
  const resolvedTags: ResolvedTag[][] = [];
  for (const action of actions) {
    const tags = TagResolver.parseTags(action.release.tags as TagInput[] | undefined);
    if (tags.length > 0 && !action.validationErrors?.length) {
      resolvedTags.push(await tagResolver.resolveAll(tags));
    } else {
      resolvedTags.push([]);
    }
  }

  // Preview
  displayPreview(actions, resolvedTags);

  if (!confirm) {
    display.info('Dry run complete. Use --confirm to submit.');
    return {
      created: 0,
      updated: 0,
      skipped: actions.filter(
        (a) => a.action === "skip" || a.validationErrors?.length,
      ).length,
      errors: 0,
      actions,
    };
  }

  // Execute
  display.header("Submitting...");
  return executeActions(client, actions, tagResolver);
}
