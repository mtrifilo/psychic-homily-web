import { APIClient, APIError } from "../lib/api";
import type { EnvironmentConfig } from "../lib/types";
import * as display from "../lib/display";
import { green, yellow, gray, dim, cyan } from "../lib/ansi";
import { validateFestival } from "../lib/schemas";
import {
  checkDuplicate,
  type DuplicateCheckResult,
  type FieldComparison,
} from "../lib/duplicates";
import { TagResolver, formatTagsPreview, formatFuzzyWarning } from "../lib/tags";
import type { TagInput, ResolvedTag } from "../lib/tags";

/** Festival artist entry from the input JSON. */
interface FestivalArtistInput {
  name: string;
  billing_tier?: string;
  position?: number;
  day_date?: string;
  stage?: string;
  set_time?: string;
}

/** Festival venue entry from the input JSON. */
interface FestivalVenueInput {
  name: string;
  is_primary?: boolean;
}

/** Parsed festival input data. */
interface FestivalInput {
  name: string;
  series_slug: string;
  edition_year: number;
  start_date: string;
  end_date: string;
  description?: string;
  location_name?: string;
  city?: string;
  state?: string;
  country?: string;
  website?: string;
  ticket_url?: string;
  flyer_url?: string;
  status?: string;
  artists?: FestivalArtistInput[];
  venues?: FestivalVenueInput[];
  tags?: TagInput[];
}

/** Result of processing a single festival. */
export interface FestivalResult {
  name: string;
  action: "created" | "updated" | "skipped" | "error";
  id?: number;
  artistResults?: ArtistLinkResult[];
  venueResults?: VenueLinkResult[];
  error?: string;
}

interface ArtistLinkResult {
  name: string;
  action: "linked" | "already_linked" | "not_found" | "error";
  artistId?: number;
  error?: string;
}

interface VenueLinkResult {
  name: string;
  action: "linked" | "already_linked" | "not_found" | "error";
  venueId?: number;
  error?: string;
}

/** Planned action for a festival, after duplicate check. */
interface PlannedFestival {
  input: FestivalInput;
  dupResult: DuplicateCheckResult;
}

const VALID_BILLING_TIERS = [
  "headliner",
  "sub_headliner",
  "mid_card",
  "undercard",
  "local",
  "dj",
  "host",
];

/**
 * Resolve an artist name to an ID via GET /artists/search.
 * Returns the best match's ID, or null if not found.
 */
async function resolveArtistId(
  client: APIClient,
  name: string,
): Promise<{ id: number; name: string } | null> {
  try {
    const result = await client.get<{
      artists: Array<{ id: number; name: string; slug: string }>;
    }>("/artists/search", { q: name });

    if (!result.artists?.length) return null;

    // Look for exact match first (case-insensitive)
    const exact = result.artists.find(
      (a) => a.name.toLowerCase() === name.toLowerCase(),
    );
    if (exact) return { id: exact.id, name: exact.name };

    // Fall back to first result if it's a close enough match
    return { id: result.artists[0].id, name: result.artists[0].name };
  } catch {
    return null;
  }
}

/**
 * Resolve a venue name to an ID via GET /venues/search.
 * Returns the best match's ID, or null if not found.
 */
async function resolveVenueId(
  client: APIClient,
  name: string,
): Promise<{ id: number; name: string } | null> {
  try {
    const result = await client.get<{
      venues: Array<{ id: number; name: string; slug: string }>;
    }>("/venues/search", { q: name });

    if (!result.venues?.length) return null;

    const exact = result.venues.find(
      (v) => v.name.toLowerCase() === name.toLowerCase(),
    );
    if (exact) return { id: exact.id, name: exact.name };

    return { id: result.venues[0].id, name: result.venues[0].name };
  } catch {
    return null;
  }
}

/**
 * Build the update body, sending only fields with "new_info" status.
 */
function buildUpdateBody(
  fields: FieldComparison[],
  input: FestivalInput,
): Record<string, unknown> {
  const body: Record<string, unknown> = {};

  for (const f of fields) {
    if (f.status !== "new_info") continue;

    const value = (input as unknown as Record<string, unknown>)[f.field];
    if (value !== undefined && value !== null && value !== "") {
      body[f.field] = value;
    }
  }

  return body;
}

/**
 * Parse JSON input (single object or array) into an array of festival inputs.
 */
export function parseFestivalInput(jsonStr: string): FestivalInput[] {
  const parsed = JSON.parse(jsonStr);

  if (Array.isArray(parsed)) {
    return parsed;
  }

  return [parsed];
}

/**
 * Submit festivals: validate, check duplicates, preview, and optionally create/update.
 *
 * Exported for testing.
 */
export async function submitFestivals(
  festivals: FestivalInput[],
  env: EnvironmentConfig,
  confirm: boolean,
): Promise<FestivalResult[]> {
  const client = new APIClient(env);

  // --- Phase 1: Validate ---
  display.header("Validating festivals...");

  const validFestivals: FestivalInput[] = [];
  const results: FestivalResult[] = [];

  for (const festival of festivals) {
    const validation = validateFestival(festival);
    if (!validation.valid) {
      const errMsg = validation.errors
        .map((e) => `${e.field}: ${e.message}`)
        .join(", ");
      display.error(`${festival.name || "(unnamed)"}: ${errMsg}`);
      results.push({
        name: festival.name || "(unnamed)",
        action: "error",
        error: errMsg,
      });
      continue;
    }

    // Validate billing tiers on artists if present
    if (festival.artists) {
      let tierError = false;
      for (const artist of festival.artists) {
        if (
          artist.billing_tier &&
          !VALID_BILLING_TIERS.includes(artist.billing_tier)
        ) {
          display.error(
            `${festival.name}: artist "${artist.name}" has invalid billing_tier "${artist.billing_tier}". ` +
              `Must be one of: ${VALID_BILLING_TIERS.join(", ")}`,
          );
          tierError = true;
        }
      }
      if (tierError) {
        results.push({
          name: festival.name,
          action: "error",
          error: "Invalid billing tier",
        });
        continue;
      }
    }

    validFestivals.push(festival);
  }

  if (validFestivals.length === 0) {
    display.warn("No valid festivals to process.");
    return results;
  }

  // --- Phase 2: Duplicate check ---
  display.header("Checking for duplicates...");

  const planned: PlannedFestival[] = [];

  for (const festival of validFestivals) {
    const dupResult = await checkDuplicate(
      client,
      "festival",
      festival as unknown as Record<string, unknown>,
    );
    planned.push({ input: festival, dupResult });
  }

  // --- Phase 2b: Resolve tags ---
  const tagResolver = new TagResolver(client);
  const resolvedTags: ResolvedTag[][] = [];
  for (const p of planned) {
    const tags = TagResolver.parseTags(p.input.tags as TagInput[] | undefined);
    if (tags.length > 0) {
      resolvedTags.push(await tagResolver.resolveAll(tags));
    } else {
      resolvedTags.push([]);
    }
  }

  // --- Phase 3: Preview ---
  display.header("Preview");

  const creates = planned.filter((p) => p.dupResult.action === "create");
  const updates = planned.filter((p) => p.dupResult.action === "update");
  const skips = planned.filter((p) => p.dupResult.action === "skip");

  for (const p of creates) {
    const planIdx = planned.indexOf(p);
    const f = p.input;
    display.info(
      `${green("CREATE")} ${f.name} (${f.series_slug} ${f.edition_year})`,
    );
    display.kv("Dates", `${f.start_date} to ${f.end_date}`);
    if (f.city) display.kv("Location", `${f.city}${f.state ? `, ${f.state}` : ""}`);
    if (f.artists?.length) {
      display.kv("Artists", `${f.artists.length} to resolve`);
    }
    if (f.venues?.length) {
      display.kv("Venues", `${f.venues.length} to resolve`);
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

  for (const p of updates) {
    const planIdx = planned.indexOf(p);
    const f = p.input;
    const newFields = p.dupResult.fields.filter(
      (field) => field.status === "new_info",
    );
    display.info(
      `${yellow("UPDATE")} ${f.name} → ${p.dupResult.existingName} (ID: ${p.dupResult.existingId})`,
    );
    display.kv("Match", `${p.dupResult.match} (${(p.dupResult.confidence * 100).toFixed(0)}%)`);
    for (const field of newFields) {
      display.fieldDiff(field.field, field.existing, field.proposed);
    }
    if (f.artists?.length) {
      display.kv("Artists", `${f.artists.length} to resolve & link`);
    }
    if (f.venues?.length) {
      display.kv("Venues", `${f.venues.length} to resolve & link`);
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

  for (const p of skips) {
    const planIdx = planned.indexOf(p);
    display.info(
      `${dim("SKIP")} ${p.input.name} → already exists as "${p.dupResult.existingName}" (ID: ${p.dupResult.existingId})`,
    );
    if (p.input.artists?.length) {
      display.kv("Artists", `${p.input.artists.length} to resolve & link`);
    }
    if (p.input.venues?.length) {
      display.kv("Venues", `${p.input.venues.length} to resolve & link`);
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

  display.summary(creates.length, updates.length, skips.length);

  // --- Phase 4: Execute (if --confirm) ---
  if (!confirm) {
    // Add skip results for dry run
    for (const p of skips) {
      results.push({
        name: p.input.name,
        action: "skipped",
        id: p.dupResult.existingId,
      });
    }
    display.warn('Dry run. Pass --confirm to submit.');
    return results;
  }

  // Process skipped festivals — still link artists/venues and apply tags
  for (const p of skips) {
    const f = p.input;
    const festivalId = p.dupResult.existingId!;
    const parsedTags = TagResolver.parseTags(f.tags as TagInput[] | undefined);

    display.info(
      `Festival already exists: "${p.dupResult.existingName}" (ID: ${festivalId}), linking artists/venues...`,
    );

    const result: FestivalResult = {
      name: f.name,
      action: "skipped",
      id: festivalId,
      artistResults: [],
      venueResults: [],
    };

    // Link artists
    if (f.artists?.length) {
      result.artistResults = await linkArtists(client, festivalId, f.artists);
    }

    // Link venues
    if (f.venues?.length) {
      result.venueResults = await linkVenues(client, festivalId, f.venues);
    }

    // Apply tags
    if (festivalId && parsedTags.length > 0) {
      const tagResult = await tagResolver.applyToEntity("festival", festivalId, parsedTags);
      if (tagResult.applied > 0) {
        display.info(`  Applied ${tagResult.applied} tag(s) to "${f.name}"`);
      }
    }

    results.push(result);
  }

  display.header("Submitting...");

  // Process creates
  for (const p of creates) {
    const planIdx = planned.indexOf(p);
    const f = p.input;
    const parsedTags = TagResolver.parseTags(f.tags as TagInput[] | undefined);
    try {
      const body: Record<string, unknown> = {
        name: f.name,
        series_slug: f.series_slug,
        edition_year: f.edition_year,
        start_date: f.start_date,
        end_date: f.end_date,
      };
      if (f.description) body.description = f.description;
      if (f.location_name) body.location_name = f.location_name;
      if (f.city) body.city = f.city;
      if (f.state) body.state = f.state;
      if (f.country) body.country = f.country;
      if (f.website) body.website = f.website;
      if (f.ticket_url) body.ticket_url = f.ticket_url;
      if (f.flyer_url) body.flyer_url = f.flyer_url;
      if (f.status) body.status = f.status;

      const created = await client.post<{ id: number }>(
        "/festivals",
        body,
      );

      const festivalId = created.id;
      display.success(`Created "${f.name}" (ID: ${festivalId})`);

      const result: FestivalResult = {
        name: f.name,
        action: "created",
        id: festivalId,
        artistResults: [],
        venueResults: [],
      };

      // Link artists
      if (f.artists?.length) {
        result.artistResults = await linkArtists(client, festivalId, f.artists);
      }

      // Link venues
      if (f.venues?.length) {
        result.venueResults = await linkVenues(client, festivalId, f.venues);
      }

      // Apply tags if any
      if (festivalId && parsedTags.length > 0) {
        const tagResult = await tagResolver.applyToEntity("festival", festivalId, parsedTags);
        if (tagResult.applied > 0) {
          display.info(`  Applied ${tagResult.applied} tag(s)`);
        }
      }

      results.push(result);
    } catch (err) {
      const message = err instanceof Error ? err.message : "Unknown error";
      display.error(`Failed to create "${f.name}": ${message}`);
      results.push({ name: f.name, action: "error", error: message });
    }
  }

  // Process updates
  for (const p of updates) {
    const planIdx = planned.indexOf(p);
    const f = p.input;
    const festivalId = p.dupResult.existingId!;
    const parsedTags = TagResolver.parseTags(f.tags as TagInput[] | undefined);
    try {
      const updateBody = buildUpdateBody(p.dupResult.fields, f);

      if (Object.keys(updateBody).length > 0) {
        await client.put(`/festivals/${festivalId}`, updateBody);
        display.success(
          `Updated "${p.dupResult.existingName}" (ID: ${festivalId}) with ${Object.keys(updateBody).length} field(s)`,
        );
      } else {
        display.info(
          `No field updates needed for "${p.dupResult.existingName}" (ID: ${festivalId})`,
        );
      }

      const result: FestivalResult = {
        name: f.name,
        action: "updated",
        id: festivalId,
        artistResults: [],
        venueResults: [],
      };

      // Link artists
      if (f.artists?.length) {
        result.artistResults = await linkArtists(client, festivalId, f.artists);
      }

      // Link venues
      if (f.venues?.length) {
        result.venueResults = await linkVenues(client, festivalId, f.venues);
      }

      // Apply tags if any
      if (festivalId && parsedTags.length > 0) {
        const tagResult = await tagResolver.applyToEntity("festival", festivalId, parsedTags);
        if (tagResult.applied > 0) {
          display.info(`  Applied ${tagResult.applied} tag(s)`);
        }
      }

      results.push(result);
    } catch (err) {
      const message = err instanceof Error ? err.message : "Unknown error";
      display.error(
        `Failed to update "${p.dupResult.existingName}" (ID: ${festivalId}): ${message}`,
      );
      results.push({ name: f.name, action: "error", error: message });
    }
  }

  // --- Phase 5: Final report ---
  display.header("Results");

  const created = results.filter((r) => r.action === "created").length;
  const updated = results.filter((r) => r.action === "updated").length;
  const skipped = results.filter((r) => r.action === "skipped").length;
  const errored = results.filter((r) => r.action === "error").length;

  display.summary(created, updated, skipped);
  if (errored > 0) {
    display.error(`${errored} error(s)`);
  }

  return results;
}

/**
 * Resolve artist names and link them to a festival.
 */
async function linkArtists(
  client: APIClient,
  festivalId: number,
  artists: FestivalArtistInput[],
): Promise<ArtistLinkResult[]> {
  const linkResults: ArtistLinkResult[] = [];

  for (const artist of artists) {
    const resolved = await resolveArtistId(client, artist.name);
    if (!resolved) {
      display.warn(
        `Artist "${artist.name}" not found in database — skipping`,
      );
      linkResults.push({
        name: artist.name,
        action: "not_found",
      });
      continue;
    }

    try {
      const body: Record<string, unknown> = {
        artist_id: resolved.id,
      };
      if (artist.billing_tier) body.billing_tier = artist.billing_tier;
      if (artist.position !== undefined) body.position = artist.position;
      if (artist.day_date) body.day_date = artist.day_date;
      if (artist.stage) body.stage = artist.stage;
      if (artist.set_time) body.set_time = artist.set_time;

      await client.post(`/festivals/${festivalId}/artists`, body);
      display.success(
        `  Linked artist "${resolved.name}" (ID: ${resolved.id})${artist.billing_tier ? ` as ${artist.billing_tier}` : ""}`,
      );
      linkResults.push({
        name: artist.name,
        action: "linked",
        artistId: resolved.id,
      });
    } catch (err) {
      // Treat 409 Conflict as "already linked" — not an error
      if (err instanceof APIError && err.status === 409) {
        display.info(
          `  Already linked: "${resolved.name}" (ID: ${resolved.id})`,
        );
        linkResults.push({
          name: artist.name,
          action: "already_linked",
          artistId: resolved.id,
        });
      } else {
        const message = err instanceof Error ? err.message : "Unknown error";
        display.error(
          `  Failed to link artist "${resolved.name}": ${message}`,
        );
        linkResults.push({
          name: artist.name,
          action: "error",
          artistId: resolved.id,
          error: message,
        });
      }
    }
  }

  return linkResults;
}

/**
 * Resolve venue names and link them to a festival.
 */
async function linkVenues(
  client: APIClient,
  festivalId: number,
  venues: FestivalVenueInput[],
): Promise<VenueLinkResult[]> {
  const linkResults: VenueLinkResult[] = [];

  for (const venue of venues) {
    const resolved = await resolveVenueId(client, venue.name);
    if (!resolved) {
      display.warn(
        `Venue "${venue.name}" not found in database — skipping`,
      );
      linkResults.push({
        name: venue.name,
        action: "not_found",
      });
      continue;
    }

    try {
      const body: Record<string, unknown> = {
        venue_id: resolved.id,
      };
      if (venue.is_primary !== undefined) body.is_primary = venue.is_primary;

      await client.post(`/festivals/${festivalId}/venues`, body);
      display.success(
        `  Linked venue "${resolved.name}" (ID: ${resolved.id})${venue.is_primary ? " (primary)" : ""}`,
      );
      linkResults.push({
        name: venue.name,
        action: "linked",
        venueId: resolved.id,
      });
    } catch (err) {
      // Treat 409 Conflict as "already linked" — not an error
      if (err instanceof APIError && err.status === 409) {
        display.info(
          `  Already linked: "${resolved.name}" (ID: ${resolved.id})`,
        );
        linkResults.push({
          name: venue.name,
          action: "already_linked",
          venueId: resolved.id,
        });
      } else {
        const message = err instanceof Error ? err.message : "Unknown error";
        display.error(
          `  Failed to link venue "${resolved.name}": ${message}`,
        );
        linkResults.push({
          name: venue.name,
          action: "error",
          venueId: resolved.id,
          error: message,
        });
      }
    }
  }

  return linkResults;
}

/**
 * Entry point called from CLI dispatcher.
 */
export async function runSubmitFestival(
  json: string | undefined,
  env: EnvironmentConfig,
  confirm: boolean,
): Promise<void> {
  let jsonStr = json;

  // Read from stdin if no JSON argument provided
  if (!jsonStr) {
    const chunks: string[] = [];
    const reader = process.stdin;
    reader.resume();
    reader.setEncoding("utf-8");

    jsonStr = await new Promise<string>((resolve, reject) => {
      reader.on("data", (chunk: string) => chunks.push(chunk));
      reader.on("end", () => resolve(chunks.join("")));
      reader.on("error", reject);
    });
  }

  if (!jsonStr?.trim()) {
    display.error(
      "No JSON provided. Pass as argument or pipe to stdin.",
    );
    process.exit(1);
  }

  let festivals: FestivalInput[];
  try {
    festivals = parseFestivalInput(jsonStr);
  } catch (err) {
    display.error(
      `Invalid JSON: ${err instanceof Error ? err.message : "parse error"}`,
    );
    process.exit(1);
  }

  if (festivals.length === 0) {
    display.warn("Empty array — nothing to submit.");
    return;
  }

  display.info(`Processing ${festivals.length} festival(s)...`);

  const results = await submitFestivals(festivals, env, confirm);

  const hasErrors = results.some((r) => r.action === "error");
  if (hasErrors) {
    process.exit(1);
  }
}
