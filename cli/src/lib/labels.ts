import type { APIClient } from "./api";
import * as display from "./display";

/**
 * Distributor placeholder shape: five or more digits, then "Records",
 * optionally a two-letter suffix ("3245973 Records DK"). Real labels that
 * start with digits ("4AD", "12XU", "2000 Records") have fewer digits or no
 * "Records" word, so they do not match.
 */
const PLACEHOLDER_LABEL_NAME = /^\d{5,}\s+records(?:\s+[a-z]{2})?$/i;

/** True when `name` has the distributor placeholder shape; surrounding whitespace is ignored. */
export function isPlaceholderLabelName(name: string): boolean {
  return PLACEHOLDER_LABEL_NAME.test(name.trim());
}

/**
 * Dry-run warning for a placeholder-shaped label name, or null for any other
 * name. A warning only: the name is still submitted if the run is confirmed.
 */
export function placeholderLabelWarning(name: unknown): string | null {
  if (typeof name !== "string" || !isPlaceholderLabelName(name)) return null;
  return `Label "${name}" looks like a distributor placeholder, not a real label: five or more digits, then "Records"`;
}

/** Print the placeholder warning for `name` when it has the placeholder shape. */
export function warnIfPlaceholderLabel(name: unknown): void {
  const warning = placeholderLabelWarning(name);
  if (warning) display.warn(warning);
}

/**
 * Resolve a label name to its ID via the search API.
 * Returns the label ID if an exact (case-insensitive) match is found, or null.
 */
export async function resolveLabelByName(
  client: APIClient,
  name: string,
): Promise<{ id: number; name: string } | null> {
  try {
    const result = await client.get<{
      labels: Array<{ id: number; name: string; slug: string }>;
    }>("/labels/search", { q: name });

    if (!result.labels?.length) return null;

    // Exact case-insensitive match
    const normalizedName = name.toLowerCase().trim();
    const exact = result.labels.find(
      (l) => l.name.toLowerCase().trim() === normalizedName,
    );
    if (exact) return { id: exact.id, name: exact.name };

    // No fuzzy fallback for labels — exact match only to avoid wrong associations
    return null;
  } catch {
    return null;
  }
}

/**
 * Link an artist to a label via the admin API.
 * Idempotent — succeeds silently if the link already exists.
 */
export async function linkArtistToLabel(
  client: APIClient,
  labelId: number,
  artistId: number,
): Promise<boolean> {
  try {
    await client.post(`/admin/labels/${labelId}/artists`, {
      artist_id: artistId,
    });
    return true;
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    display.warn(`Failed to link artist ${artistId} to label ${labelId}: ${message}`);
    return false;
  }
}

/**
 * Link a release to a label via the admin API.
 * Idempotent — succeeds silently if the link already exists.
 *
 * `catalog_number` is write-once on the backend: by default an existing link's
 * number is preserved. Pass `overwrite=true` to correct an already-stored
 * number (PSY-1194). Overwrite only makes sense alongside a catalogNumber, so
 * the flag is sent only when a number is also present.
 */
export async function linkReleaseToLabel(
  client: APIClient,
  labelId: number,
  releaseId: number,
  catalogNumber?: string,
  overwrite?: boolean,
): Promise<boolean> {
  try {
    const body: Record<string, unknown> = { release_id: releaseId };
    if (catalogNumber) {
      body.catalog_number = catalogNumber;
      if (overwrite) {
        body.overwrite_catalog_number = true;
      }
    }
    await client.post(`/admin/labels/${labelId}/releases`, body);
    return true;
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    display.warn(`Failed to link release ${releaseId} to label ${labelId}: ${message}`);
    return false;
  }
}

/**
 * Resolve a label name and link it to a release (and optionally its artists).
 * Returns the label ID if successful, or null.
 */
export async function resolveAndLinkReleaseLabel(
  client: APIClient,
  labelName: string,
  releaseId: number,
  artistIds?: number[],
  catalogNumber?: string,
  overwrite?: boolean,
): Promise<number | null> {
  const label = await resolveLabelByName(client, labelName);
  if (!label) {
    display.warn(`Label "${labelName}" not found — skipping release-label link`);
    return null;
  }

  const linked = await linkReleaseToLabel(client, label.id, releaseId, catalogNumber, overwrite);
  if (linked) {
    display.info(`  Linked release ${releaseId} to label "${label.name}" (ID: ${label.id})`);
  }

  // Also link each artist to the label
  if (artistIds?.length) {
    for (const artistId of artistIds) {
      await linkArtistToLabel(client, label.id, artistId);
    }
    display.info(`  Linked ${artistIds.length} artist(s) to label "${label.name}"`);
  }

  return linked ? label.id : null;
}
