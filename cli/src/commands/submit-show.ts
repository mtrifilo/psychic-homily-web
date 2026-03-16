import { APIClient } from "../lib/api";
import type { EnvironmentConfig } from "../lib/types";
import { validateShow } from "../lib/schemas";
import { searchArtistsByName, searchVenuesByName } from "../lib/duplicates";
import { TagResolver, formatTagsPreview, formatFuzzyWarning } from "../lib/tags";
import type { TagInput, ResolvedTag } from "../lib/tags";
import * as display from "../lib/display";
import { green, yellow, dim, gray } from "../lib/ansi";

/** Normalize a date string to ISO 8601. Adds T20:00:00Z if only YYYY-MM-DD. */
function normalizeDate(date: string): string {
  if (/^\d{4}-\d{2}-\d{2}$/.test(date)) {
    return `${date}T20:00:00Z`;
  }
  return date;
}

// -- Types -------------------------------------------------------------------

interface ShowArtistInput {
  name: string;
  is_headliner?: boolean;
}

interface ShowVenueInput {
  name: string;
  city?: string;
  state?: string;
  address?: string;
}

interface ShowInput {
  event_date: string;
  title?: string;
  city: string;
  state: string;
  price?: number;
  age_requirement?: string;
  description?: string;
  artists: ShowArtistInput[];
  venues: ShowVenueInput[];
  tags?: TagInput[];
}

interface ResolvedArtist {
  id?: number;
  name: string;
  is_headliner?: boolean;
  status: "existing" | "new";
}

interface ResolvedVenue {
  id?: number;
  name: string;
  city?: string;
  state?: string;
  address?: string;
  status: "existing" | "new";
}

export interface ShowPlan {
  input: ShowInput;
  artists: ResolvedArtist[];
  venues: ResolvedVenue[];
  valid: boolean;
  errors: string[];
}

export interface SubmitShowsResult {
  plans: ShowPlan[];
  created: number;
  failed: number;
  skipped: number;
}

// -- Core logic (exported for testing) ---------------------------------------

/** Parse JSON input into an array of show objects. */
export function parseShowInput(jsonStr: string): ShowInput[] {
  let parsed: unknown;
  try {
    parsed = JSON.parse(jsonStr);
  } catch {
    throw new Error("Invalid JSON input");
  }

  if (Array.isArray(parsed)) {
    return parsed as ShowInput[];
  }
  return [parsed as ShowInput];
}

/** Resolve artists against the API, returning resolved entries with IDs when found. */
export async function resolveArtists(
  client: APIClient,
  artists: ShowArtistInput[],
): Promise<ResolvedArtist[]> {
  const resolved: ResolvedArtist[] = [];

  for (const artist of artists) {
    try {
      const results = await searchArtistsByName(client, artist.name);
      if (results.length > 0) {
        // Use the best match (first result from search)
        resolved.push({
          id: results[0].id,
          name: results[0].name,
          is_headliner: artist.is_headliner,
          status: "existing",
        });
      } else {
        resolved.push({
          name: artist.name,
          is_headliner: artist.is_headliner,
          status: "new",
        });
      }
    } catch {
      // If search fails, treat as new
      resolved.push({
        name: artist.name,
        is_headliner: artist.is_headliner,
        status: "new",
      });
    }
  }

  return resolved;
}

/** Resolve venues against the API, returning resolved entries with IDs when found. */
export async function resolveVenues(
  client: APIClient,
  venues: ShowVenueInput[],
): Promise<ResolvedVenue[]> {
  const resolved: ResolvedVenue[] = [];

  for (const venue of venues) {
    try {
      const results = await searchVenuesByName(client, venue.name);
      if (results.length > 0) {
        resolved.push({
          id: results[0].id,
          name: results[0].name,
          city: venue.city,
          state: venue.state,
          address: venue.address,
          status: "existing",
        });
      } else {
        resolved.push({
          name: venue.name,
          city: venue.city,
          state: venue.state,
          address: venue.address,
          status: "new",
        });
      }
    } catch {
      resolved.push({
        name: venue.name,
        city: venue.city,
        state: venue.state,
        address: venue.address,
        status: "new",
      });
    }
  }

  return resolved;
}

/** Build the API request body for creating a show. */
export function buildShowPayload(plan: ShowPlan): Record<string, unknown> {
  const payload: Record<string, unknown> = {
    event_date: normalizeDate(plan.input.event_date),
    city: plan.input.city,
    state: plan.input.state,
    artists: plan.artists.map((a) => {
      const artist: Record<string, unknown> = {};
      if (a.id) artist.id = a.id;
      if (!a.id) artist.name = a.name;
      if (a.is_headliner !== undefined) artist.is_headliner = a.is_headliner;
      return artist;
    }),
    venues: plan.venues.map((v) => {
      const venue: Record<string, unknown> = {};
      if (v.id) venue.id = v.id;
      if (!v.id) {
        venue.name = v.name;
        if (v.city) venue.city = v.city;
        if (v.state) venue.state = v.state;
        if (v.address) venue.address = v.address;
      }
      return venue;
    }),
  };

  if (plan.input.title) payload.title = plan.input.title;
  if (plan.input.price !== undefined) payload.price = plan.input.price;
  if (plan.input.age_requirement) payload.age_requirement = plan.input.age_requirement;
  if (plan.input.description) payload.description = plan.input.description;

  return payload;
}

/** Main entry point: validate, resolve, preview, and optionally submit shows. */
export async function submitShows(
  client: APIClient,
  jsonStr: string,
  confirm: boolean,
): Promise<SubmitShowsResult> {
  // 1. Parse input
  const shows = parseShowInput(jsonStr);

  // 2. Validate and resolve each show
  const plans: ShowPlan[] = [];

  for (const show of shows) {
    const validation = validateShow(show);
    if (!validation.valid) {
      plans.push({
        input: show,
        artists: [],
        venues: [],
        valid: false,
        errors: validation.errors.map((e) => `${e.field}: ${e.message}`),
      });
      continue;
    }

    // Resolve artists and venues against API
    const artists = await resolveArtists(client, show.artists);
    const venues = await resolveVenues(client, show.venues);

    plans.push({
      input: show,
      artists,
      venues,
      valid: true,
      errors: [],
    });
  }

  // 2b. Resolve tags for all shows
  const tagResolver = new TagResolver(client);
  const resolvedTags: ResolvedTag[][] = [];
  for (const plan of plans) {
    const tags = TagResolver.parseTags(plan.input.tags as TagInput[] | undefined);
    if (tags.length > 0 && plan.valid) {
      resolvedTags.push(await tagResolver.resolveAll(tags));
    } else {
      resolvedTags.push([]);
    }
  }

  // 3. Display preview
  displayPreview(plans, resolvedTags);

  // 4. Summary
  const validPlans = plans.filter((p) => p.valid);
  const invalidCount = plans.length - validPlans.length;

  if (invalidCount > 0) {
    display.warn(`${invalidCount} show${invalidCount !== 1 ? "s" : ""} failed validation and will be skipped.`);
  }

  if (validPlans.length === 0) {
    display.error("No valid shows to submit.");
    return { plans, created: 0, failed: 0, skipped: plans.length };
  }

  // 5. Submit if confirmed
  if (!confirm) {
    display.info(`Dry run: ${validPlans.length} show${validPlans.length !== 1 ? "s" : ""} would be created. Use --confirm to submit.`);
    return { plans, created: 0, failed: 0, skipped: validPlans.length };
  }

  let created = 0;
  let failed = 0;

  for (const plan of validPlans) {
    const payload = buildShowPayload(plan);
    try {
      const result = await client.post<{ id: number; slug?: string }>("/shows", payload);
      created++;
      const label = plan.input.title || `${plan.input.event_date} show`;
      display.success(`Created: ${label} (ID: ${result.id})`);
      // Apply tags if any
      const parsedTags = TagResolver.parseTags(plan.input.tags as TagInput[] | undefined);
      if (result.id && parsedTags.length > 0) {
        const tagResult = await tagResolver.applyToEntity("show", result.id, parsedTags);
        if (tagResult.applied > 0) {
          display.info(`  Applied ${tagResult.applied} tag(s)`);
        }
      }
    } catch (err) {
      failed++;
      const label = plan.input.title || `${plan.input.event_date} show`;
      const message = err instanceof Error ? err.message : "Unknown error";
      display.error(`Failed to create ${label}: ${message}`);
    }
  }

  display.summary(created, 0, failed + invalidCount);

  return { plans, created, failed, skipped: invalidCount };
}

// -- Display helpers ---------------------------------------------------------

function displayPreview(plans: ShowPlan[], resolvedTags?: ResolvedTag[][]): void {
  for (let i = 0; i < plans.length; i++) {
    const plan = plans[i];
    const idx = plans.length > 1 ? ` [${i + 1}/${plans.length}]` : "";

    if (!plan.valid) {
      display.header(`Show${idx}: INVALID`);
      for (const err of plan.errors) {
        display.error(err);
      }
      continue;
    }

    const label = plan.input.title || `${plan.input.event_date} in ${plan.input.city}, ${plan.input.state}`;
    display.header(`Show${idx}: ${label}`);
    display.kv("Date", plan.input.event_date);
    display.kv("Location", `${plan.input.city}, ${plan.input.state}`);

    if (plan.input.price !== undefined) {
      display.kv("Price", `$${plan.input.price}`);
    }
    if (plan.input.age_requirement) {
      display.kv("Ages", plan.input.age_requirement);
    }

    // Artists
    process.stderr.write(`\n  ${gray("Artists:")}\n`);
    for (const artist of plan.artists) {
      const tag = artist.status === "existing"
        ? green(`EXISTING (ID: ${artist.id})`)
        : yellow("NEW");
      const headliner = artist.is_headliner ? dim(" [headliner]") : "";
      process.stderr.write(`    ${artist.name} ${tag}${headliner}\n`);
    }

    // Venues
    process.stderr.write(`\n  ${gray("Venues:")}\n`);
    for (const venue of plan.venues) {
      const tag = venue.status === "existing"
        ? green(`EXISTING (ID: ${venue.id})`)
        : yellow("NEW");
      process.stderr.write(`    ${venue.name} ${tag}\n`);
    }

    // Tags
    if (resolvedTags && resolvedTags[i].length > 0) {
      display.kv("tags", formatTagsPreview(resolvedTags[i]));
      for (const tag of resolvedTags[i]) {
        const warning = formatFuzzyWarning(tag);
        if (warning) display.warn(warning);
      }
    }
  }
}

// -- CLI runner (called from cli.ts) -----------------------------------------

export async function runSubmitShow(
  json: string | undefined,
  env: EnvironmentConfig,
  confirm: boolean,
): Promise<void> {
  let jsonStr: string;

  if (json) {
    jsonStr = json;
  } else {
    // Read from stdin
    jsonStr = await readStdin();
    if (!jsonStr.trim()) {
      display.error("No JSON input provided. Pass JSON as argument or pipe via stdin.");
      process.exit(1);
    }
  }

  const client = new APIClient(env);
  const result = await submitShows(client, jsonStr, confirm);

  if (result.failed > 0 || (result.created === 0 && confirm)) {
    process.exit(1);
  }
}

async function readStdin(): Promise<string> {
  // Check if stdin has data (piped input)
  if (process.stdin.isTTY) {
    return "";
  }

  const chunks: Uint8Array[] = [];
  for await (const chunk of process.stdin) {
    chunks.push(new Uint8Array(chunk));
  }
  return Buffer.concat(chunks).toString("utf-8");
}
