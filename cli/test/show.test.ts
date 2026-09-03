import { describe, test, expect, beforeEach } from "bun:test";
import {
  addArtistsToShow,
  removeArtistFromShow,
  parseShowArtistInput,
  getShow,
  headlinerCollisions,
  unroundtrippableActs,
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

  test("reports an error when show not found, so the run does not exit 0", async () => {
    // No show mock — will get 404
    const artists = [{ name: "Pavement" }];
    const results = await addArtistsToShow("99999", artists, TEST_ENV, true);

    expect(results).toHaveLength(1);
    expect(results[0].action).toBe("error");
    expect(results[0].error).toContain("not found");
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
    const putBody = putCalls[0].body as { artists: { id: number }[] };
    const newArtist = putBody.artists.find((a) => a.id === 50);
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
    setupShowMock({
      artists: [
        { id: 10, name: "Pavement", slug: "pavement", is_headliner: true },
        { id: 20, name: "Soapbox Derby", slug: "soapbox-derby", is_headliner: false },
      ],
    });
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
    setupShowMock({
      artists: [
        { id: 10, name: "Pavement", slug: "pavement", is_headliner: true },
        { id: 20, name: "Soapbox Derby", slug: "soapbox-derby", is_headliner: false },
      ],
    });
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

// --- Curated bill roles survive an edit -------------------------------------

/** A bill whose acts each hold a different curated role. */
function setupCuratedBillMock(): void {
  setupShowMock({
    artists: [
      { id: 10, name: "Pavement", slug: "pavement", is_headliner: true, set_type: "headliner" },
      { id: 11, name: "Bosses Band", slug: "bosses-band", is_headliner: false, set_type: "direct_support" },
      { id: 12, name: "Soapbox Derby", slug: "soapbox-derby", is_headliner: false, set_type: "opener" },
    ],
  });
}

/** The bill sent by the single PUT this edit issued. */
function putBill(): Record<string, unknown>[] {
  const putCalls = fetchCalls.filter(
    (c) => c.method === "PUT" && /\/shows\/668$/.test(c.url),
  );
  expect(putCalls).toHaveLength(1);
  return (putCalls[0].body as { artists: Record<string, unknown>[] }).artists;
}

describe("bill roles round-trip", () => {
  test("add-artist preserves every untouched act's role byte-identically", async () => {
    setupCuratedBillMock();
    setupArtistSearchMock({
      "Nite Fields": { id: 20, name: "Nite Fields", slug: "nite-fields" },
    });
    addMockRoute("PUT", /\/shows\/668$/, () => ({ id: 668 }));

    await addArtistsToShow("668", [{ name: "Nite Fields" }], TEST_ENV, true);

    const bill = putBill();
    expect(bill.slice(0, 3)).toEqual([
      { id: 10, is_headliner: true, set_type: "headliner" },
      { id: 11, is_headliner: false, set_type: "direct_support" },
      { id: 12, is_headliner: false, set_type: "opener" },
    ]);
  });

  test("remove-artist preserves every remaining act's role byte-identically", async () => {
    setupCuratedBillMock();
    addMockRoute("PUT", /\/shows\/668$/, () => ({ id: 668 }));

    await removeArtistFromShow("668", "12", TEST_ENV, true);

    expect(putBill()).toEqual([
      { id: 10, is_headliner: true, set_type: "headliner" },
      { id: 11, is_headliner: false, set_type: "direct_support" },
    ]);
  });

  test("a stored 'performer' goes back as an ABSENT key, never a stated role", async () => {
    setupShowMock({
      artists: [
        { id: 10, name: "Pavement", slug: "pavement", is_headliner: true, set_type: "headliner" },
        { id: 11, name: "Bosses Band", slug: "bosses-band", is_headliner: false, set_type: "performer" },
      ],
    });
    setupArtistSearchMock({
      "Nite Fields": { id: 20, name: "Nite Fields", slug: "nite-fields" },
    });
    addMockRoute("PUT", /\/shows\/668$/, () => ({ id: 668 }));

    await addArtistsToShow("668", [{ name: "Nite Fields" }], TEST_ENV, true);

    const bill = putBill();
    expect("set_type" in bill[1]).toBe(false);
    expect(bill[1]).toEqual({ id: 11, is_headliner: false });
  });

  test("an act stating no role is added with no set_type key", async () => {
    setupCuratedBillMock();
    setupArtistSearchMock({
      "Nite Fields": { id: 20, name: "Nite Fields", slug: "nite-fields" },
    });
    addMockRoute("PUT", /\/shows\/668$/, () => ({ id: 668 }));

    await addArtistsToShow("668", [{ name: "Nite Fields" }], TEST_ENV, true);

    const added = putBill()[3];
    expect("set_type" in added).toBe(false);
    expect(added).toEqual({ id: 20, is_headliner: false });
  });

  test("--role states the added act's role and derives is_headliner from it", async () => {
    setupCuratedBillMock();
    setupArtistSearchMock({
      "Nite Fields": { id: 20, name: "Nite Fields", slug: "nite-fields" },
    });
    addMockRoute("PUT", /\/shows\/668$/, () => ({ id: 668 }));

    await addArtistsToShow(
      "668",
      [{ name: "Nite Fields" }],
      TEST_ENV,
      true,
      "special_guest",
    );

    expect(putBill()[3]).toEqual({
      id: 20,
      is_headliner: false,
      set_type: "special_guest",
    });
  });

  test("--role headliner derives is_headliner true", async () => {
    setupShowMock({
      artists: [
        { id: 11, name: "Bosses Band", slug: "bosses-band", is_headliner: false, set_type: "opener" },
      ],
    });
    setupArtistSearchMock({
      "Nite Fields": { id: 20, name: "Nite Fields", slug: "nite-fields" },
    });
    addMockRoute("PUT", /\/shows\/668$/, () => ({ id: 668 }));

    await addArtistsToShow(
      "668",
      [{ name: "Nite Fields" }],
      TEST_ENV,
      true,
      "headliner",
    );

    expect(putBill()[1]).toEqual({
      id: 20,
      is_headliner: true,
      set_type: "headliner",
    });
  });

  test("a per-act set_type outranks --role", async () => {
    setupCuratedBillMock();
    setupArtistSearchMock({
      "Nite Fields": { id: 20, name: "Nite Fields", slug: "nite-fields" },
    });
    addMockRoute("PUT", /\/shows\/668$/, () => ({ id: 668 }));

    await addArtistsToShow(
      "668",
      [{ name: "Nite Fields", set_type: "dj" }],
      TEST_ENV,
      true,
      "opener",
    );

    expect(putBill()[3]).toEqual({
      id: 20,
      is_headliner: false,
      set_type: "dj",
    });
  });

  test("an invalid --role is refused before any request goes out", async () => {
    setupCuratedBillMock();
    setupArtistSearchMock({
      "Nite Fields": { id: 20, name: "Nite Fields", slug: "nite-fields" },
    });
    addMockRoute("PUT", /\/shows\/668$/, () => ({ id: 668 }));

    const results = await addArtistsToShow(
      "668",
      [{ name: "Nite Fields" }],
      TEST_ENV,
      true,
      "support",
    );

    expect(results).toHaveLength(1);
    expect(results[0].action).toBe("error");
    expect(results[0].error).toContain("direct_support");
    expect(fetchCalls).toHaveLength(0);
  });

  test("an invalid per-act set_type is refused before any request goes out", async () => {
    setupCuratedBillMock();

    const results = await addArtistsToShow(
      "668",
      [{ name: "Nite Fields", set_type: "Headliner" }],
      TEST_ENV,
      true,
    );

    expect(results[0].action).toBe("error");
    expect(results[0].error).toContain("headliner");
    expect(fetchCalls).toHaveLength(0);
  });

  test("a 422 from the API surfaces as an error result", async () => {
    setupCuratedBillMock();
    setupArtistSearchMock({
      "Nite Fields": { id: 20, name: "Nite Fields", slug: "nite-fields" },
    });
    addMockRouteWithStatus("PUT", /\/shows\/668$/, 422, () => ({
      title: "Unprocessable Entity",
      detail: "expected value from [headliner direct_support opener special_guest dj performer]",
    }));

    const results = await addArtistsToShow(
      "668",
      [{ name: "Nite Fields" }],
      TEST_ENV,
      true,
    );

    expect(results).toHaveLength(1);
    expect(results[0]).toMatchObject({ name: "Nite Fields", action: "error" });
    expect(results[0].error).toBeDefined();
  });

  test("an unrecognized stored role is dropped rather than failing the edit", async () => {
    setupShowMock({
      artists: [
        { id: 10, name: "Pavement", slug: "pavement", is_headliner: true, set_type: "headliner" },
        { id: 11, name: "Bosses Band", slug: "bosses-band", is_headliner: false, set_type: "co-headliner" },
      ],
    });
    setupArtistSearchMock({
      "Nite Fields": { id: 20, name: "Nite Fields", slug: "nite-fields" },
    });
    addMockRoute("PUT", /\/shows\/668$/, () => ({ id: 668 }));

    await addArtistsToShow("668", [{ name: "Nite Fields" }], TEST_ENV, true);

    const bill = putBill();
    expect("set_type" in bill[1]).toBe(false);
    expect(bill[1]).toEqual({ id: 11, is_headliner: false });
  });

  test("refuses to remove the only act rather than reporting a no-op as done", async () => {
    setupShowMock({
      artists: [
        { id: 10, name: "Pavement", slug: "pavement", is_headliner: true, set_type: "headliner" },
      ],
    });
    addMockRoute("PUT", /\/shows\/668$/, () => ({ id: 668 }));

    const result = await removeArtistFromShow("668", "10", TEST_ENV, true);

    expect(result.action).toBe("error");
    expect(result.error).toContain("last act");
    expect(fetchCalls.filter((c) => c.method === "PUT")).toHaveLength(0);
  });

  test("an added headliner still lands when a kept act already holds the slot", async () => {
    setupCuratedBillMock();
    setupArtistSearchMock({
      "Nite Fields": { id: 20, name: "Nite Fields", slug: "nite-fields" },
    });
    addMockRoute("PUT", /\/shows\/668$/, () => ({ id: 668 }));

    // Two stated headliners is the caller's own statement, so the edit goes
    // through; the collision is reported, not refused.
    await addArtistsToShow(
      "668",
      [{ name: "Nite Fields" }],
      TEST_ENV,
      true,
      "headliner",
    );

    expect(putBill()[3]).toEqual({
      id: 20,
      is_headliner: true,
      set_type: "headliner",
    });
  });

  test("an explicit per-act is_headliner outranks the --role default", async () => {
    setupShowMock({
      artists: [
        { id: 11, name: "Bosses Band", slug: "bosses-band", is_headliner: false, set_type: "opener" },
      ],
    });
    setupArtistSearchMock({
      "Nite Fields": { id: 20, name: "Nite Fields", slug: "nite-fields" },
    });
    addMockRoute("PUT", /\/shows\/668$/, () => ({ id: 668 }));

    await addArtistsToShow(
      "668",
      [{ name: "Nite Fields", is_headliner: true }],
      TEST_ENV,
      true,
      "opener",
    );

    const added = putBill()[1];
    expect("set_type" in added).toBe(false);
    expect(added).toEqual({ id: 20, is_headliner: true });
  });

  test("each act reports its OWN invalid role, not another act's", async () => {
    setupCuratedBillMock();

    const results = await addArtistsToShow(
      "668",
      [
        { name: "Good Act", set_type: "opener" },
        { name: "Bad Act", set_type: "co-headliner" },
      ],
      TEST_ENV,
      true,
    );

    expect(results[0].error).toContain("Bad Act");
    expect(results[1].error).toContain("Bad Act");
    expect(results[1].error).toContain("co-headliner");
    expect(fetchCalls).toHaveLength(0);
  });

  test("sends an artist once when two inputs resolve to the same act", async () => {
    setupShowMock({
      artists: [
        { id: 11, name: "Bosses Band", slug: "bosses-band", is_headliner: false, set_type: "opener" },
      ],
    });
    addMockRoute("GET", /\/artists\/search/, () => ({
      artists: [{ id: 20, name: "Nite Fields", slug: "nite-fields" }],
    }));
    addMockRoute("PUT", /\/shows\/668$/, () => ({ id: 668 }));

    await addArtistsToShow(
      "668",
      [{ name: "Nite Fields" }, { name: "Nite Fields" }],
      TEST_ENV,
      true,
    );

    const bill = putBill();
    expect(bill.filter((a) => a.id === 20)).toHaveLength(1);
  });

  test("a transport failure on the show fetch is an error, not a missing show", async () => {
    addMockRouteWithStatus("GET", /\/shows\/\d+$/, 500, () => ({
      message: "Internal server error",
    }));

    const added = await addArtistsToShow(
      "668",
      [{ name: "Nite Fields" }],
      TEST_ENV,
      true,
    );
    expect(added).toHaveLength(1);
    expect(added[0].action).toBe("error");

    const removed = await removeArtistFromShow("668", "11", TEST_ENV, true);
    expect(removed.action).toBe("error");
  });

  test("a genuine 404 still reports the show as missing", async () => {
    const removed = await removeArtistFromShow("99999", "11", TEST_ENV, true);
    expect(removed.action).toBe("not_found");
  });

  test("remove-artist drops an unroundtrippable role on a kept act", async () => {
    setupShowMock({
      artists: [
        { id: 10, name: "Pavement", slug: "pavement", is_headliner: true, set_type: "headliner" },
        { id: 11, name: "Bosses Band", slug: "bosses-band", is_headliner: false, set_type: "co-headliner" },
        { id: 12, name: "Soapbox Derby", slug: "soapbox-derby", is_headliner: false, set_type: "opener" },
      ],
    });
    addMockRoute("PUT", /\/shows\/668$/, () => ({ id: 668 }));

    await removeArtistFromShow("668", "12", TEST_ENV, true);

    expect(putBill()).toEqual([
      { id: 10, is_headliner: true, set_type: "headliner" },
      { id: 11, is_headliner: false },
    ]);
  });

  test("dry-run sends no PUT for either subcommand", async () => {
    setupCuratedBillMock();
    setupArtistSearchMock({
      "Nite Fields": { id: 20, name: "Nite Fields", slug: "nite-fields" },
    });

    await addArtistsToShow("668", [{ name: "Nite Fields" }], TEST_ENV, false, "opener");
    await removeArtistFromShow("668", "12", TEST_ENV, false);

    expect(fetchCalls.filter((c) => c.method === "PUT")).toHaveLength(0);
  });
});

describe("unroundtrippableActs", () => {
  test("names exactly the acts whose stored role cannot be sent back", () => {
    expect(
      unroundtrippableActs([
        { id: 1, name: "Curated", slug: "curated", set_type: "opener" },
        { id: 2, name: "Silent", slug: "silent", set_type: "performer" },
        { id: 3, name: "Blank", slug: "blank", set_type: "" },
        { id: 4, name: "Legacy", slug: "legacy", set_type: "co-headliner" },
        { id: 5, name: "Absent", slug: "absent" },
      ]).map((a) => a.name),
    ).toEqual(["Legacy"]);
  });
});

describe("headlinerCollisions", () => {
  const curatedBill = [
    { id: 10, name: "Pavement", slug: "pavement", is_headliner: true, set_type: "headliner" },
    { id: 11, name: "Bosses Band", slug: "bosses-band", is_headliner: false, set_type: "opener" },
  ];
  const nite = { id: 20, name: "Nite Fields" };

  test("reports an added headliner against the act already holding the slot", () => {
    expect(
      headlinerCollisions(
        curatedBill,
        [{ input: { name: "Nite Fields" }, resolved: nite, role: "headliner" }],
        new Set(),
      ),
    ).toEqual([{ added: "Nite Fields", existing: "Pavement" }]);
  });

  test("reads the legacy flag as a claim on the slot", () => {
    expect(
      headlinerCollisions(
        curatedBill,
        [
          {
            input: { name: "Nite Fields", is_headliner: true },
            resolved: nite,
            role: undefined,
          },
        ],
        new Set(),
      ),
    ).toHaveLength(1);
  });

  test("a stated role outranks the legacy flag", () => {
    expect(
      headlinerCollisions(
        curatedBill,
        [
          {
            input: { name: "Nite Fields", is_headliner: true },
            resolved: nite,
            role: "opener",
          },
        ],
        new Set(),
      ),
    ).toHaveLength(0);
  });

  test("is silent for an act claiming no headline slot", () => {
    expect(
      headlinerCollisions(
        curatedBill,
        [{ input: { name: "Nite Fields" }, resolved: nite, role: "dj" }],
        new Set(),
      ),
    ).toHaveLength(0);
  });

  test("is silent when the kept bill holds no headliner", () => {
    expect(
      headlinerCollisions(
        [{ id: 11, name: "Bosses Band", slug: "bosses-band", set_type: "opener" }],
        [{ input: { name: "Nite Fields" }, resolved: nite, role: "headliner" }],
        new Set(),
      ),
    ).toHaveLength(0);
  });

  test("is silent for an unresolved or already-linked act", () => {
    expect(
      headlinerCollisions(
        curatedBill,
        [{ input: { name: "Ghost" }, resolved: null, role: "headliner" }],
        new Set(),
      ),
    ).toHaveLength(0);
    expect(
      headlinerCollisions(
        curatedBill,
        [{ input: { name: "Nite Fields" }, resolved: nite, role: "headliner" }],
        new Set([20]),
      ),
    ).toHaveLength(0);
  });
});
