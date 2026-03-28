import { describe, test, expect, beforeEach } from "bun:test";
import {
  addArtistsToShow,
  removeArtistFromShow,
  parseShowArtistInput,
  getShow,
  type ArtistAddResult,
  type ArtistRemoveResult,
} from "../src/commands/show";

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

// --- Helper to set up a show that can be resolved ---
function setupShowMock(show?: Record<string, unknown>): void {
  const defaultShow = {
    id: 668,
    title: "Pavement @ Valley Bar",
    slug: "pavement-valley-bar-2026-03-15",
    artists: [
      { id: 10, name: "Pavement", slug: "pavement", is_headliner: true },
    ],
  };

  const merged = { ...defaultShow, ...show };

  addMockRoute("GET", /\/shows\/\d+$/, () => merged);
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

describe("parseShowArtistInput", () => {
  test("parses array of artist objects", () => {
    const input = JSON.stringify([
      { name: "Soapbox Derby", is_headliner: false },
      { name: "Bosses Band" },
    ]);
    const result = parseShowArtistInput(input);
    expect(result).toHaveLength(2);
    expect(result[0].name).toBe("Soapbox Derby");
    expect(result[0].is_headliner).toBe(false);
    expect(result[1].name).toBe("Bosses Band");
  });

  test("wraps a single object in array", () => {
    const input = JSON.stringify({ name: "Soapbox Derby" });
    const result = parseShowArtistInput(input);
    expect(result).toHaveLength(1);
    expect(result[0].name).toBe("Soapbox Derby");
  });

  test("throws on invalid JSON", () => {
    expect(() => parseShowArtistInput("not json")).toThrow();
  });

  test("handles is_headliner field", () => {
    const input = JSON.stringify([{ name: "Pavement", is_headliner: true }]);
    const result = parseShowArtistInput(input);
    expect(result[0]).toMatchObject({
      name: "Pavement",
      is_headliner: true,
    });
  });
});

describe("getShow", () => {
  test("resolves show by numeric ID", async () => {
    setupShowMock();
    const { APIClient } = await import("../src/lib/api");
    const client = new APIClient(TEST_ENV);
    const result = await getShow(client, "668");
    expect(result).toMatchObject({ id: 668, title: "Pavement @ Valley Bar" });
    expect(result?.artists).toHaveLength(1);
  });

  test("returns null for unknown show", async () => {
    // No mock set up — will get 404
    const { APIClient } = await import("../src/lib/api");
    const client = new APIClient(TEST_ENV);
    const result = await getShow(client, "99999");
    expect(result).toBeNull();
  });
});

describe("addArtistsToShow", () => {
  test("adds artists in dry-run mode (no mutations)", async () => {
    setupShowMock();
    setupArtistSearchMock({
      "Soapbox Derby": { id: 20, name: "Soapbox Derby", slug: "soapbox-derby" },
    });

    const artists = [{ name: "Soapbox Derby" }];
    const results = await addArtistsToShow("668", artists, TEST_ENV, false);

    // Dry-run returns empty results
    expect(results).toHaveLength(0);

    // No PUT calls should have been made
    const mutationCalls = fetchCalls.filter(
      (c) => c.method === "PUT" || c.method === "POST" || c.method === "DELETE",
    );
    expect(mutationCalls).toHaveLength(0);
  });

  test("adds artists with --confirm", async () => {
    setupShowMock();
    setupArtistSearchMock({
      "Soapbox Derby": { id: 20, name: "Soapbox Derby", slug: "soapbox-derby" },
      "Bosses Band": { id: 30, name: "Bosses Band", slug: "bosses-band" },
    });

    // PUT to update show
    addMockRoute("PUT", /\/shows\/668$/, () => ({ id: 668 }));

    const artists = [
      { name: "Soapbox Derby" },
      { name: "Bosses Band" },
    ];
    const results = await addArtistsToShow("668", artists, TEST_ENV, true);

    expect(results).toHaveLength(2);
    expect(results[0]).toMatchObject({
      name: "Soapbox Derby",
      action: "added",
      artistId: 20,
    });
    expect(results[1]).toMatchObject({
      name: "Bosses Band",
      action: "added",
      artistId: 30,
    });

    // Verify PUT call was made with merged artist list
    const putCalls = fetchCalls.filter(
      (c) => c.method === "PUT" && /\/shows\/668$/.test(c.url),
    );
    expect(putCalls).toHaveLength(1);
    // Should include existing artist (Pavement, ID 10) + new ones
    expect(putCalls[0].body).toMatchObject({
      artists: [
        { id: 10, is_headliner: true },
        { id: 20, is_headliner: false },
        { id: 30, is_headliner: false },
      ],
    });
  });

  test("handles artist not found gracefully", async () => {
    setupShowMock();
    addMockRoute("GET", /\/artists\/search/, () => ({ artists: [] }));

    // PUT for the remaining valid artists (if any)
    addMockRoute("PUT", /\/shows\/668$/, () => ({ id: 668 }));

    const artists = [{ name: "Unknown Band" }];
    const results = await addArtistsToShow("668", artists, TEST_ENV, true);

    expect(results).toHaveLength(1);
    expect(results[0]).toMatchObject({
      name: "Unknown Band",
      action: "not_found",
    });

    // No PUT call should have been made (nothing to add)
    const putCalls = fetchCalls.filter(
      (c) => c.method === "PUT" && /\/shows\/668$/.test(c.url),
    );
    expect(putCalls).toHaveLength(0);
  });

  test("handles already-linked artist gracefully", async () => {
    setupShowMock(); // Has Pavement (ID: 10) already
    setupArtistSearchMock({
      "Pavement": { id: 10, name: "Pavement", slug: "pavement" },
    });

    const artists = [{ name: "Pavement" }];
    const results = await addArtistsToShow("668", artists, TEST_ENV, true);

    expect(results).toHaveLength(1);
    expect(results[0]).toMatchObject({
      name: "Pavement",
      action: "already_linked",
      artistId: 10,
    });

    // No PUT call should have been made (nothing new to add)
    const putCalls = fetchCalls.filter(
      (c) => c.method === "PUT" && /\/shows\/668$/.test(c.url),
    );
    expect(putCalls).toHaveLength(0);
  });

  test("returns empty when show not found", async () => {
    // No show mock — will get 404
    const artists = [{ name: "Pavement" }];
    const results = await addArtistsToShow("99999", artists, TEST_ENV, true);

    expect(results).toHaveLength(0);
  });

  test("handles PUT error gracefully", async () => {
    setupShowMock();
    setupArtistSearchMock({
      "Soapbox Derby": { id: 20, name: "Soapbox Derby", slug: "soapbox-derby" },
    });

    // PUT returns 500
    addMockRouteWithStatus("PUT", /\/shows\/668$/, 500, () => ({
      message: "Internal server error",
    }));

    const artists = [{ name: "Soapbox Derby" }];
    const results = await addArtistsToShow("668", artists, TEST_ENV, true);

    expect(results).toHaveLength(1);
    expect(results[0]).toMatchObject({
      name: "Soapbox Derby",
      action: "error",
      artistId: 20,
    });
    expect(results[0].error).toBeDefined();
  });

  test("adds artist with is_headliner flag", async () => {
    setupShowMock();
    setupArtistSearchMock({
      "New Headliner": { id: 50, name: "New Headliner", slug: "new-headliner" },
    });

    addMockRoute("PUT", /\/shows\/668$/, () => ({ id: 668 }));

    const artists = [{ name: "New Headliner", is_headliner: true }];
    const results = await addArtistsToShow("668", artists, TEST_ENV, true);

    expect(results).toHaveLength(1);
    expect(results[0]).toMatchObject({
      name: "New Headliner",
      action: "added",
      artistId: 50,
    });

    // Verify PUT body includes is_headliner
    const putCalls = fetchCalls.filter(
      (c) => c.method === "PUT" && /\/shows\/668$/.test(c.url),
    );
    expect(putCalls).toHaveLength(1);
    const newArtist = putCalls[0].body.artists.find(
      (a: { id: number }) => a.id === 50,
    );
    expect(newArtist).toMatchObject({ id: 50, is_headliner: true });
  });

  test("mix of new, already-linked, and not-found artists", async () => {
    setupShowMock(); // Has Pavement (ID: 10) already
    setupArtistSearchMock({
      "Pavement": { id: 10, name: "Pavement", slug: "pavement" },
      "Soapbox Derby": { id: 20, name: "Soapbox Derby", slug: "soapbox-derby" },
    });

    addMockRoute("PUT", /\/shows\/668$/, () => ({ id: 668 }));

    const artists = [
      { name: "Pavement" },       // already linked
      { name: "Soapbox Derby" },   // new
      { name: "Unknown Band" },    // not found
    ];
    const results = await addArtistsToShow("668", artists, TEST_ENV, true);

    expect(results).toHaveLength(3);
    expect(results.find((r) => r.name === "Pavement")?.action).toBe("already_linked");
    expect(results.find((r) => r.name === "Soapbox Derby")?.action).toBe("added");
    expect(results.find((r) => r.name === "Unknown Band")?.action).toBe("not_found");
  });
});

describe("removeArtistFromShow", () => {
  test("removes artist by name in dry-run mode (no mutations)", async () => {
    setupShowMock();
    setupArtistSearchMock({
      "Pavement": { id: 10, name: "Pavement", slug: "pavement" },
    });

    const result = await removeArtistFromShow("668", "Pavement", TEST_ENV, false);

    // Dry-run still returns the planned action
    expect(result.action).toBe("removed");
    expect(result.artistId).toBe(10);

    // No PUT calls should have been made
    const putCalls = fetchCalls.filter((c) => c.method === "PUT");
    expect(putCalls).toHaveLength(0);
  });

  test("removes artist by name with --confirm", async () => {
    setupShowMock({
      artists: [
        { id: 10, name: "Pavement", slug: "pavement", is_headliner: true },
        { id: 20, name: "Soapbox Derby", slug: "soapbox-derby", is_headliner: false },
      ],
    });
    setupArtistSearchMock({
      "Pavement": { id: 10, name: "Pavement", slug: "pavement" },
    });
    addMockRoute("PUT", /\/shows\/668$/, () => ({ id: 668 }));

    const result = await removeArtistFromShow("668", "Pavement", TEST_ENV, true);

    expect(result).toMatchObject({
      name: "Pavement",
      action: "removed",
      artistId: 10,
    });

    // Verify PUT was called with remaining artist only
    const putCalls = fetchCalls.filter(
      (c) => c.method === "PUT" && /\/shows\/668$/.test(c.url),
    );
    expect(putCalls).toHaveLength(1);
    expect(putCalls[0].body).toMatchObject({
      artists: [{ id: 20, is_headliner: false }],
    });
  });

  test("removes artist by numeric ID", async () => {
    setupShowMock({
      artists: [
        { id: 10, name: "Pavement", slug: "pavement", is_headliner: true },
        { id: 20, name: "Soapbox Derby", slug: "soapbox-derby", is_headliner: false },
      ],
    });
    addMockRoute("PUT", /\/shows\/668$/, () => ({ id: 668 }));

    const result = await removeArtistFromShow("668", "10", TEST_ENV, true);

    expect(result).toMatchObject({
      name: "10",
      action: "removed",
      artistId: 10,
    });

    // Verify PUT was called with remaining artist only
    const putCalls = fetchCalls.filter(
      (c) => c.method === "PUT" && /\/shows\/668$/.test(c.url),
    );
    expect(putCalls).toHaveLength(1);
    expect(putCalls[0].body).toMatchObject({
      artists: [{ id: 20, is_headliner: false }],
    });
  });

  test("returns not_found when show not found", async () => {
    // No show mock — 404
    const result = await removeArtistFromShow("99999", "Pavement", TEST_ENV, true);
    expect(result.action).toBe("not_found");
  });

  test("returns not_found when artist name not resolved", async () => {
    setupShowMock();
    addMockRoute("GET", /\/artists\/search/, () => ({ artists: [] }));

    const result = await removeArtistFromShow("668", "Unknown Band", TEST_ENV, true);
    expect(result.action).toBe("not_found");
  });

  test("returns not_found when artist is not on the show", async () => {
    setupShowMock(); // Only has Pavement (ID: 10)
    setupArtistSearchMock({
      "Soapbox Derby": { id: 20, name: "Soapbox Derby", slug: "soapbox-derby" },
    });

    const result = await removeArtistFromShow("668", "Soapbox Derby", TEST_ENV, true);

    expect(result).toMatchObject({
      name: "Soapbox Derby",
      action: "not_found",
      artistId: 20,
    });

    // No PUT calls should have been made
    const putCalls = fetchCalls.filter((c) => c.method === "PUT");
    expect(putCalls).toHaveLength(0);
  });

  test("handles PUT error gracefully", async () => {
    setupShowMock();
    setupArtistSearchMock({
      "Pavement": { id: 10, name: "Pavement", slug: "pavement" },
    });
    addMockRouteWithStatus("PUT", /\/shows\/668$/, 500, () => ({
      message: "Internal server error",
    }));

    const result = await removeArtistFromShow("668", "Pavement", TEST_ENV, true);

    expect(result.action).toBe("error");
    expect(result.error).toBeDefined();
  });

  test("returns not_found when numeric ID is not on the show", async () => {
    setupShowMock(); // Only has Pavement (ID: 10)

    const result = await removeArtistFromShow("668", "99", TEST_ENV, true);

    expect(result).toMatchObject({
      name: "99",
      action: "not_found",
      artistId: 99,
    });
  });
});
