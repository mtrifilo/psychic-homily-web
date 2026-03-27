import { describe, test, expect, beforeEach } from "bun:test";
import {
  linkArtistsToFestival,
  unlinkArtistFromFestival,
  parseArtistInput,
  resolveFestival,
  resolveArtistId,
  getFestivalArtists,
  type ArtistLinkResult,
  type ArtistUnlinkResult,
} from "../src/commands/festival";

// --- Mock fetch for API calls ---

type MockRoute = {
  method: string;
  pattern: RegExp;
  handler: (url: string, body?: unknown) => { status?: number; body: unknown };
};

let mockRoutes: MockRoute[] = [];
let fetchCalls: { method: string; url: string; body?: unknown }[] = [];

function addMockRoute(
  method: string,
  pattern: RegExp,
  handler: (url: string, body?: unknown) => unknown,
): void {
  mockRoutes.push({
    method,
    pattern,
    handler: (url, body) => ({ status: 200, body: handler(url, body) }),
  });
}

function addMockRouteWithStatus(
  method: string,
  pattern: RegExp,
  status: number,
  handler: (url: string, body?: unknown) => unknown,
): void {
  mockRoutes.push({
    method,
    pattern,
    handler: (url, body) => ({ status, body: handler(url, body) }),
  });
}

function resetMocks(): void {
  mockRoutes = [];
  fetchCalls = [];
}

// Install global fetch mock
beforeEach(() => {
  resetMocks();

  globalThis.fetch = (async (
    input: string | URL | Request,
    init?: RequestInit,
  ) => {
    const url = typeof input === "string" ? input : input.toString();
    const method = init?.method || "GET";
    const body = init?.body ? JSON.parse(init.body as string) : undefined;

    fetchCalls.push({ method, url, body });

    for (const route of mockRoutes) {
      if (route.method === method && route.pattern.test(url)) {
        const response = route.handler(url, body);
        return new Response(JSON.stringify(response.body), {
          status: response.status ?? 200,
          headers: { "Content-Type": "application/json" },
        });
      }
    }

    // Default: 404
    return new Response(
      JSON.stringify({ message: "Not found" }),
      { status: 404 },
    );
  }) as typeof fetch;
});

const TEST_ENV = { url: "http://localhost:8080", token: "phk_test_token" };

// --- Helper to set up a festival that can be resolved ---
function setupFestivalMock(festival?: Record<string, unknown>): void {
  const defaultFestival = {
    id: 42,
    name: "Viva PHX 2026",
    slug: "viva-phx-2026",
    series_slug: "viva-phx",
    edition_year: 2026,
    ...festival,
  };

  addMockRoute("GET", /\/festivals\/[^/]+$/, () => defaultFestival);
}

// --- Helper to set up artist search ---
function setupArtistSearchMock(
  artists: Record<string, { id: number; name: string; slug: string }>,
): void {
  addMockRoute("GET", /\/artists\/search/, (url) => {
    const urlObj = new URL(url);
    const q = (urlObj.searchParams.get("q") || "").toLowerCase();
    for (const [key, artist] of Object.entries(artists)) {
      if (q.includes(key.toLowerCase()) || key.toLowerCase().includes(q)) {
        return { artists: [artist] };
      }
    }
    return { artists: [] };
  });
}

// --- Helper to set up festival artist listing ---
function setupFestivalArtistsMock(
  artists: Array<{ artist_id: number; artist_name: string }>,
): void {
  addMockRoute("GET", /\/festivals\/\d+\/artists$/, () => ({
    artists,
    count: artists.length,
  }));
}

describe("parseArtistInput", () => {
  test("parses array of artist objects", () => {
    const input = JSON.stringify([
      { name: "Pavement", billing_tier: "headliner" },
      { name: "Yo La Tengo" },
    ]);
    const result = parseArtistInput(input);
    expect(result).toHaveLength(2);
    expect(result[0].name).toBe("Pavement");
    expect(result[0].billing_tier).toBe("headliner");
    expect(result[1].name).toBe("Yo La Tengo");
  });

  test("wraps a single object in array", () => {
    const input = JSON.stringify({ name: "Pavement" });
    const result = parseArtistInput(input);
    expect(result).toHaveLength(1);
    expect(result[0].name).toBe("Pavement");
  });

  test("throws on invalid JSON", () => {
    expect(() => parseArtistInput("not json")).toThrow();
  });

  test("handles all optional fields", () => {
    const input = JSON.stringify([{
      name: "Khruangbin",
      billing_tier: "headliner",
      position: 0,
      day_date: "2026-03-07",
      stage: "Main Stage",
      set_time: "21:00:00",
    }]);
    const result = parseArtistInput(input);
    expect(result[0]).toMatchObject({
      name: "Khruangbin",
      billing_tier: "headliner",
      position: 0,
      day_date: "2026-03-07",
      stage: "Main Stage",
      set_time: "21:00:00",
    });
  });
});

describe("resolveFestival", () => {
  test("resolves festival by slug", async () => {
    setupFestivalMock();
    const { APIClient } = await import("../src/lib/api");
    const client = new APIClient(TEST_ENV);
    const result = await resolveFestival(client, "viva-phx-2026");
    expect(result).toEqual({ id: 42, name: "Viva PHX 2026", slug: "viva-phx-2026" });
  });

  test("returns null for unknown festival", async () => {
    // No mock set up — will get 404
    const { APIClient } = await import("../src/lib/api");
    const client = new APIClient(TEST_ENV);
    const result = await resolveFestival(client, "unknown-fest");
    expect(result).toBeNull();
  });
});

describe("resolveArtistId", () => {
  test("resolves exact match", async () => {
    setupArtistSearchMock({
      "Pavement": { id: 10, name: "Pavement", slug: "pavement" },
    });
    const { APIClient } = await import("../src/lib/api");
    const client = new APIClient(TEST_ENV);
    const result = await resolveArtistId(client, "Pavement");
    expect(result).toEqual({ id: 10, name: "Pavement", confidence: 1.0 });
  });

  test("returns null for no match", async () => {
    addMockRoute("GET", /\/artists\/search/, () => ({ artists: [] }));
    const { APIClient } = await import("../src/lib/api");
    const client = new APIClient(TEST_ENV);
    const result = await resolveArtistId(client, "Nonexistent Band");
    expect(result).toBeNull();
  });
});

describe("linkArtistsToFestival", () => {
  test("links artists in dry-run mode (no mutations)", async () => {
    setupFestivalMock();
    setupArtistSearchMock({
      "Pavement": { id: 10, name: "Pavement", slug: "pavement" },
    });

    const artists = [{ name: "Pavement", billing_tier: "headliner" }];
    const results = await linkArtistsToFestival("viva-phx-2026", artists, TEST_ENV, {
      confirm: false,
      replace: false,
    });

    // Dry-run returns empty results
    expect(results).toHaveLength(0);

    // No POST or DELETE calls should have been made
    const mutationCalls = fetchCalls.filter(
      (c) => c.method === "POST" || c.method === "DELETE",
    );
    expect(mutationCalls).toHaveLength(0);
  });

  test("links artists with --confirm", async () => {
    setupFestivalMock();
    setupArtistSearchMock({
      "Pavement": { id: 10, name: "Pavement", slug: "pavement" },
      "Yo La Tengo": { id: 20, name: "Yo La Tengo", slug: "yo-la-tengo" },
    });

    // POST to link artists
    addMockRoute("POST", /\/festivals\/42\/artists$/, () => ({ id: 1 }));

    const artists = [
      { name: "Pavement", billing_tier: "headliner" },
      { name: "Yo La Tengo", billing_tier: "sub_headliner" },
    ];
    const results = await linkArtistsToFestival("viva-phx-2026", artists, TEST_ENV, {
      confirm: true,
      replace: false,
    });

    expect(results).toHaveLength(2);
    expect(results[0]).toMatchObject({
      name: "Pavement",
      action: "linked",
      artistId: 10,
    });
    expect(results[1]).toMatchObject({
      name: "Yo La Tengo",
      action: "linked",
      artistId: 20,
    });

    // Verify POST calls
    const postCalls = fetchCalls.filter(
      (c) => c.method === "POST" && /\/festivals\/42\/artists$/.test(c.url),
    );
    expect(postCalls).toHaveLength(2);
    expect(postCalls[0].body).toMatchObject({
      artist_id: 10,
      billing_tier: "headliner",
    });
    expect(postCalls[1].body).toMatchObject({
      artist_id: 20,
      billing_tier: "sub_headliner",
    });
  });

  test("handles artist not found gracefully", async () => {
    setupFestivalMock();
    addMockRoute("GET", /\/artists\/search/, () => ({ artists: [] }));

    const artists = [{ name: "Unknown Band" }];
    const results = await linkArtistsToFestival("viva-phx-2026", artists, TEST_ENV, {
      confirm: true,
      replace: false,
    });

    expect(results).toHaveLength(1);
    expect(results[0]).toMatchObject({
      name: "Unknown Band",
      action: "not_found",
    });
  });

  test("handles 409 already linked gracefully", async () => {
    setupFestivalMock();
    setupArtistSearchMock({
      "Pavement": { id: 10, name: "Pavement", slug: "pavement" },
    });

    // POST returns 409
    addMockRouteWithStatus("POST", /\/festivals\/42\/artists$/, 409, () => ({
      message: "Artist already linked to festival",
    }));

    const artists = [{ name: "Pavement" }];
    const results = await linkArtistsToFestival("viva-phx-2026", artists, TEST_ENV, {
      confirm: true,
      replace: false,
    });

    expect(results).toHaveLength(1);
    expect(results[0]).toMatchObject({
      name: "Pavement",
      action: "already_linked",
      artistId: 10,
    });
  });

  test("returns empty when festival not found", async () => {
    // No festival mock — will get 404
    const artists = [{ name: "Pavement" }];
    const results = await linkArtistsToFestival("unknown-fest", artists, TEST_ENV, {
      confirm: true,
      replace: false,
    });

    expect(results).toHaveLength(0);
  });

  test("returns empty on invalid billing tier", async () => {
    setupFestivalMock();

    const artists = [{ name: "Pavement", billing_tier: "mega_star" }];
    const results = await linkArtistsToFestival("viva-phx-2026", artists, TEST_ENV, {
      confirm: true,
      replace: false,
    });

    expect(results).toHaveLength(0);
  });

  test("replace mode removes existing artists first", async () => {
    setupFestivalMock();

    // Existing artists on the festival
    setupFestivalArtistsMock([
      { artist_id: 5, artist_name: "Old Artist" },
      { artist_id: 6, artist_name: "Another Old" },
    ]);

    setupArtistSearchMock({
      "Pavement": { id: 10, name: "Pavement", slug: "pavement" },
    });

    // DELETE to remove existing artists
    addMockRoute("DELETE", /\/festivals\/42\/artists\/\d+$/, () => ({}));

    // POST to link new artists
    addMockRoute("POST", /\/festivals\/42\/artists$/, () => ({ id: 1 }));

    const artists = [{ name: "Pavement", billing_tier: "headliner" }];
    const results = await linkArtistsToFestival("viva-phx-2026", artists, TEST_ENV, {
      confirm: true,
      replace: true,
    });

    expect(results).toHaveLength(1);
    expect(results[0]).toMatchObject({
      name: "Pavement",
      action: "linked",
      artistId: 10,
    });

    // Verify DELETE calls were made for existing artists
    const deleteCalls = fetchCalls.filter(
      (c) => c.method === "DELETE" && /\/festivals\/42\/artists\/\d+$/.test(c.url),
    );
    expect(deleteCalls).toHaveLength(2);

    // Verify POST call was made for new artist
    const postCalls = fetchCalls.filter(
      (c) => c.method === "POST" && /\/festivals\/42\/artists$/.test(c.url),
    );
    expect(postCalls).toHaveLength(1);
  });

  test("replace mode with no existing artists still links new ones", async () => {
    setupFestivalMock();

    // No existing artists
    setupFestivalArtistsMock([]);

    setupArtistSearchMock({
      "Pavement": { id: 10, name: "Pavement", slug: "pavement" },
    });

    addMockRoute("POST", /\/festivals\/42\/artists$/, () => ({ id: 1 }));

    const artists = [{ name: "Pavement" }];
    const results = await linkArtistsToFestival("viva-phx-2026", artists, TEST_ENV, {
      confirm: true,
      replace: true,
    });

    expect(results).toHaveLength(1);
    expect(results[0]).toMatchObject({
      name: "Pavement",
      action: "linked",
    });

    // No DELETE calls (no existing artists)
    const deleteCalls = fetchCalls.filter((c) => c.method === "DELETE");
    expect(deleteCalls).toHaveLength(0);
  });

  test("passes all optional fields to POST body", async () => {
    setupFestivalMock();
    setupArtistSearchMock({
      "Khruangbin": { id: 10, name: "Khruangbin", slug: "khruangbin" },
    });
    addMockRoute("POST", /\/festivals\/42\/artists$/, () => ({ id: 1 }));

    const artists = [{
      name: "Khruangbin",
      billing_tier: "headliner",
      position: 0,
      day_date: "2026-03-07",
      stage: "Main Stage",
      set_time: "21:00:00",
    }];
    await linkArtistsToFestival("viva-phx-2026", artists, TEST_ENV, {
      confirm: true,
      replace: false,
    });

    const postCalls = fetchCalls.filter(
      (c) => c.method === "POST" && /\/festivals\/42\/artists$/.test(c.url),
    );
    expect(postCalls).toHaveLength(1);
    expect(postCalls[0].body).toMatchObject({
      artist_id: 10,
      billing_tier: "headliner",
      position: 0,
      day_date: "2026-03-07",
      stage: "Main Stage",
      set_time: "21:00:00",
    });
  });
});

describe("unlinkArtistFromFestival", () => {
  test("unlinks artist by name in dry-run mode (no mutations)", async () => {
    setupFestivalMock();
    setupArtistSearchMock({
      "Pavement": { id: 10, name: "Pavement", slug: "pavement" },
    });

    const result = await unlinkArtistFromFestival("viva-phx-2026", "Pavement", TEST_ENV, false);

    // Dry-run still returns the planned action
    expect(result.action).toBe("unlinked");
    expect(result.artistId).toBe(10);

    // No DELETE calls should have been made
    const deleteCalls = fetchCalls.filter((c) => c.method === "DELETE");
    expect(deleteCalls).toHaveLength(0);
  });

  test("unlinks artist by name with --confirm", async () => {
    setupFestivalMock();
    setupArtistSearchMock({
      "Pavement": { id: 10, name: "Pavement", slug: "pavement" },
    });
    addMockRoute("DELETE", /\/festivals\/42\/artists\/10$/, () => ({}));

    const result = await unlinkArtistFromFestival("viva-phx-2026", "Pavement", TEST_ENV, true);

    expect(result).toMatchObject({
      name: "Pavement",
      action: "unlinked",
      artistId: 10,
    });

    const deleteCalls = fetchCalls.filter(
      (c) => c.method === "DELETE" && /\/festivals\/42\/artists\/10$/.test(c.url),
    );
    expect(deleteCalls).toHaveLength(1);
  });

  test("unlinks artist by numeric ID", async () => {
    setupFestivalMock();
    addMockRoute("DELETE", /\/festivals\/42\/artists\/99$/, () => ({}));

    const result = await unlinkArtistFromFestival("viva-phx-2026", "99", TEST_ENV, true);

    expect(result).toMatchObject({
      name: "99",
      action: "unlinked",
      artistId: 99,
    });
  });

  test("returns not_found when festival not found", async () => {
    // No festival mock — 404
    const result = await unlinkArtistFromFestival("unknown-fest", "Pavement", TEST_ENV, true);
    expect(result.action).toBe("not_found");
  });

  test("returns not_found when artist name not resolved", async () => {
    setupFestivalMock();
    addMockRoute("GET", /\/artists\/search/, () => ({ artists: [] }));

    const result = await unlinkArtistFromFestival("viva-phx-2026", "Unknown Band", TEST_ENV, true);
    expect(result.action).toBe("not_found");
  });

  test("handles 404 when artist is not linked to festival", async () => {
    setupFestivalMock();
    setupArtistSearchMock({
      "Pavement": { id: 10, name: "Pavement", slug: "pavement" },
    });
    addMockRouteWithStatus("DELETE", /\/festivals\/42\/artists\/10$/, 404, () => ({
      message: "Artist not found in festival lineup",
    }));

    const result = await unlinkArtistFromFestival("viva-phx-2026", "Pavement", TEST_ENV, true);

    expect(result).toMatchObject({
      name: "Pavement",
      action: "not_found",
      artistId: 10,
    });
  });

  test("handles API error gracefully", async () => {
    setupFestivalMock();
    setupArtistSearchMock({
      "Pavement": { id: 10, name: "Pavement", slug: "pavement" },
    });
    addMockRouteWithStatus("DELETE", /\/festivals\/42\/artists\/10$/, 500, () => ({
      message: "Internal server error",
    }));

    const result = await unlinkArtistFromFestival("viva-phx-2026", "Pavement", TEST_ENV, true);

    expect(result.action).toBe("error");
    expect(result.error).toBeDefined();
  });
});
