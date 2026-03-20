import type { APIClient } from "./api";
import * as display from "./display";

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
 */
export async function linkReleaseToLabel(
  client: APIClient,
  labelId: number,
  releaseId: number,
  catalogNumber?: string,
): Promise<boolean> {
  try {
    const body: Record<string, unknown> = { release_id: releaseId };
    if (catalogNumber) {
      body.catalog_number = catalogNumber;
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
 * Resolve a label name and link it to an artist.
 * Returns the label ID if successful, or null.
 */
export async function resolveAndLinkArtistLabel(
  client: APIClient,
  labelName: string,
  artistId: number,
): Promise<number | null> {
  const label = await resolveLabelByName(client, labelName);
  if (!label) {
    display.warn(`Label "${labelName}" not found — skipping artist-label link`);
    return null;
  }

  const linked = await linkArtistToLabel(client, label.id, artistId);
  if (linked) {
    display.info(`  Linked artist ${artistId} to label "${label.name}" (ID: ${label.id})`);
    return label.id;
  }
  return null;
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
): Promise<number | null> {
  const label = await resolveLabelByName(client, labelName);
  if (!label) {
    display.warn(`Label "${labelName}" not found — skipping release-label link`);
    return null;
  }

  const linked = await linkReleaseToLabel(client, label.id, releaseId, catalogNumber);
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
