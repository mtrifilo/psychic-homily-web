import { APIClient } from "../lib/api";
import type { EnvironmentConfig, EntityType } from "../lib/types";
import * as display from "../lib/display";
import { gray, dim } from "../lib/ansi";

const VALID_TYPES: EntityType[] = [
  "artist",
  "venue",
  "show",
  "release",
  "label",
  "festival",
];

export async function runSearch(
  entityType: string,
  query: string,
  env: EnvironmentConfig,
): Promise<void> {
  if (!VALID_TYPES.includes(entityType as EntityType)) {
    display.error(
      `Invalid entity type "${entityType}". Must be one of: ${VALID_TYPES.join(", ")}`,
    );
    process.exit(1);
  }

  if (!query.trim()) {
    display.error("Search query cannot be empty.");
    process.exit(1);
  }

  const client = new APIClient(env);

  display.info(`Searching ${entityType}s for "${query}"...`);

  switch (entityType as EntityType) {
    case "artist":
      await searchArtists(client, query);
      break;
    case "venue":
      await searchVenues(client, query);
      break;
    case "release":
      await searchReleases(client, query);
      break;
    case "label":
      await searchLabels(client, query);
      break;
    case "festival":
      await searchFestivals(client, query);
      break;
    case "show":
      await searchShows(client, query);
      break;
  }
}

async function searchArtists(client: APIClient, query: string): Promise<void> {
  const result = await client.get<{
    artists: Array<{
      id: number;
      name: string;
      slug: string;
      city?: string;
      state?: string;
      show_count?: number;
    }>;
    count: number;
  }>("/artists/search", { q: query });

  if (!result.artists?.length) {
    display.warn("No artists found.");
    return;
  }

  display.table([
    ["ID", "Name", "Slug", "City", "Shows"],
    ...result.artists.map((a) => [
      String(a.id),
      a.name,
      a.slug,
      a.city ? `${a.city}, ${a.state || ""}`.trim() : dim("—"),
      String(a.show_count ?? 0),
    ]),
  ]);
}

async function searchVenues(client: APIClient, query: string): Promise<void> {
  const result = await client.get<{
    venues: Array<{
      id: number;
      name: string;
      slug: string;
      city: string;
      state: string;
    }>;
    count: number;
  }>("/venues/search", { q: query });

  if (!result.venues?.length) {
    display.warn("No venues found.");
    return;
  }

  display.table([
    ["ID", "Name", "Slug", "Location"],
    ...result.venues.map((v) => [
      String(v.id),
      v.name,
      v.slug,
      `${v.city}, ${v.state}`,
    ]),
  ]);
}

async function searchReleases(
  client: APIClient,
  query: string,
): Promise<void> {
  // Uses list endpoint with filtering (until search endpoint is added in PSY-140)
  const result = await client.get<{
    releases: Array<{
      id: number;
      title: string;
      slug: string;
      release_type?: string;
      release_year?: number;
    }>;
    count: number;
  }>("/releases", {});

  // Client-side filter until backend search exists
  const filtered = (result.releases || []).filter((r) =>
    r.title.toLowerCase().includes(query.toLowerCase()),
  );

  if (!filtered.length) {
    display.warn(`No releases matching "${query}".`);
    process.stderr.write(
      `  ${gray("Note: Release search is client-side until PSY-140 adds GET /releases/search")}\n`,
    );
    return;
  }

  display.table([
    ["ID", "Title", "Type", "Year"],
    ...filtered.slice(0, 20).map((r) => [
      String(r.id),
      r.title,
      r.release_type || dim("—"),
      r.release_year ? String(r.release_year) : dim("—"),
    ]),
  ]);
}

async function searchLabels(client: APIClient, query: string): Promise<void> {
  // Uses list endpoint until PSY-140
  const result = await client.get<{
    labels: Array<{
      id: number;
      name: string;
      slug: string;
      city?: string;
      state?: string;
      status?: string;
    }>;
    count: number;
  }>("/labels", {});

  const filtered = (result.labels || []).filter((l) =>
    l.name.toLowerCase().includes(query.toLowerCase()),
  );

  if (!filtered.length) {
    display.warn(`No labels matching "${query}".`);
    process.stderr.write(
      `  ${gray("Note: Label search is client-side until PSY-140 adds GET /labels/search")}\n`,
    );
    return;
  }

  display.table([
    ["ID", "Name", "Location", "Status"],
    ...filtered.slice(0, 20).map((l) => [
      String(l.id),
      l.name,
      l.city ? `${l.city}, ${l.state || ""}`.trim() : dim("—"),
      l.status || dim("—"),
    ]),
  ]);
}

async function searchFestivals(
  client: APIClient,
  query: string,
): Promise<void> {
  // Uses list endpoint until PSY-140
  const result = await client.get<{
    festivals: Array<{
      id: number;
      name: string;
      slug: string;
      edition_year?: number;
      city?: string;
      state?: string;
    }>;
    count: number;
  }>("/festivals", {});

  const filtered = (result.festivals || []).filter((f) =>
    f.name.toLowerCase().includes(query.toLowerCase()),
  );

  if (!filtered.length) {
    display.warn(`No festivals matching "${query}".`);
    process.stderr.write(
      `  ${gray("Note: Festival search is client-side until PSY-140 adds GET /festivals/search")}\n`,
    );
    return;
  }

  display.table([
    ["ID", "Name", "Year", "Location"],
    ...filtered.slice(0, 20).map((f) => [
      String(f.id),
      f.name,
      f.edition_year ? String(f.edition_year) : dim("—"),
      f.city ? `${f.city}, ${f.state || ""}`.trim() : dim("—"),
    ]),
  ]);
}

async function searchShows(client: APIClient, query: string): Promise<void> {
  // Shows don't have a text search — search by city or date
  display.warn(
    'Show search is limited. Use filters like: ph search show "Phoenix" (searches by city).',
  );
  const result = await client.get<{
    shows: Array<{
      id: number;
      title?: string;
      event_date: string;
      city: string;
      state: string;
    }>;
    count: number;
  }>("/shows", { city: query });

  if (!result.shows?.length) {
    display.warn(`No shows found in "${query}".`);
    return;
  }

  display.table([
    ["ID", "Date", "Title", "Location"],
    ...result.shows.slice(0, 20).map((s) => [
      String(s.id),
      s.event_date.slice(0, 10),
      s.title || dim("(untitled)"),
      `${s.city}, ${s.state}`,
    ]),
  ]);
}
