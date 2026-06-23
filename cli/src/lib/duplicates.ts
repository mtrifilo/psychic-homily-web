import type { APIClient } from "./api";
import type { EntityType } from "./types";
import { getTimezoneForState, localTimeToUTC } from "./timezone";

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

/**
 * Check if the shorter string is embedded inside a longer word in the other string,
 * creating a false positive risk.
 *
 * E.g., "dram" inside "dream" — "dram" is a substring of the word "dream", which is
 * a completely different word. Returns true (trap).
 *
 * But "the shin" inside "the shins" — the only difference is a trailing 's' (plural).
 * We allow this because it's a minor suffix variation. Returns false (not a trap).
 *
 * Heuristic: if the non-matching portion on either side is just 1 character, it's likely
 * a typo, plural, or minor variant — not a trap. If 2+ extra characters, it's a trap.
 */
/**
 * Check if two individual words are similar enough to be variants of each other
 * (e.g., plural "shins"/"shin", minor suffix like "mannequin"/"mannequins").
 * Returns false for clearly different words like "keys"/"lips" or "pussy"/"s".
 */
function areSimilarWords(a: string, b: string): boolean {
  if (a === b) return true;
  const [shorter, longer] = a.length <= b.length ? [a, b] : [b, a];
  // Allow singular/plural: "shin"/"shins", "key"/"keys", "box"/"boxes"
  if (longer === shorter + "s" || longer === shorter + "es") return true;
  // Check character prefix overlap — 75%+ means likely a variant
  let prefix = 0;
  for (let i = 0; i < shorter.length; i++) {
    if (shorter[i] === longer[i]) prefix++;
    else break;
  }
  return prefix / longer.length >= 0.75;
}

function isSubstringTrap(shorter: string, longer: string): boolean {
  const idx = longer.indexOf(shorter);
  if (idx === -1) return false;

  // Check character boundaries
  const charsBefore = idx;
  const charsAfter = longer.length - (idx + shorter.length);

  const startsAtWordBoundary = idx === 0 || /\W/.test(longer[idx - 1]);
  const endsAtWordBoundary = (idx + shorter.length === longer.length) || /\W/.test(longer[idx + shorter.length]);

  // If both ends are at word boundaries, not a trap (e.g., "national" in "the national")
  if (startsAtWordBoundary && endsAtWordBoundary) {
    return false;
  }

  // If the non-matching part is just 1 trailing character (like plural 's'), allow it
  // "the shin" in "the shins" — only 1 char after, not a trap
  // But only when the start is aligned to a word boundary (avoid "dram" in "drama")
  if (!endsAtWordBoundary && charsAfter === 1 && startsAtWordBoundary) {
    return false;
  }

  // More than 1 extra character on a non-word-boundary side, or both sides misaligned
  return true;
}

/** Simple similarity score between two strings (0-1). Uses normalized comparison. */
export function similarityScore(a: string, b: string): number {
  const na = normalizeForComparison(a);
  const nb = normalizeForComparison(b);

  if (na === nb) return 1.0;
  if (na.length === 0 || nb.length === 0) return 0;

  const longer = na.length >= nb.length ? na : nb;
  const shorter = na.length < nb.length ? na : nb;

  // Short name guard: names with 3 or fewer chars require exact match
  if (shorter.length <= 3) {
    return 0;
  }

  // Short name guard: names with 4 chars get heavily penalized for non-exact matches
  // Only a very close match (like a single accent difference already handled above) should pass
  if (shorter.length <= 4) {
    // Only allow if the shorter appears as a complete word (both boundaries aligned)
    if (longer.includes(shorter)) {
      const idx = longer.indexOf(shorter);
      const startOk = idx === 0 || /\W/.test(longer[idx - 1]);
      const endOk = (idx + shorter.length === longer.length) || /\W/.test(longer[idx + shorter.length]);
      if (startOk && endOk) {
        const coverage = shorter.length / longer.length;
        if (coverage >= 0.6) {
          return 0.8 + 0.2 * coverage;
        }
      }
    }
    // For 4-char names, prefix/suffix overlap rarely indicates a real match
    return 0;
  }

  // Substring trap: if shorter is embedded inside a longer word, reject
  if (longer.includes(shorter) && isSubstringTrap(shorter, longer)) {
    return 0.3; // Low score — not a match
  }

  // One contains the other (word-boundary aligned)
  if (longer.includes(shorter)) {
    const coverage = shorter.length / longer.length;
    // Require at least 60% coverage for substring match to count
    // "house" in "houseofvivian" = 38% → not a match
    // "national" in "the national" = 67% → match
    if (coverage >= 0.6) {
      return 0.8 + 0.2 * coverage;
    }
    // Low coverage substring: penalize heavily to prevent false matches
    // "langhorne slim" in "viva phx: langhorne slim" = 58% → not a match
    return 0.4 + 0.3 * coverage;
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
  const rawScore = totalOverlap / maxLen;

  // Word-divergence guard: when names share prefix characters but diverge into
  // different words, cap the score to prevent false positives.
  // "Black Keys" vs "Black Lips", "Mannequin Pussy" vs "Mannequins"
  if (commonPrefix >= 4 && rawScore >= 0.5) {
    const wordsA = na.split(" ");
    const wordsB = nb.split(" ");

    // Count shared complete words from start
    let sharedWords = 0;
    for (let i = 0; i < Math.min(wordsA.length, wordsB.length); i++) {
      if (wordsA[i] === wordsB[i]) sharedWords++;
      else break;
    }

    // Case 1: Names share complete words but diverge after
    // "black keys" vs "black lips" → shared ["black"], diverge at "keys" vs "lips"
    if (sharedWords > 0 && sharedWords < Math.min(wordsA.length, wordsB.length)) {
      const nextA = wordsA[sharedWords];
      const nextB = wordsB[sharedWords];
      if (!areSimilarWords(nextA, nextB)) {
        return Math.min(rawScore, 0.5);
      }
    }

    // Case 2: First words are similar (e.g., "mannequin"/"mannequins") but one name
    // has additional words — clearly different entities.
    // "mannequin pussy" vs "mannequins" → first words similar, extra word "pussy"
    if (sharedWords === 0) {
      const firstA = wordsA[0];
      const firstB = wordsB[0];
      const maxWords = Math.max(wordsA.length, wordsB.length);
      if (areSimilarWords(firstA, firstB) && maxWords > 1) {
        return Math.min(rawScore, 0.5);
      }
    }
  }

  return rawScore;
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

// Canonical field names matching the create/update path (submit-artist.ts
// artistApiFields) — these names double as PATCH body keys via buildUpdateBody,
// so they MUST be the flat API names (bandcamp, not bandcamp_url). PSY-1171.
const ARTIST_FIELDS = [
  "name", "city", "state", "country", "description",
  "website", "bandcamp", "spotify", "instagram",
  "facebook", "twitter", "youtube", "soundcloud",
];

// Mirrors ARTIST_FIELDS: only fields searchVenues can populate from the
// /venues/search response are compared. address/zipcode/capacity are
// deliberately excluded — capacity has no backend column at all, the response
// key is `zipcode` (never `zip_code`), and address/zipcode are hidden for
// unverified venues — so comparing any of them reads empty and forces a
// spurious UPDATE on every re-ingest. Restoring them correctly (verified-gated,
// plus a capacity column) is tracked in PSY-1179. PSY-1171.
const VENUE_FIELDS = [
  "name", "city", "state", "country", "description",
  "website", "bandcamp", "spotify", "instagram",
  "facebook", "twitter", "youtube", "soundcloud",
];

const RELEASE_FIELDS = [
  "title", "release_type", "release_year", "release_date",
  "bandcamp_url", "spotify_url", "description",
];

// Only fields searchLabels maps from the /labels list response are compared.
// founded_year/status and the non-website/bandcamp socials are excluded: the
// LabelListResponse (PSY-1157) carries only website + bandcamp, so comparing the
// others reads empty and forces a spurious UPDATE every re-ingest. Enriching
// label socials needs the list response widened first — tracked in PSY-1179,
// matching the venue treatment above. PSY-1171.
const LABEL_FIELDS = [
  "name", "city", "state", "country", "website", "bandcamp", "description",
];

const FESTIVAL_FIELDS = [
  "name", "series_slug", "edition_year", "start_date", "end_date",
  "city", "state", "country", "website", "description",
];

/** Get the comparable fields for a given entity type. Exported so tests can
 * assert the field-list ⊆ search-mapper invariant without hardcoding the set. */
export function getFieldsForType(entityType: EntityType): string[] {
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
  // /artists/search returns the full ArtistDetailResponse, which nests the link
  // fields (website + socials) under `social`. Flatten them to the canonical
  // top-level names the create/update path uses so dedup compares real existing
  // values instead of always reading empty — which previously suppressed link
  // enrichment on re-ingest (the field names also never matched: ARTIST_FIELDS
  // asked for bandcamp_url while the proposed entity carries bandcamp). PSY-1171.
  const result = await client.get<{
    artists: Array<{
      id: number;
      name: string;
      slug: string;
      city?: string;
      state?: string;
      country?: string;
      description?: string;
      social?: {
        website?: string;
        bandcamp?: string;
        spotify?: string;
        instagram?: string;
        facebook?: string;
        twitter?: string;
        youtube?: string;
        soundcloud?: string;
      };
    }>;
  }>("/artists/search", { q: name });

  return (result.artists || []).map((a) => ({
    id: a.id,
    name: a.name,
    slug: a.slug,
    city: a.city || "",
    state: a.state || "",
    country: a.country || "",
    description: a.description || "",
    website: a.social?.website || "",
    bandcamp: a.social?.bandcamp || "",
    spotify: a.social?.spotify || "",
    instagram: a.social?.instagram || "",
    facebook: a.social?.facebook || "",
    twitter: a.social?.twitter || "",
    youtube: a.social?.youtube || "",
    soundcloud: a.social?.soundcloud || "",
  }));
}

async function searchVenues(
  client: APIClient,
  name: string,
): Promise<EntitySearchResult[]> {
  // /venues/search returns the full VenueDetailResponse, which nests the link
  // fields (website + socials) under `social`. Flatten them to the canonical
  // top-level names the create/update path uses so dedup compares real existing
  // values instead of always reading empty (which suppressed link enrichment on
  // re-ingest). Mirrors the artist fix above. PSY-1171.
  // Only the fields in VENUE_FIELDS are mapped; address/zipcode/capacity are
  // deliberately omitted (see VENUE_FIELDS for why).
  const result = await client.get<{
    venues: Array<{
      id: number;
      name: string;
      slug: string;
      city: string;
      state: string;
      country?: string;
      description?: string;
      social?: {
        website?: string;
        bandcamp?: string;
        spotify?: string;
        instagram?: string;
        facebook?: string;
        twitter?: string;
        youtube?: string;
        soundcloud?: string;
      };
    }>;
  }>("/venues/search", { q: name });

  return (result.venues || []).map((v) => ({
    id: v.id,
    name: v.name,
    slug: v.slug,
    city: v.city,
    state: v.state,
    country: v.country || "",
    description: v.description || "",
    website: v.social?.website || "",
    bandcamp: v.social?.bandcamp || "",
    spotify: v.social?.spotify || "",
    instagram: v.social?.instagram || "",
    facebook: v.social?.facebook || "",
    twitter: v.social?.twitter || "",
    youtube: v.social?.youtube || "",
    soundcloud: v.social?.soundcloud || "",
  }));
}

async function searchReleases(
  client: APIClient,
  title: string,
): Promise<EntitySearchResult[]> {
  // PSY-1184: hit the dedicated /releases/search endpoint (searches the whole
  // dataset) instead of GET /releases (first page only) + a client-side filter.
  // The old approach silently missed any release past page 1, so re-running a
  // release batch at scale re-created existing releases as duplicates. Mirrors
  // searchArtists/searchVenues. findBestMatch applies the fuzzy score, so no
  // client-side pre-filter is needed.
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
  }>("/releases/search", { q: title });

  return (result.releases || []).map((r) => ({
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
  // Client-side filter until backend search endpoint exists.
  // The list/search response carries the dedup fields (country/website/bandcamp/
  // description) as of PSY-1157, so existing values compare correctly instead of
  // always reading empty (which forced spurious UPDATEs on re-ingest).
  const result = await client.get<{
    labels: Array<{
      id: number;
      name: string;
      slug: string;
      city?: string;
      state?: string;
      country?: string;
      website?: string;
      bandcamp?: string;
      description?: string;
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
      bandcamp: l.bandcamp || "",
      description: l.description || "",
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

// -- Show deduplication -------------------------------------------------------

export interface ShowDuplicateResult {
  isDuplicate: boolean;
  existingShowId?: number;
  existingShowSlug?: string;
}

interface ShowResponseForDedup {
  id: number;
  slug?: string;
  event_date: string;
  venues?: Array<{ id: number; name: string }>;
  artists?: Array<{ id: number; name: string }>;
}

/**
 * Extract the calendar date (YYYY-MM-DD) from a date string.
 * Handles full ISO timestamps, date-only strings, etc.
 */
function extractCalendarDate(dateStr: string): string {
  return dateStr.slice(0, 10);
}

/**
 * Compute the UTC query window that covers a show's local calendar day.
 *
 * Shows are stored with their event_date normalized to the venue's local
 * evening time converted to UTC (see `normalizeDate` in submit-show.ts) — e.g.
 * 2026-07-17 at a California (PDT) venue is stored as 2026-07-18T03:00:00Z.
 * A naive `${date}T00:00:00Z`..`${date}T23:59:59Z` window therefore queries the
 * wrong UTC day for every US timezone (20:00 local always crosses into the next
 * UTC day), never matches the stored row, and lets a re-run miss the duplicate —
 * the backend then rejects the re-insert with a confusing 422 SHOW_CREATE_FAILED.
 *
 * Deriving the window from venue-local 00:00..23:59 (converted to UTC) keeps the
 * dedup check aligned with how the show was actually stored. The default
 * (America/Phoenix) mirrors `normalizeDate`'s fallback.
 */
export function showDedupWindow(
  eventDate: string,
  state?: string,
): { fromDate: string; toDate: string } {
  const calendarDate = extractCalendarDate(eventDate);
  const timezone = state ? getTimezoneForState(state) : "America/Phoenix";
  return {
    fromDate: localTimeToUTC(calendarDate, "00:00", timezone),
    toDate: localTimeToUTC(calendarDate, "23:59", timezone),
  };
}

/**
 * Check if a show is a duplicate by searching for existing shows on the same
 * date with the same venue and at least one overlapping artist.
 *
 * Requires at least one resolved venue ID and one resolved artist ID/name to check.
 * `state` (the venue/show state) is used to align the query window with the
 * venue-timezone normalization applied when the show is created.
 */
export async function checkShowDuplicate(
  client: APIClient,
  eventDate: string,
  resolvedVenueIds: number[],
  resolvedArtistIds: number[],
  resolvedArtistNames: string[],
  state?: string,
): Promise<ShowDuplicateResult> {
  const noMatch: ShowDuplicateResult = { isDuplicate: false };

  // Need at least one venue ID to check
  if (resolvedVenueIds.length === 0) return noMatch;

  // Need at least one artist identifier to compare
  if (resolvedArtistIds.length === 0 && resolvedArtistNames.length === 0) return noMatch;

  try {
    // Query shows across the venue-local calendar day (converted to UTC) so the
    // window matches how event_date is stored. See showDedupWindow.
    const { fromDate, toDate } = showDedupWindow(eventDate, state);

    const result = await client.get<ShowResponseForDedup[]>("/shows", {
      from_date: fromDate,
      to_date: toDate,
    });

    const shows = Array.isArray(result) ? result : [];

    for (const show of shows) {
      // Check if any venue matches
      const showVenueIds = (show.venues || []).map((v) => v.id);
      const venueOverlap = resolvedVenueIds.some((vid) => showVenueIds.includes(vid));
      if (!venueOverlap) continue;

      // Check if any artist matches (by ID or fuzzy name)
      const showArtistIds = (show.artists || []).map((a) => a.id);
      const showArtistNames = (show.artists || []).map((a) => a.name);

      const artistIdOverlap = resolvedArtistIds.some((aid) => showArtistIds.includes(aid));
      const artistNameOverlap = resolvedArtistNames.some((name) =>
        showArtistNames.some((existingName) => similarityScore(name, existingName) >= 0.7),
      );

      if (artistIdOverlap || artistNameOverlap) {
        return {
          isDuplicate: true,
          existingShowId: show.id,
          existingShowSlug: show.slug,
        };
      }
    }
  } catch {
    // If the search fails, don't block creation
    return noMatch;
  }

  return noMatch;
}
