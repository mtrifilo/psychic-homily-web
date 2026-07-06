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
