import { APIClient, APIError } from "../lib/api";
import type { EnvironmentConfig } from "../lib/types";
import * as display from "../lib/display";
import { green, yellow, gray, dim, cyan } from "../lib/ansi";
import { similarityScore } from "../lib/duplicates";

/** Festival artist entry from the input JSON. */
export interface FestivalArtistInput {
  name: string;
  billing_tier?: string;
  position?: number;
  day_date?: string;
  stage?: string;
  set_time?: string;
}

/** Result of linking a single artist. */
export interface ArtistLinkResult {
  name: string;
  action: "linked" | "already_linked" | "not_found" | "error";
  artistId?: number;
  error?: string;
}

/** Result of unlinking a single artist. */
export interface ArtistUnlinkResult {
  name: string;
  action: "unlinked" | "not_found" | "error";
  artistId?: number;
  error?: string;
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
 * Returns the best match's ID and confidence score, or null if not found.
 */
export async function resolveArtistId(
  client: APIClient,
  name: string,
): Promise<{ id: number; name: string; confidence: number } | null> {
  try {
    const result = await client.get<{
      artists: Array<{ id: number; name: string; slug: string }>;
    }>("/artists/search", { q: name });

    if (!result.artists?.length) return null;

    // Look for exact match first (case-insensitive)
    const exact = result.artists.find(
      (a) => a.name.toLowerCase() === name.toLowerCase(),
    );
    if (exact) return { id: exact.id, name: exact.name, confidence: 1.0 };

    // Find best match by similarity score, require >= 0.7
    const scored = result.artists
      .map((a) => ({ ...a, score: similarityScore(name, a.name) }))
      .filter((a) => a.score >= 0.7)
      .sort((a, b) => b.score - a.score);

    if (scored.length > 0) {
      return { id: scored[0].id, name: scored[0].name, confidence: scored[0].score };
    }

    return null;
  } catch {
    return null;
  }
}

/**
 * Resolve a festival slug or numeric ID to a festival object via the API.
 * Returns { id, name, slug } or null if not found.
 */
export async function resolveFestival(
  client: APIClient,
  festivalRef: string,
): Promise<{ id: number; name: string; slug: string } | null> {
  try {
    const result = await client.get<{
      id: number;
      name: string;
      slug: string;
    }>(`/festivals/${festivalRef}`);

    if (result?.id) {
      return { id: result.id, name: result.name, slug: result.slug };
    }
    return null;
  } catch {
    return null;
  }
}

/**
 * Get the list of artists currently linked to a festival.
 */
export async function getFestivalArtists(
  client: APIClient,
  festivalId: number,
): Promise<Array<{ artist_id: number; artist_name: string }>> {
  try {
    const result = await client.get<{
      artists: Array<{ artist_id: number; artist_name: string }>;
    }>(`/festivals/${festivalId}/artists`);
    return result.artists || [];
  } catch {
    return [];
  }
}

/**
 * Link a single artist to a festival. Returns the link result.
 */
async function linkSingleArtist(
  client: APIClient,
  festivalId: number,
  artist: FestivalArtistInput,
  resolved: { id: number; name: string; confidence: number },
): Promise<ArtistLinkResult> {
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
    const confidenceStr = resolved.confidence < 1.0
      ? ` (${(resolved.confidence * 100).toFixed(0)}% match)`
      : "";
    display.success(
      `  Linked artist "${resolved.name}" (ID: ${resolved.id})${confidenceStr}${artist.billing_tier ? ` as ${artist.billing_tier}` : ""}`,
    );
    return {
      name: artist.name,
      action: "linked",
      artistId: resolved.id,
    };
  } catch (err) {
    if (err instanceof APIError && err.status === 409) {
      display.info(
        `  Already linked: "${resolved.name}" (ID: ${resolved.id})`,
      );
      return {
        name: artist.name,
        action: "already_linked",
        artistId: resolved.id,
      };
    }
    const message = err instanceof Error ? err.message : "Unknown error";
    display.error(
      `  Failed to link artist "${resolved.name}": ${message}`,
    );
    return {
      name: artist.name,
      action: "error",
      artistId: resolved.id,
      error: message,
    };
  }
}

/**
 * Parse JSON input for artist entries.
 * Accepts a JSON array of FestivalArtistInput objects.
 */
export function parseArtistInput(jsonStr: string): FestivalArtistInput[] {
  const parsed = JSON.parse(jsonStr);

  if (Array.isArray(parsed)) {
    return parsed;
  }

  // Single object — wrap in array
  return [parsed];
}

/**
 * Link artists to an existing festival.
 *
 * @param festivalRef - Festival slug or numeric ID
 * @param artists - Array of artist inputs to link
 * @param env - API environment config
 * @param options - Options: confirm (execute), replace (clear existing first)
 * @returns Array of link results
 */
export async function linkArtistsToFestival(
  festivalRef: string,
  artists: FestivalArtistInput[],
  env: EnvironmentConfig,
  options: { confirm: boolean; replace: boolean },
): Promise<ArtistLinkResult[]> {
  const client = new APIClient(env);
  const results: ArtistLinkResult[] = [];

  // --- Step 1: Resolve festival ---
  display.header("Resolving festival...");
  const festival = await resolveFestival(client, festivalRef);
  if (!festival) {
    display.error(`Festival "${festivalRef}" not found.`);
    return [];
  }
  display.success(`Found festival: "${festival.name}" (ID: ${festival.id}, slug: ${festival.slug})`);

  // --- Step 2: Validate billing tiers ---
  for (const artist of artists) {
    if (artist.billing_tier && !VALID_BILLING_TIERS.includes(artist.billing_tier)) {
      display.error(
        `Artist "${artist.name}" has invalid billing_tier "${artist.billing_tier}". ` +
          `Must be one of: ${VALID_BILLING_TIERS.join(", ")}`,
      );
      return [];
    }
  }

  // --- Step 3: Resolve artist names ---
  display.header("Resolving artists...");
  const resolutions: Array<{
    input: FestivalArtistInput;
    resolved: { id: number; name: string; confidence: number } | null;
  }> = [];

  for (const artist of artists) {
    const match = await resolveArtistId(client, artist.name);
    resolutions.push({ input: artist, resolved: match });
  }

  // --- Step 4: Get existing artists (for replace mode preview) ---
  let existingArtists: Array<{ artist_id: number; artist_name: string }> = [];
  if (options.replace) {
    existingArtists = await getFestivalArtists(client, festival.id);
  }

  // --- Step 5: Preview ---
  display.header("Preview");

  if (options.replace && existingArtists.length > 0) {
    display.info(
      `${yellow("REPLACE")} Will remove ${existingArtists.length} existing artist(s) first:`,
    );
    for (const ea of existingArtists) {
      display.info(`    ${dim("UNLINK")} "${ea.artist_name}" (ID: ${ea.artist_id})`);
    }
    display.info("");
  }

  let linkCount = 0;
  let notFoundCount = 0;

  for (const r of resolutions) {
    if (r.resolved) {
      const conf = `${(r.resolved.confidence * 100).toFixed(0)}%`;
      const matchLabel = r.resolved.confidence >= 1.0
        ? green(`EXACT -> "${r.resolved.name}" (ID: ${r.resolved.id})`)
        : yellow(`FUZZY ${conf} -> "${r.resolved.name}" (ID: ${r.resolved.id})`);
      const tierLabel = r.input.billing_tier ? ` [${r.input.billing_tier}]` : "";
      display.info(`  ${green("LINK")} ${r.input.name} ${matchLabel}${tierLabel}`);
      linkCount++;
    } else {
      display.warn(`  ${r.input.name} — not found in database`);
      notFoundCount++;
    }
  }

  display.info("");
  const parts: string[] = [];
  if (linkCount > 0) parts.push(green(`${linkCount} to link`));
  if (notFoundCount > 0) parts.push(yellow(`${notFoundCount} not found`));
  display.info(`Summary: ${parts.join(", ")}`);

  // --- Step 6: Execute (if --confirm) ---
  if (!options.confirm) {
    display.warn("Dry run. Pass --confirm to execute.");
    return [];
  }

  // Replace: remove all existing artists first
  if (options.replace && existingArtists.length > 0) {
    display.header("Removing existing artists...");
    for (const ea of existingArtists) {
      try {
        await client.delete(`/festivals/${festival.id}/artists/${ea.artist_id}`);
        display.success(`  Removed "${ea.artist_name}" (ID: ${ea.artist_id})`);
      } catch (err) {
        const message = err instanceof Error ? err.message : "Unknown error";
        display.error(`  Failed to remove "${ea.artist_name}": ${message}`);
      }
    }
  }

  // Link new artists
  display.header("Linking artists...");
  for (const r of resolutions) {
    if (!r.resolved) {
      display.warn(`  Artist "${r.input.name}" not found — skipping`);
      results.push({
        name: r.input.name,
        action: "not_found",
      });
      continue;
    }

    const linkResult = await linkSingleArtist(client, festival.id, r.input, r.resolved);
    results.push(linkResult);
  }

  // --- Step 7: Final report ---
  display.header("Results");
  const linked = results.filter((r) => r.action === "linked").length;
  const alreadyLinked = results.filter((r) => r.action === "already_linked").length;
  const notFound = results.filter((r) => r.action === "not_found").length;
  const errors = results.filter((r) => r.action === "error").length;

  const reportParts: string[] = [];
  if (linked > 0) reportParts.push(green(`${linked} linked`));
  if (alreadyLinked > 0) reportParts.push(gray(`${alreadyLinked} already linked`));
  if (notFound > 0) reportParts.push(yellow(`${notFound} not found`));
  if (errors > 0) reportParts.push(`${errors} error(s)`);
  display.info(`Summary: ${reportParts.join(", ")}`);

  return results;
}

/**
 * Unlink a single artist from a festival.
 *
 * @param festivalRef - Festival slug or numeric ID
 * @param artistRef - Artist name or numeric ID
 * @param env - API environment config
 * @param confirm - Whether to execute (default: dry-run)
 * @returns The unlink result
 */
export async function unlinkArtistFromFestival(
  festivalRef: string,
  artistRef: string,
  env: EnvironmentConfig,
  confirm: boolean,
): Promise<ArtistUnlinkResult> {
  const client = new APIClient(env);

  // --- Step 1: Resolve festival ---
  display.header("Resolving festival...");
  const festival = await resolveFestival(client, festivalRef);
  if (!festival) {
    display.error(`Festival "${festivalRef}" not found.`);
    return { name: artistRef, action: "not_found" };
  }
  display.success(`Found festival: "${festival.name}" (ID: ${festival.id}, slug: ${festival.slug})`);

  // --- Step 2: Resolve artist ---
  display.header("Resolving artist...");
  let artistId: number;
  let artistName: string;

  // Check if it's a numeric ID
  const numericId = parseInt(artistRef, 10);
  if (!isNaN(numericId) && String(numericId) === artistRef) {
    artistId = numericId;
    artistName = `ID:${numericId}`;
    display.info(`Using artist ID: ${numericId}`);
  } else {
    // Resolve by name
    const resolved = await resolveArtistId(client, artistRef);
    if (!resolved) {
      display.error(`Artist "${artistRef}" not found in database.`);
      return { name: artistRef, action: "not_found" };
    }
    artistId = resolved.id;
    artistName = resolved.name;
    const confidenceStr = resolved.confidence < 1.0
      ? ` (${(resolved.confidence * 100).toFixed(0)}% match)`
      : "";
    display.success(`Found artist: "${resolved.name}" (ID: ${resolved.id})${confidenceStr}`);
  }

  // --- Step 3: Preview ---
  display.header("Preview");
  display.info(`Will unlink "${artistName}" (ID: ${artistId}) from "${festival.name}"`);

  if (!confirm) {
    display.warn("Dry run. Pass --confirm to execute.");
    return { name: artistRef, action: "unlinked", artistId };
  }

  // --- Step 4: Execute ---
  display.header("Unlinking artist...");
  try {
    await client.delete(`/festivals/${festival.id}/artists/${artistId}`);
    display.success(`Unlinked "${artistName}" (ID: ${artistId}) from "${festival.name}"`);
    return { name: artistRef, action: "unlinked", artistId };
  } catch (err) {
    if (err instanceof APIError && err.status === 404) {
      display.warn(`Artist "${artistName}" is not linked to "${festival.name}"`);
      return { name: artistRef, action: "not_found", artistId };
    }
    const message = err instanceof Error ? err.message : "Unknown error";
    display.error(`Failed to unlink: ${message}`);
    return { name: artistRef, action: "error", artistId, error: message };
  }
}

/**
 * Entry point for `ph festival link-artists`.
 */
export async function runFestivalLinkArtists(
  festivalRef: string,
  json: string | undefined,
  env: EnvironmentConfig,
  options: { confirm?: boolean; replace?: boolean; file?: string },
): Promise<void> {
  let jsonStr = json;

  // Read from file if --file provided
  if (options.file) {
    try {
      const file = Bun.file(options.file);
      jsonStr = await file.text();
    } catch (err) {
      display.error(
        `Failed to read file "${options.file}": ${err instanceof Error ? err.message : "unknown error"}`,
      );
      process.exit(1);
    }
  }

  // Read from stdin if no JSON argument and no file
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
      "No JSON provided. Pass as argument, use --file, or pipe to stdin.",
    );
    process.exit(1);
  }

  let artists: FestivalArtistInput[];
  try {
    artists = parseArtistInput(jsonStr);
  } catch (err) {
    display.error(
      `Invalid JSON: ${err instanceof Error ? err.message : "parse error"}`,
    );
    process.exit(1);
  }

  if (artists.length === 0) {
    display.warn("Empty array — nothing to link.");
    return;
  }

  display.info(`Processing ${artists.length} artist(s)...`);

  const results = await linkArtistsToFestival(festivalRef, artists, env, {
    confirm: !!options.confirm,
    replace: !!options.replace,
  });

  const hasErrors = results.some((r) => r.action === "error");
  if (hasErrors) {
    process.exit(1);
  }
}

/**
 * Entry point for `ph festival unlink-artist`.
 */
export async function runFestivalUnlinkArtist(
  festivalRef: string,
  artistRef: string,
  env: EnvironmentConfig,
  confirm: boolean,
): Promise<void> {
  const result = await unlinkArtistFromFestival(
    festivalRef,
    artistRef,
    env,
    confirm,
  );

  if (result.action === "error") {
    process.exit(1);
  }
}
