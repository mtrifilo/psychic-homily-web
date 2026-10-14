import type { APIClient } from "./api";

export interface RadioRematchResult {
  total: number;
  matched: number;
  unmatched: number;
  persist_errors?: number;
}

export interface UnmatchedPlayGroup {
  artist_name: string;
  play_count: number;
  station_names: string[];
}

export interface ChunkedRematchOptions {
  /** Filter unmatched groups to one station (admin unmatched API). */
  stationId?: number;
  /** Collect unmatched artist_name values from one show's episodes only. */
  showSlug?: string;
  /** Rematch these names only (skips unmatched listing). */
  artistNames?: string[];
  /** List names that would be rematched without POSTing. */
  dryRun?: boolean;
  /** Cap how many distinct artist names to process. */
  maxGroups?: number;
  onProgress?: (artistName: string, index: number, total: number) => void;
}

export interface ChunkedRematchResult extends RadioRematchResult {
  namesProcessed: number;
  /** Populated on dry-run for display. */
  names?: string[];
}

const UNMATCHED_PAGE_SIZE = 100;

/** Rematch unmatched radio plays for a single artist or label name. */
export async function rematchRadioPlays(
  client: APIClient,
  opts?: { artistName?: string; labelName?: string },
): Promise<RadioRematchResult> {
  const body: Record<string, string> = {};
  if (opts?.artistName) body.artist_name = opts.artistName;
  if (opts?.labelName) body.label_name = opts.labelName;
  return client.post<RadioRematchResult>("/admin/radio/rematch", body);
}

/** Sum per-name rematch results into one aggregate. */
export function aggregateRematchResults(
  results: RadioRematchResult[],
): RadioRematchResult {
  return results.reduce<RadioRematchResult>(
    (acc, r) => ({
      total: acc.total + r.total,
      matched: acc.matched + r.matched,
      unmatched: acc.unmatched + r.unmatched,
      persist_errors: (acc.persist_errors ?? 0) + (r.persist_errors ?? 0),
    }),
    { total: 0, matched: 0, unmatched: 0, persist_errors: 0 },
  );
}

/** Dedupe artist names preserving first-seen order. */
export function dedupeArtistNames(names: string[]): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const raw of names) {
    const name = raw.trim();
    if (!name || seen.has(name)) continue;
    seen.add(name);
    out.push(name);
  }
  return out;
}

/** Paginate GET /admin/radio/unmatched and return distinct artist_name values. */
export async function listUnmatchedArtistNames(
  client: APIClient,
  opts?: { stationId?: number; maxGroups?: number },
): Promise<string[]> {
  const names: string[] = [];
  let offset = 0;
  let total = Infinity;

  while (offset < total) {
    if (opts?.maxGroups !== undefined && names.length >= opts.maxGroups) {
      break;
    }

    const pageLimit =
      opts?.maxGroups !== undefined
        ? Math.min(UNMATCHED_PAGE_SIZE, opts.maxGroups - names.length)
        : UNMATCHED_PAGE_SIZE;

    const params: Record<string, string> = {
      limit: String(pageLimit),
      offset: String(offset),
    };
    if (opts?.stationId !== undefined && opts.stationId > 0) {
      params.station_id = String(opts.stationId);
    }

    const page = await client.get<{ groups: UnmatchedPlayGroup[]; total: number }>(
      "/admin/radio/unmatched",
      params,
    );

    total = page.total ?? 0;
    const groups = page.groups ?? [];
    if (groups.length === 0) break;

    for (const group of groups) {
      if (group.artist_name?.trim()) {
        names.push(group.artist_name);
      }
    }

    offset += groups.length;
  }

  return dedupeArtistNames(names);
}

/** Collect distinct unmatched artist_name values across all episodes of a show. */
export async function listUnmatchedArtistNamesForShow(
  client: APIClient,
  showSlug: string,
): Promise<string[]> {
  const names = new Set<string>();
  let offset = 0;
  let total = Infinity;
  const encoded = encodeURIComponent(showSlug);

  while (offset < total) {
    const page = await client.get<{
      episodes: Array<{ air_date: string }>;
      total: number;
    }>(`/radio-shows/${encoded}/episodes`, {
      limit: String(UNMATCHED_PAGE_SIZE),
      offset: String(offset),
    });

    total = page.total ?? 0;
    const episodes = page.episodes ?? [];
    if (episodes.length === 0) break;

    for (const episode of episodes) {
      const detail = await client.get<{
        plays: Array<{ artist_name: string; artist_id?: number | null }>;
      }>(`/radio-shows/${encoded}/episodes/${episode.air_date}`);

      for (const play of detail.plays ?? []) {
        if (!play.artist_id && play.artist_name?.trim()) {
          names.add(play.artist_name);
        }
      }
    }

    offset += episodes.length;
  }

  return [...names].sort((a, b) => a.localeCompare(b));
}

/**
 * Rematch unmatched plays in bounded per-name requests so each call stays
 * under HTTP gateway timeouts (full-table POST {} can 502 on large archives).
 */
export async function rematchRadioPlaysChunked(
  client: APIClient,
  opts: ChunkedRematchOptions = {},
): Promise<ChunkedRematchResult> {
  let names: string[];

  if (opts.artistNames?.length) {
    names = dedupeArtistNames(opts.artistNames);
    if (opts.maxGroups !== undefined) {
      names = names.slice(0, opts.maxGroups);
    }
  } else if (opts.showSlug) {
    names = await listUnmatchedArtistNamesForShow(client, opts.showSlug);
    if (opts.maxGroups !== undefined) {
      names = names.slice(0, opts.maxGroups);
    }
  } else {
    names = await listUnmatchedArtistNames(client, {
      stationId: opts.stationId,
      maxGroups: opts.maxGroups,
    });
  }

  if (opts.dryRun) {
    return {
      namesProcessed: names.length,
      names,
      total: 0,
      matched: 0,
      unmatched: 0,
      persist_errors: 0,
    };
  }

  const results: RadioRematchResult[] = [];
  for (let i = 0; i < names.length; i++) {
    const name = names[i]!;
    opts.onProgress?.(name, i + 1, names.length);
    const result = await rematchRadioPlays(client, { artistName: name });
    results.push(result);
  }

  const aggregated = aggregateRematchResults(results);
  return { namesProcessed: names.length, ...aggregated };
}

// ─── Artist enrichment candidates (GET /admin/radio/candidates/artists) ─────

export type CandidateLane = "auto" | "human";

export interface CandidateCount {
  name: string;
  play_count: number;
}

export interface ArtistCandidate {
  artist_key: string;
  display_name: string;
  artist_spellings: CandidateCount[];
  artist_spelling_count: number;
  lane: CandidateLane;
  score: number;
  play_count: number;
  station_row_count: number;
  station_family_count: number;
  show_count: number;
  first_aired_on: string;
  last_aired_on: string;
  stations: Array<{ station_id: number; slug: string; name: string; play_count: number }>;
  top_tracks: Array<{ track_id: number; title: string; play_count: number }>;
  album_names: CandidateCount[];
  album_name_count: number;
  label_names: CandidateCount[];
  label_name_count: number;
  mb_artist_id?: string;
  mb_artist_ids: string[];
  mb_artist_id_conflict: boolean;
  shared_mb_artist_keys: string[];
  non_latin_spelling: boolean;
}

export interface ArtistCandidateListing {
  candidates: ArtistCandidate[];
  total: number;
  auto_total: number;
  human_total: number;
  as_of: string;
  window_start: string;
  window_days: number;
  min_station_rows: number;
  min_plays: number;
  unkeyable_plays: number;
  tracks_rebuilt_at?: string;
}

/**
 * Fetch one snapshot of the artist candidate ranking. Read-only. Unset options
 * are omitted from the query, so the server's defaults apply.
 */
export async function listArtistCandidates(
  client: APIClient,
  opts: { lane?: string; sort?: string; limit?: string },
): Promise<ArtistCandidateListing> {
  return client.get<ArtistCandidateListing>("/admin/radio/candidates/artists", {
    lane: opts.lane ?? "",
    sort: opts.sort ?? "",
    limit: opts.limit ?? "",
  });
}

/**
 * Characters that can drive, reorder or hide text in a terminal: C0 and C1
 * controls, DEL, the bidirectional controls, and the invisible characters
 * U+00AD, U+200B, U+2060..U+2064 and U+FEFF. Joiners U+200C and U+200D stay,
 * since emoji and some scripts need them. Radio play text comes from station
 * playlists, so none of these may reach the terminal raw.
 */
const TERMINAL_CONTROL =
  /[\u0000-\u001f\u007f-\u009f\u00ad\u061c\u200b\u200e\u200f\u202a-\u202e\u2060-\u2064\u2066-\u2069\ufeff]/g;

/** Replace every TERMINAL_CONTROL character with U+FFFD. */
export function printable(text: string): string {
  return text.replace(TERMINAL_CONTROL, "\uFFFD");
}

/**
 * The TERMINAL_CONTROL characters JSON.stringify leaves raw: it already
 * escapes C0 controls inside strings, and its own newlines are layout.
 */
const JSON_RAW_TERMINAL_CONTROL =
  /[\u007f-\u009f\u00ad\u061c\u200b\u200e\u200f\u202a-\u202e\u2060-\u2064\u2066-\u2069\ufeff]/g;

/**
 * The value as indented JSON with every TERMINAL_CONTROL character inside a
 * string written as a \uXXXX escape, so the output stays valid JSON and safe
 * to print.
 */
export function printableJSON(value: unknown): string {
  return JSON.stringify(value, null, 2).replace(
    JSON_RAW_TERMINAL_CONTROL,
    (c) => `\\u${c.charCodeAt(0).toString(16).padStart(4, "0")}`,
  );
}

function countList(items: CandidateCount[], distinct?: number): string {
  if (items.length === 0) return "none";
  const shown = items.map((i) => `${i.name} (${i.play_count})`).join("; ");
  return distinct !== undefined && distinct > items.length ? `${shown}; ${distinct} distinct` : shown;
}

/**
 * Render the ranking as plain text, one block per candidate with its lane and
 * evidence. Every line passes through printable. Pure, so the report can be
 * tested without a server.
 */
export function formatArtistCandidateReport(listing: ArtistCandidateListing): string[] {
  const lines: string[] = [
    `Artist candidates as of ${listing.as_of}: plays aired ${listing.window_start} or later ` +
      `(${listing.window_days} days); floors ${listing.min_station_rows} station rows and ${listing.min_plays} plays.`,
    `${listing.total} in the requested lanes (${listing.auto_total} auto, ${listing.human_total} human overall); ` +
      `showing ${listing.candidates.length}.`,
    `${listing.unkeyable_plays} plays counted toward no candidate (the name normalizes to empty).`,
    `Top tracks reflect the radio_tracks rebuild of ${listing.tracks_rebuilt_at ?? "(none recorded)"}.`,
  ];
  listing.candidates.forEach((c, i) => {
    lines.push("");
    lines.push(
      `${String(i + 1).padStart(3)}. [${c.lane.padEnd(5)}] ${c.display_name}  ` +
        `score ${c.score.toFixed(2)}  plays ${c.play_count}  rows ${c.station_row_count}  ` +
        `families ${c.station_family_count}  shows ${c.show_count}  ` +
        `aired ${c.first_aired_on} to ${c.last_aired_on}`,
    );
    lines.push(`       key:      ${c.artist_key}`);
    if (c.artist_spelling_count > 1) {
      lines.push(`       spellings: ${countList(c.artist_spellings, c.artist_spelling_count)}`);
    }
    if (c.non_latin_spelling) {
      lines.push("       note:     a spelling carries a character normalization may not fold; the key may be a fragment of it");
    }
    lines.push(
      `       stations: ${c.stations.map((s) => `${s.name} (${s.slug}) ${s.play_count}`).join(", ") || "none"}`,
    );
    lines.push(
      `       tracks:   ${countList(c.top_tracks.map((t) => ({ name: t.title, play_count: t.play_count })))}`,
    );
    lines.push(`       albums:   ${countList(c.album_names, c.album_name_count)}`);
    lines.push(`       labels:   ${countList(c.label_names, c.label_name_count)}`);
    if (c.mb_artist_id_conflict) {
      lines.push(`       mbids:    ${c.mb_artist_ids.join(", ")} (conflict: more than one id)`);
    } else if (c.mb_artist_id) {
      lines.push(`       mbid:     ${c.mb_artist_id}`);
    }
    if (c.shared_mb_artist_keys.length > 0) {
      lines.push(`       shares its MusicBrainz id with: ${c.shared_mb_artist_keys.join(", ")}`);
    }
  });
  return lines.map(printable);
}
