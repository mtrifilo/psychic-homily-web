import { APIClient, APIError } from "../lib/api";
import type { EnvironmentConfig } from "../lib/types";
import * as display from "../lib/display";
import { green, yellow, gray, dim } from "../lib/ansi";
import { resolveArtistId } from "./festival";
import {
  curatedSetType,
  isUnroundtrippableSetType,
  isValidSetType,
  setTypeVocabularyCSV,
} from "../lib/setType";
import type { SetType } from "../lib/setType";

/** Show artist entry from the input JSON. */
export interface ShowArtistInput {
  name: string;
  is_headliner?: boolean;
  /** Curated bill role for this act. Overrides the command's --role flag. */
  set_type?: string;
}

/**
 * One entry of the bill this command PUTs back.
 *
 * `set_type` is OPTIONAL and its absence is load-bearing: an absent key is the
 * only way to say "nobody has curated this act's slot". Sending `performer`
 * instead would publish a default as a curated fact.
 *
 * `is_headliner` is always present, and that is equally load-bearing on the way
 * out: an act that states NEITHER field is read as the headliner when it is
 * first on a bill where no act names one. Both subcommands rewrite the whole
 * bill, so dropping the flag would let a preserved act be re-inferred into a
 * slot nobody gave it.
 */
interface ShowArtistUpdate {
  id: number;
  is_headliner: boolean;
  set_type?: SetType;
}

/** Result of adding a single artist. */
export interface ArtistAddResult {
  name: string;
  action: "added" | "already_linked" | "not_found" | "error";
  artistId?: number;
  error?: string;
}

/** Result of removing a single artist. */
export interface ArtistRemoveResult {
  name: string;
  action: "removed" | "not_found" | "error";
  artistId?: number;
  error?: string;
}

/** Artist as returned in the show response. */
interface ShowArtistResponse {
  id: number;
  name: string;
  slug: string;
  is_headliner?: boolean | null;
  set_type?: string | null;
}

/** Minimal show response shape for our needs. */
interface ShowResponse {
  id: number;
  title: string;
  slug: string;
  artists: ShowArtistResponse[];
}

/**
 * Fetch a show by numeric ID.
 * Returns the show object or null if not found.
 */
export async function getShow(
  client: APIClient,
  showId: string,
): Promise<ShowResponse | null> {
  try {
    const result = await client.get<ShowResponse>(`/shows/${showId}`);
    if (result?.id) {
      return result;
    }
    return null;
  } catch {
    return null;
  }
}

/**
 * Parse JSON input for show artist entries.
 * Accepts a JSON array of ShowArtistInput objects.
 */
export function parseShowArtistInput(jsonStr: string): ShowArtistInput[] {
  const parsed = JSON.parse(jsonStr);

  if (Array.isArray(parsed)) {
    return parsed;
  }

  // Single object — wrap in array
  return [parsed];
}

/**
 * The bill entry that re-states an act already on the show, unchanged.
 *
 * Both subcommands edit a bill by rewriting it whole, so every act they are not
 * touching has to be re-stated exactly or its curation is lost. That is the
 * defect this function exists to close: sending only `is_headliner` flattened
 * every `direct_support` / `opener` / `dj` on the bill back to the default.
 */
function preserveBillEntry(existing: ShowArtistResponse): ShowArtistUpdate {
  const entry: ShowArtistUpdate = {
    id: existing.id,
    is_headliner: existing.is_headliner ?? false,
  };
  const role = curatedSetType(existing.set_type);
  if (role) entry.set_type = role;
  return entry;
}

/**
 * The role stated for an act being ADDED, or undefined when none was.
 *
 * The act's own `set_type` outranks the command's `--role`, which is a default
 * for the acts that state nothing. Undefined leaves the key off the payload
 * entirely: the act's slot is then unknown, which is a fact the API records as
 * such rather than a role it has to invent.
 */
function requestedRole(
  artist: ShowArtistInput,
  fallbackRole?: string,
): string | undefined {
  const stated = artist.set_type ?? fallbackRole;
  return stated === undefined || stated.trim() === "" ? undefined : stated;
}

/**
 * Every stated role that the API would reject, as operator-facing messages.
 *
 * Checked before the first request rather than after: an invalid role names the
 * vocabulary here instead of arriving as a 422 body, and no partial bill is
 * written on the way to finding out.
 */
export function invalidRoleErrors(
  artists: ShowArtistInput[],
  fallbackRole?: string,
): string[] {
  const errors: string[] = [];
  if (
    fallbackRole !== undefined &&
    fallbackRole.trim() !== "" &&
    !isValidSetType(fallbackRole)
  ) {
    errors.push(
      `--role "${fallbackRole}" is not a valid bill role (allowed: ${setTypeVocabularyCSV()})`,
    );
  }
  for (const artist of artists) {
    const stated = artist.set_type;
    if (stated === undefined || stated.trim() === "") continue;
    if (isValidSetType(stated)) continue;
    errors.push(
      `"${artist.name}": set_type "${stated}" is not a valid bill role (allowed: ${setTypeVocabularyCSV()})`,
    );
  }
  return errors;
}

/**
 * Warns about acts whose stored role cannot be sent back, once per edit.
 *
 * The rewrite drops such a value (see curatedSetType), which is a real change to
 * a row the operator did not name, so it is never made silently.
 */
function warnUnroundtrippableRoles(artists: ShowArtistResponse[]): void {
  for (const artist of artists) {
    if (isUnroundtrippableSetType(artist.set_type)) {
      display.warn(
        `  "${artist.name}" (ID: ${artist.id}) has an unrecognized role "${artist.set_type}" — it will be reset to the default (allowed: ${setTypeVocabularyCSV()})`,
      );
    }
  }
}

/** How an act's stored role reads in the preview, or "" when it states none. */
function roleLabel(setType: string | null | undefined): string {
  const role = curatedSetType(setType);
  return role ? ` ${dim(`[${role}]`)}` : "";
}

/**
 * Add artists to an existing show.
 *
 * Strategy: GET the show to get current artists, merge new ones in,
 * PUT back the full artist list via the show update endpoint.
 *
 * Acts already on the bill are re-stated exactly, curated role included, so an
 * edit here never rewrites a slot the operator did not name.
 *
 * @param showId - Numeric show ID
 * @param artists - Array of artist inputs to add
 * @param env - API environment config
 * @param confirm - Whether to execute (default: dry-run)
 * @param role - Bill role for added acts that state none of their own
 * @returns Array of add results
 */
export async function addArtistsToShow(
  showId: string,
  artists: ShowArtistInput[],
  env: EnvironmentConfig,
  confirm: boolean,
  role?: string,
): Promise<ArtistAddResult[]> {
  const client = new APIClient(env);
  const results: ArtistAddResult[] = [];

  // --- Step 0: Refuse invalid roles before any request goes out ---
  const roleErrors = invalidRoleErrors(artists, role);
  if (roleErrors.length > 0) {
    for (const message of roleErrors) {
      display.error(message);
    }
    return artists.map((artist) => ({
      name: artist.name,
      action: "error" as const,
      error: roleErrors[0],
    }));
  }

  // --- Step 1: Fetch the show ---
  display.header("Resolving show...");
  const show = await getShow(client, showId);
  if (!show) {
    display.error(`Show "${showId}" not found.`);
    return [];
  }
  display.success(
    `Found show: "${show.title || "(untitled)"}" (ID: ${show.id}, slug: ${show.slug})`,
  );

  // --- Step 2: Resolve artist names ---
  display.header("Resolving artists...");
  const resolutions: Array<{
    input: ShowArtistInput;
    resolved: { id: number; name: string; confidence: number } | null;
  }> = [];

  for (const artist of artists) {
    const match = await resolveArtistId(client, artist.name);
    resolutions.push({ input: artist, resolved: match });
  }

  // --- Step 3: Check which are already linked ---
  const existingArtistIds = new Set(show.artists.map((a) => a.id));

  // --- Step 4: Preview ---
  display.header("Preview");

  display.info(
    `Show currently has ${show.artists.length} artist(s): ${show.artists.map((a) => `"${a.name}"`).join(", ") || "(none)"}`,
  );
  for (const existing of show.artists) {
    const label = roleLabel(existing.set_type);
    if (label) {
      display.info(`  ${gray("KEEP")} "${existing.name}"${label}`);
    }
  }
  warnUnroundtrippableRoles(show.artists);
  display.info("");

  let addCount = 0;
  let alreadyLinkedCount = 0;
  let notFoundCount = 0;

  for (const r of resolutions) {
    if (r.resolved) {
      if (existingArtistIds.has(r.resolved.id)) {
        display.info(
          `  ${gray("SKIP")} "${r.input.name}" -> "${r.resolved.name}" (ID: ${r.resolved.id}) — already linked`,
        );
        alreadyLinkedCount++;
      } else {
        const conf = `${(r.resolved.confidence * 100).toFixed(0)}%`;
        const matchLabel =
          r.resolved.confidence >= 1.0
            ? green(
                `EXACT -> "${r.resolved.name}" (ID: ${r.resolved.id})`,
              )
            : yellow(
                `FUZZY ${conf} -> "${r.resolved.name}" (ID: ${r.resolved.id})`,
              );
        const stated = requestedRole(r.input, role);
        const roleTag = stated
          ? ` [${stated}]`
          : r.input.is_headliner
            ? " [headliner]"
            : "";
        display.info(`  ${green("ADD")} ${r.input.name} ${matchLabel}${roleTag}`);
        addCount++;
      }
    } else {
      display.warn(`  ${r.input.name} — not found in database`);
      notFoundCount++;
    }
  }

  display.info("");
  const parts: string[] = [];
  if (addCount > 0) parts.push(green(`${addCount} to add`));
  if (alreadyLinkedCount > 0) parts.push(gray(`${alreadyLinkedCount} already linked`));
  if (notFoundCount > 0) parts.push(yellow(`${notFoundCount} not found`));
  display.info(`Summary: ${parts.join(", ")}`);

  // --- Step 5: Execute (if --confirm) ---
  if (!confirm) {
    display.warn("Dry run. Pass --confirm to execute.");
    return [];
  }

  if (addCount === 0) {
    display.info("Nothing to add.");
    // Still report already-linked and not-found
    for (const r of resolutions) {
      if (r.resolved && existingArtistIds.has(r.resolved.id)) {
        results.push({
          name: r.input.name,
          action: "already_linked",
          artistId: r.resolved.id,
        });
      } else if (!r.resolved) {
        results.push({
          name: r.input.name,
          action: "not_found",
        });
      }
    }
    return results;
  }

  // Build the merged artist list: keep existing + add new
  const updatedArtists: ShowArtistUpdate[] = show.artists.map(preserveBillEntry);

  // Add new artists
  for (const r of resolutions) {
    if (!r.resolved) {
      results.push({ name: r.input.name, action: "not_found" });
      continue;
    }

    if (existingArtistIds.has(r.resolved.id)) {
      results.push({
        name: r.input.name,
        action: "already_linked",
        artistId: r.resolved.id,
      });
      continue;
    }

    // A stated role is authoritative and is_headliner is DERIVED from it, so
    // the two halves of the payload cannot contradict each other.
    const stated = requestedRole(r.input, role);
    const entry: ShowArtistUpdate = stated
      ? { id: r.resolved.id, is_headliner: stated === "headliner", set_type: stated as SetType }
      : { id: r.resolved.id, is_headliner: r.input.is_headliner ?? false };
    updatedArtists.push(entry);
  }

  // PUT the updated artist list
  display.header("Updating show artists...");
  try {
    await client.put(`/shows/${show.id}`, {
      artists: updatedArtists,
    });

    // Mark all new artists as added
    for (const r of resolutions) {
      if (r.resolved && !existingArtistIds.has(r.resolved.id)) {
        const stated = requestedRole(r.input, role);
        const roleStr = stated
          ? ` as ${stated}`
          : r.input.is_headliner
            ? " as headliner"
            : "";
        display.success(
          `  Added "${r.resolved.name}" (ID: ${r.resolved.id})${roleStr}`,
        );
        results.push({
          name: r.input.name,
          action: "added",
          artistId: r.resolved.id,
        });
      }
    }
  } catch (err) {
    const message = err instanceof Error ? err.message : "Unknown error";
    display.error(`Failed to update show: ${message}`);

    // Mark all pending adds as errors
    for (const r of resolutions) {
      if (r.resolved && !existingArtistIds.has(r.resolved.id)) {
        results.push({
          name: r.input.name,
          action: "error",
          artistId: r.resolved.id,
          error: message,
        });
      }
    }
  }

  // --- Step 6: Final report ---
  display.header("Results");
  const added = results.filter((r) => r.action === "added").length;
  const alreadyLinked = results.filter((r) => r.action === "already_linked").length;
  const notFound = results.filter((r) => r.action === "not_found").length;
  const errors = results.filter((r) => r.action === "error").length;

  const reportParts: string[] = [];
  if (added > 0) reportParts.push(green(`${added} added`));
  if (alreadyLinked > 0) reportParts.push(gray(`${alreadyLinked} already linked`));
  if (notFound > 0) reportParts.push(yellow(`${notFound} not found`));
  if (errors > 0) reportParts.push(`${errors} error(s)`);
  display.info(`Summary: ${reportParts.join(", ")}`);

  return results;
}

/**
 * Remove an artist from an existing show.
 *
 * Strategy: GET the show to get current artists, filter out the target,
 * PUT back the remaining artist list via the show update endpoint.
 *
 * @param showId - Numeric show ID
 * @param artistRef - Artist name or numeric ID
 * @param env - API environment config
 * @param confirm - Whether to execute (default: dry-run)
 * @returns The remove result
 */
export async function removeArtistFromShow(
  showId: string,
  artistRef: string,
  env: EnvironmentConfig,
  confirm: boolean,
): Promise<ArtistRemoveResult> {
  const client = new APIClient(env);

  // --- Step 1: Fetch the show ---
  display.header("Resolving show...");
  const show = await getShow(client, showId);
  if (!show) {
    display.error(`Show "${showId}" not found.`);
    return { name: artistRef, action: "not_found" };
  }
  display.success(
    `Found show: "${show.title || "(untitled)"}" (ID: ${show.id}, slug: ${show.slug})`,
  );

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
    const confidenceStr =
      resolved.confidence < 1.0
        ? ` (${(resolved.confidence * 100).toFixed(0)}% match)`
        : "";
    display.success(
      `Found artist: "${resolved.name}" (ID: ${resolved.id})${confidenceStr}`,
    );
  }

  // --- Step 3: Check if artist is on this show ---
  const existingArtist = show.artists.find((a) => a.id === artistId);
  if (!existingArtist) {
    display.warn(
      `Artist "${artistName}" (ID: ${artistId}) is not linked to this show.`,
    );
    display.info(
      `Current artists: ${show.artists.map((a) => `"${a.name}" (ID: ${a.id})`).join(", ") || "(none)"}`,
    );
    return { name: artistRef, action: "not_found", artistId };
  }

  // Use the real name from the show if we only had an ID
  if (artistName.startsWith("ID:")) {
    artistName = existingArtist.name;
  }

  // --- Step 4: Preview ---
  display.header("Preview");
  display.info(
    `Will remove "${artistName}" (ID: ${artistId}) from "${show.title || "(untitled)"}"`,
  );
  display.info(
    `Show will have ${show.artists.length - 1} artist(s) after removal.`,
  );
  for (const remaining of show.artists) {
    if (remaining.id === artistId) continue;
    const label = roleLabel(remaining.set_type);
    if (label) {
      display.info(`  ${gray("KEEP")} "${remaining.name}"${label}`);
    }
  }
  warnUnroundtrippableRoles(show.artists.filter((a) => a.id !== artistId));

  if (!confirm) {
    display.warn("Dry run. Pass --confirm to execute.");
    return { name: artistRef, action: "removed", artistId };
  }

  // --- Step 5: Execute ---
  display.header("Removing artist...");

  // Build the artist list without the target artist
  const remainingArtists = show.artists
    .filter((a) => a.id !== artistId)
    .map(preserveBillEntry);

  try {
    await client.put(`/shows/${show.id}`, {
      artists: remainingArtists,
    });
    display.success(
      `Removed "${artistName}" (ID: ${artistId}) from "${show.title || "(untitled)"}"`,
    );
    return { name: artistRef, action: "removed", artistId };
  } catch (err) {
    const message = err instanceof Error ? err.message : "Unknown error";
    display.error(`Failed to update show: ${message}`);
    return { name: artistRef, action: "error", artistId, error: message };
  }
}

/**
 * Entry point for `ph show add-artist`.
 */
export async function runShowAddArtist(
  showId: string,
  json: string | undefined,
  env: EnvironmentConfig,
  options: { confirm?: boolean; file?: string; role?: string },
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

  let artists: ShowArtistInput[];
  try {
    artists = parseShowArtistInput(jsonStr);
  } catch (err) {
    display.error(
      `Invalid JSON: ${err instanceof Error ? err.message : "parse error"}`,
    );
    process.exit(1);
  }

  if (artists.length === 0) {
    display.warn("Empty array — nothing to add.");
    return;
  }

  display.info(`Processing ${artists.length} artist(s)...`);

  const results = await addArtistsToShow(
    showId,
    artists,
    env,
    !!options.confirm,
    options.role,
  );

  // `process.exitCode`, not `process.exit()`: bill edits written before a
  // failure still need their ISR revalidation flushed at end of run (PSY-1691).
  const hasErrors = results.some((r) => r.action === "error");
  if (hasErrors) {
    process.exitCode = 1;
  }
}

/**
 * Entry point for `ph show remove-artist`.
 */
export async function runShowRemoveArtist(
  showId: string,
  artistRef: string,
  env: EnvironmentConfig,
  confirm: boolean,
): Promise<void> {
  const result = await removeArtistFromShow(showId, artistRef, env, confirm);

  // `process.exitCode`, not `process.exit()`: see runShowAddArtist (PSY-1691).
  if (result.action === "error") {
    process.exitCode = 1;
  }
}
