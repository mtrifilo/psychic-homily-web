import type { APIClient } from "./api";
import type { EntityType } from "./types";

export type MatchResult = "exact" | "fuzzy" | "none";
export type ActionType = "create" | "update" | "skip";

export interface FieldComparison {
  field: string;
  existing: string;
  proposed: string;
  status: "new_info" | "already_set" | "unchanged";
}

export interface DuplicateCheckResult {
  action: ActionType;
  match: MatchResult;
  existingId?: number;
  existingSlug?: string;
  existingName?: string;
  fields: FieldComparison[];
  confidence: number;
}

export interface EntitySearchResult {
  id: number;
  name: string;
  slug: string;
  [key: string]: unknown;
}

/** Normalize a string for comparison: lowercase, trim, collapse whitespace, strip accents. */
export function normalizeForComparison(s: string): string {
  return s
    .normalize("NFD")
    .replace(/[\u0300-\u036f]/g, "") // strip combining diacritical marks
    .toLowerCase()
    .trim()
    .replace(/\s+/g, " ");
}

/** Simple similarity score between two strings (0-1). Uses normalized comparison. */
export function similarityScore(a: string, b: string): number {
  const na = normalizeForComparison(a);
  const nb = normalizeForComparison(b);

  if (na === nb) return 1.0;
  if (na.length === 0 || nb.length === 0) return 0;

  // One contains the other
  const longer = na.length >= nb.length ? na : nb;
  const shorter = na.length < nb.length ? na : nb;

  if (longer.includes(shorter)) {
    // Scale by how much of the longer string the shorter covers
    return 0.8 + 0.2 * (shorter.length / longer.length);
  }

  // Common prefix scoring
  let commonPrefix = 0;
  const minLen = Math.min(na.length, nb.length);
  for (let i = 0; i < minLen; i++) {
    if (na[i] === nb[i]) {
      commonPrefix++;
    } else {
      break;
    }
  }

  // Common suffix scoring
  let commonSuffix = 0;
  for (let i = 0; i < minLen - commonPrefix; i++) {
    if (na[na.length - 1 - i] === nb[nb.length - 1 - i]) {
      commonSuffix++;
    } else {
      break;
    }
  }

  const totalOverlap = commonPrefix + commonSuffix;
  const maxLen = Math.max(na.length, nb.length);

  return totalOverlap / maxLen;
}

/** Compare fields between an existing entity and a proposed entity. */
export function compareFields(
  existing: Record<string, unknown>,
  proposed: Record<string, unknown>,
  fieldNames: string[],
): FieldComparison[] {
  const comparisons: FieldComparison[] = [];

  for (const field of fieldNames) {
    const proposedVal = proposed[field];

    // Skip fields not present or empty in the proposed data
    if (proposedVal === undefined || proposedVal === null || proposedVal === "") {
      continue;
    }

    const existingVal = existing[field];
    const existingStr = existingVal != null ? String(existingVal) : "";
    const proposedStr = String(proposedVal);

    let status: FieldComparison["status"];

    if (!existingStr && proposedStr) {
      status = "new_info";
    } else if (existingStr === proposedStr) {
      status = "unchanged";
    } else {
      status = "already_set";
    }

    comparisons.push({
      field,
      existing: existingStr,
      proposed: proposedStr,
      status,
    });
  }

  return comparisons;
}

/** Classify the action based on match confidence and field comparisons. */
export function classifyAction(
  confidence: number,
  fields: FieldComparison[],
): ActionType {
  if (confidence < 0.6) return "create";

  const hasNewInfo = fields.some((f) => f.status === "new_info");
  return hasNewInfo ? "update" : "skip";
}

/** Classify the match result based on confidence. */
export function classifyMatch(confidence: number): MatchResult {
  if (confidence >= 1.0) return "exact";
  if (confidence >= 0.6) return "fuzzy";
  return "none";
}

// -- Entity-specific field lists for comparison --

const ARTIST_FIELDS = [
  "name", "city", "state", "country", "website", "bandcamp_url",
  "spotify_url", "instagram_url", "description",
];

const VENUE_FIELDS = [
  "name", "city", "state", "country", "address", "zip_code",
  "website", "capacity", "description",
];

const RELEASE_FIELDS = [
  "title", "release_type", "release_year", "release_date",
  "bandcamp_url", "spotify_url", "description",
];

const LABEL_FIELDS = [
  "name", "city", "state", "country", "website", "description",
  "bandcamp_url",
];

const FESTIVAL_FIELDS = [
  "name", "series_slug", "edition_year", "start_date", "end_date",
  "city", "state", "country", "website", "description",
];

/** Get the comparable fields for a given entity type. */
function getFieldsForType(entityType: EntityType): string[] {
  switch (entityType) {
    case "artist": return ARTIST_FIELDS;
    case "venue": return VENUE_FIELDS;
    case "release": return RELEASE_FIELDS;
    case "label": return LABEL_FIELDS;
    case "festival": return FESTIVAL_FIELDS;
    case "show": return [];
  }
}

/** Get the display name field from a proposed entity. */
function getEntityName(entityType: EntityType, proposed: Record<string, unknown>): string {
  if (entityType === "release") {
    return String(proposed.title || "");
  }
  return String(proposed.name || "");
}

// -- Entity-specific search functions --

async function searchArtists(
  client: APIClient,
  name: string,
): Promise<EntitySearchResult[]> {
  const result = await client.get<{
    artists: Array<{
      id: number;
      name: string;
      slug: string;
      city?: string;
      state?: string;
      country?: string;
      website?: string;
      bandcamp_url?: string;
      spotify_url?: string;
      instagram_url?: string;
      description?: string;
    }>;
  }>("/artists/search", { q: name });

  return (result.artists || []).map((a) => ({
    id: a.id,
    name: a.name,
    slug: a.slug,
    city: a.city || "",
    state: a.state || "",
    country: a.country || "",
    website: a.website || "",
    bandcamp_url: a.bandcamp_url || "",
    spotify_url: a.spotify_url || "",
    instagram_url: a.instagram_url || "",
    description: a.description || "",
  }));
}

async function searchVenues(
  client: APIClient,
  name: string,
): Promise<EntitySearchResult[]> {
  const result = await client.get<{
    venues: Array<{
      id: number;
      name: string;
      slug: string;
      city: string;
      state: string;
      country?: string;
      address?: string;
      zip_code?: string;
      website?: string;
      capacity?: number;
      description?: string;
    }>;
  }>("/venues/search", { q: name });

  return (result.venues || []).map((v) => ({
    id: v.id,
    name: v.name,
    slug: v.slug,
    city: v.city,
    state: v.state,
    country: v.country || "",
    address: v.address || "",
    zip_code: v.zip_code || "",
    website: v.website || "",
    capacity: v.capacity ? String(v.capacity) : "",
    description: v.description || "",
  }));
}

async function searchReleases(
  client: APIClient,
  title: string,
): Promise<EntitySearchResult[]> {
  // Client-side filter until backend search endpoint exists
  const result = await client.get<{
    releases: Array<{
      id: number;
      title: string;
      slug: string;
      release_type?: string;
      release_year?: number;
      release_date?: string;
      bandcamp_url?: string;
      spotify_url?: string;
      description?: string;
    }>;
  }>("/releases", {});

  const normalizedTitle = normalizeForComparison(title);
  return (result.releases || [])
    .filter((r) => normalizeForComparison(r.title).includes(normalizedTitle) ||
      normalizedTitle.includes(normalizeForComparison(r.title)))
    .map((r) => ({
      id: r.id,
      name: r.title,
      slug: r.slug,
      title: r.title,
      release_type: r.release_type || "",
      release_year: r.release_year ? String(r.release_year) : "",
      release_date: r.release_date || "",
      bandcamp_url: r.bandcamp_url || "",
      spotify_url: r.spotify_url || "",
      description: r.description || "",
    }));
}

async function searchLabels(
  client: APIClient,
  name: string,
): Promise<EntitySearchResult[]> {
  // Client-side filter until backend search endpoint exists
  const result = await client.get<{
    labels: Array<{
      id: number;
      name: string;
      slug: string;
      city?: string;
      state?: string;
      country?: string;
      website?: string;
      description?: string;
      bandcamp_url?: string;
    }>;
  }>("/labels", {});

  const normalizedName = normalizeForComparison(name);
  return (result.labels || [])
    .filter((l) => normalizeForComparison(l.name).includes(normalizedName) ||
      normalizedName.includes(normalizeForComparison(l.name)))
    .map((l) => ({
      id: l.id,
      name: l.name,
      slug: l.slug,
      city: l.city || "",
      state: l.state || "",
      country: l.country || "",
      website: l.website || "",
      description: l.description || "",
      bandcamp_url: l.bandcamp_url || "",
    }));
}

async function searchFestivals(
  client: APIClient,
  name: string,
): Promise<EntitySearchResult[]> {
  // Client-side filter until backend search endpoint exists
  const result = await client.get<{
    festivals: Array<{
      id: number;
      name: string;
      slug: string;
      series_slug?: string;
      edition_year?: number;
      start_date?: string;
      end_date?: string;
      city?: string;
      state?: string;
      country?: string;
      website?: string;
      description?: string;
    }>;
  }>("/festivals", {});

  const normalizedName = normalizeForComparison(name);
  return (result.festivals || [])
    .filter((f) => normalizeForComparison(f.name).includes(normalizedName) ||
      normalizedName.includes(normalizeForComparison(f.name)))
    .map((f) => ({
      id: f.id,
      name: f.name,
      slug: f.slug,
      series_slug: f.series_slug || "",
      edition_year: f.edition_year ? String(f.edition_year) : "",
      start_date: f.start_date || "",
      end_date: f.end_date || "",
      city: f.city || "",
      state: f.state || "",
      country: f.country || "",
      website: f.website || "",
      description: f.description || "",
    }));
}

/** Find the best matching entity from search results. */
function findBestMatch(
  entityType: EntityType,
  proposed: Record<string, unknown>,
  results: EntitySearchResult[],
): { entity: EntitySearchResult; confidence: number } | null {
  if (results.length === 0) return null;

  const proposedName = getEntityName(entityType, proposed);
  if (!proposedName) return null;

  let bestMatch: EntitySearchResult | null = null;
  let bestScore = 0;

  for (const result of results) {
    let score = similarityScore(proposedName, result.name);

    // Boost score for venue matches when city also matches
    if (entityType === "venue" && proposed.city && result.city) {
      const cityScore = similarityScore(
        String(proposed.city),
        String(result.city),
      );
      if (cityScore >= 0.8) {
        score = Math.min(1.0, score + 0.1);
      }
    }

    // Boost score for festival matches when edition year matches
    if (entityType === "festival" && proposed.edition_year && result.edition_year) {
      if (String(proposed.edition_year) === String(result.edition_year)) {
        score = Math.min(1.0, score + 0.1);
      }
    }

    if (score > bestScore) {
      bestScore = score;
      bestMatch = result;
    }
  }

  if (!bestMatch || bestScore < 0.6) return null;

  return { entity: bestMatch, confidence: bestScore };
}

/**
 * Check for duplicates by searching the API and comparing with the proposed entity.
 *
 * For each entity type:
 * 1. Extracts the name/title from the proposed entity
 * 2. Calls the appropriate search endpoint via the API client
 * 3. Fuzzy-matches results against the proposed entity
 * 4. If a match is found, compares all fields to classify the action
 */
export async function checkDuplicate(
  client: APIClient,
  entityType: EntityType,
  proposed: Record<string, unknown>,
): Promise<DuplicateCheckResult> {
  const noMatch: DuplicateCheckResult = {
    action: "create",
    match: "none",
    fields: [],
    confidence: 0,
  };

  // Shows use a simpler date+venue match — not implemented in v1
  if (entityType === "show") {
    return noMatch;
  }

  const name = getEntityName(entityType, proposed);
  if (!name) return noMatch;

  let results: EntitySearchResult[];
  try {
    switch (entityType) {
      case "artist":
        results = await searchArtists(client, name);
        break;
      case "venue":
        results = await searchVenues(client, name);
        break;
      case "release":
        results = await searchReleases(client, name);
        break;
      case "label":
        results = await searchLabels(client, name);
        break;
      case "festival":
        results = await searchFestivals(client, name);
        break;
      default:
        return noMatch;
    }
  } catch {
    // If search fails, default to create
    return noMatch;
  }

  const best = findBestMatch(entityType, proposed, results);
  if (!best) return noMatch;

  const fields = compareFields(
    best.entity as unknown as Record<string, unknown>,
    proposed,
    getFieldsForType(entityType),
  );

  const action = classifyAction(best.confidence, fields);
  const match = classifyMatch(best.confidence);

  return {
    action,
    match,
    existingId: best.entity.id,
    existingSlug: best.entity.slug,
    existingName: best.entity.name,
    fields,
    confidence: best.confidence,
  };
}

// -- Public search helpers for use by submit commands -------------------------

/** Search for artists by name. Returns matching results from the API. */
export async function searchArtistsByName(
  client: APIClient,
  name: string,
): Promise<EntitySearchResult[]> {
  return searchArtists(client, name);
}

/** Search for venues by name. Returns matching results from the API. */
export async function searchVenuesByName(
  client: APIClient,
  name: string,
): Promise<EntitySearchResult[]> {
  return searchVenues(client, name);
}
