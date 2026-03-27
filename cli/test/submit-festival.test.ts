import { describe, test, expect, mock, beforeEach } from "bun:test";
import {
  submitFestivals,
  parseFestivalInput,
  type FestivalResult,
} from "../src/commands/submit-festival";

// --- Mock fetch for API calls ---

type MockRoute = {
  method: string;
  pattern: RegExp;
  handler: (url: string, body?: unknown) => unknown;
};

let mockRoutes: MockRoute[] = [];
let fetchCalls: { method: string; url: string; body?: unknown }[] = [];

function addMockRoute(
  method: string,
  pattern: RegExp,
  handler: (url: string, body?: unknown) => unknown,
): void {
  mockRoutes.push({ method, pattern, handler });
}

function resetMocks(): void {
  mockRoutes = [];
  fetchCalls = [];
}

// Install global fetch mock
const originalFetch = globalThis.fetch;

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
        const responseBody = route.handler(url, body);
        return new Response(JSON.stringify(responseBody), {
          status: 200,
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

// Restore original fetch after all tests
// (Bun test runner handles this via module isolation)

const TEST_ENV = { url: "http://localhost:8080", token: "phk_test_token" };

// Helper to make a valid festival input
function validFestival(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    name: "M3F Fest 2026",
    series_slug: "m3f",
    edition_year: 2026,
    start_date: "2026-03-06",
    end_date: "2026-03-08",
    city: "Phoenix",
    state: "AZ",
    ...overrides,
  };
}

// Setup mock that returns no festival matches (for create scenarios)
function setupNoMatchMocks(): void {
  // Festival search returns empty (for duplicate check)
  addMockRoute("GET", /\/festivals(\?|$)/, () => ({
    festivals: [],
    count: 0,
  }));
}

// Setup mock that returns an existing festival (for update/skip scenarios)
function setupExistingFestivalMock(
  festival: Record<string, unknown> = {},
): void {
  const existing = {
    id: 42,
    name: "M3F Fest 2026",
    slug: "m3f-fest-2026",
    series_slug: "m3f",
    edition_year: 2026,
    start_date: "2026-03-06",
    end_date: "2026-03-08",
    city: "Phoenix",
    state: "AZ",
    ...festival,
  };

  addMockRoute("GET", /\/festivals(\?|$)/, () => ({
    festivals: [existing],
    count: 1,
  }));
}

describe("parseFestivalInput", () => {
  test("parses single festival object", () => {
    const input = JSON.stringify(validFestival());
    const result = parseFestivalInput(input);
    expect(result).toHaveLength(1);
    expect(result[0].name).toBe("M3F Fest 2026");
  });

  test("parses array of festivals", () => {
    const input = JSON.stringify([
      validFestival(),
      validFestival({ name: "Innings Festival 2026", series_slug: "innings" }),
    ]);
    const result = parseFestivalInput(input);
    expect(result).toHaveLength(2);
    expect(result[0].name).toBe("M3F Fest 2026");
    expect(result[1].name).toBe("Innings Festival 2026");
  });

  test("throws on invalid JSON", () => {
    expect(() => parseFestivalInput("not json")).toThrow();
  });
});

describe("submitFestivals", () => {
  test("creates a single festival in dry-run mode", async () => {
    setupNoMatchMocks();

    const festivals = [validFestival()] as any[];
    const results = await submitFestivals(festivals, TEST_ENV, false);

    expect(results).toHaveLength(0);

    // Should not have called POST /festivals
    const postCalls = fetchCalls.filter(
      (c) => c.method === "POST" && /\/festivals$/.test(c.url),
    );
    expect(postCalls).toHaveLength(0);
  });

  test("creates a single festival with --confirm", async () => {
    setupNoMatchMocks();

    addMockRoute("POST", /\/festivals$/, (_url, body) => ({
      id: 99,
      name: (body as Record<string, unknown>).name,
      slug: "m3f-fest-2026",
    }));

    const festivals = [validFestival()] as any[];
    const results = await submitFestivals(festivals, TEST_ENV, true);

    expect(results).toHaveLength(1);
    expect(results[0].action).toBe("created");
    expect(results[0].id).toBe(99);
    expect(results[0].name).toBe("M3F Fest 2026");

    // Verify POST was called
    const postCalls = fetchCalls.filter(
      (c) => c.method === "POST" && /\/festivals$/.test(c.url),
    );
    expect(postCalls).toHaveLength(1);
    expect(postCalls[0].body).toMatchObject({
      name: "M3F Fest 2026",
      series_slug: "m3f",
      edition_year: 2026,
    });
  });

  test("creates a festival with artist lineup resolved by search", async () => {
    setupNoMatchMocks();

    // POST /festivals creates the festival
    addMockRoute("POST", /\/festivals$/, () => ({
      id: 100,
      name: "M3F Fest 2026",
      slug: "m3f-fest-2026",
    }));

    // GET /artists/search resolves artist names to IDs
    addMockRoute("GET", /\/artists\/search/, (url) => {
      const urlObj = new URL(url);
      const q = urlObj.searchParams.get("q") || "";
      if (q.toLowerCase().includes("khruangbin")) {
        return {
          artists: [{ id: 10, name: "Khruangbin", slug: "khruangbin" }],
        };
      }
      if (q.toLowerCase().includes("japanese breakfast")) {
        return {
          artists: [
            {
              id: 20,
              name: "Japanese Breakfast",
              slug: "japanese-breakfast",
            },
          ],
        };
      }
      return { artists: [] };
    });

    // POST /festivals/{id}/artists links artists
    addMockRoute("POST", /\/festivals\/\d+\/artists$/, () => ({
      id: 1,
    }));

    const festivals = [
      validFestival({
        artists: [
          { name: "Khruangbin", billing_tier: "headliner" },
          { name: "Japanese Breakfast", billing_tier: "sub_headliner" },
        ],
      }),
    ] as any[];

    const results = await submitFestivals(festivals, TEST_ENV, true);

    expect(results).toHaveLength(1);
    expect(results[0].action).toBe("created");
    expect(results[0].artistResults).toHaveLength(2);
    expect(results[0].artistResults![0]).toMatchObject({
      name: "Khruangbin",
      action: "linked",
      artistId: 10,
    });
    expect(results[0].artistResults![1]).toMatchObject({
      name: "Japanese Breakfast",
      action: "linked",
      artistId: 20,
    });

    // Verify artist link calls
    const artistLinkCalls = fetchCalls.filter(
      (c) => c.method === "POST" && /\/festivals\/\d+\/artists$/.test(c.url),
    );
    expect(artistLinkCalls).toHaveLength(2);
    expect(artistLinkCalls[0].body).toMatchObject({
      artist_id: 10,
      billing_tier: "headliner",
    });
    expect(artistLinkCalls[1].body).toMatchObject({
      artist_id: 20,
      billing_tier: "sub_headliner",
    });
  });

  test("updates a festival when new info is available", async () => {
    // Existing festival has no website
    setupExistingFestivalMock({ website: "" });

    // PUT /festivals/{id} updates the festival
    addMockRoute("PUT", /\/festivals\/\d+$/, () => ({
      id: 42,
      name: "M3F Fest 2026",
    }));

    const festivals = [
      validFestival({ website: "https://m3ffest.com" }),
    ] as any[];

    const results = await submitFestivals(festivals, TEST_ENV, true);

    expect(results).toHaveLength(1);
    expect(results[0].action).toBe("updated");
    expect(results[0].id).toBe(42);

    // Verify PUT was called with only the new field
    const putCalls = fetchCalls.filter(
      (c) => c.method === "PUT" && /\/festivals\/42$/.test(c.url),
    );
    expect(putCalls).toHaveLength(1);
    expect(putCalls[0].body).toMatchObject({ website: "https://m3ffest.com" });
  });

  test("skips a festival when no new info (no artists)", async () => {
    setupExistingFestivalMock();

    const festivals = [validFestival()] as any[];
    const results = await submitFestivals(festivals, TEST_ENV, true);

    expect(results).toHaveLength(1);
    expect(results[0].action).toBe("skipped");
    expect(results[0].id).toBe(42);
    expect(results[0].artistResults).toEqual([]);
    expect(results[0].venueResults).toEqual([]);

    // Verify no PUT or POST was called (no artists/venues to link)
    const mutationCalls = fetchCalls.filter(
      (c) => c.method === "PUT" || (c.method === "POST" && /\/festivals/.test(c.url)),
    );
    expect(mutationCalls).toHaveLength(0);
  });

  test("reports validation error for missing required fields", async () => {
    const festivals = [
      { name: "Bad Festival" }, // Missing series_slug, edition_year, start_date, end_date
    ] as any[];

    const results = await submitFestivals(festivals, TEST_ENV, true);

    expect(results).toHaveLength(1);
    expect(results[0].action).toBe("error");
    expect(results[0].error).toContain("series_slug");
    expect(results[0].error).toContain("edition_year");
  });

  test("reports validation error for invalid billing tier", async () => {
    const festivals = [
      validFestival({
        artists: [{ name: "SomeArtist", billing_tier: "mega_star" }],
      }),
    ] as any[];

    const results = await submitFestivals(festivals, TEST_ENV, true);

    expect(results).toHaveLength(1);
    expect(results[0].action).toBe("error");
    expect(results[0].error).toContain("Invalid billing tier");
  });

  test("handles artist not found gracefully", async () => {
    setupNoMatchMocks();

    addMockRoute("POST", /\/festivals$/, () => ({
      id: 101,
      name: "Test Fest",
    }));

    // Artist search returns empty
    addMockRoute("GET", /\/artists\/search/, () => ({
      artists: [],
    }));

    const festivals = [
      validFestival({
        name: "Test Fest",
        artists: [{ name: "Unknown Band", billing_tier: "undercard" }],
      }),
    ] as any[];

    const results = await submitFestivals(festivals, TEST_ENV, true);

    expect(results).toHaveLength(1);
    expect(results[0].action).toBe("created");
    expect(results[0].artistResults).toHaveLength(1);
    expect(results[0].artistResults![0]).toMatchObject({
      name: "Unknown Band",
      action: "not_found",
    });
  });

  test("creates festival with venues resolved by search", async () => {
    setupNoMatchMocks();

    addMockRoute("POST", /\/festivals$/, () => ({
      id: 102,
      name: "Multi Venue Fest",
    }));

    addMockRoute("GET", /\/venues\/search/, (url) => {
      const urlObj = new URL(url);
      const q = urlObj.searchParams.get("q") || "";
      if (q.toLowerCase().includes("hance park")) {
        return {
          venues: [{ id: 5, name: "Margaret T. Hance Park", slug: "hance-park" }],
        };
      }
      return { venues: [] };
    });

    addMockRoute("POST", /\/festivals\/\d+\/venues$/, () => ({
      id: 1,
    }));

    const festivals = [
      validFestival({
        name: "Multi Venue Fest",
        venues: [{ name: "Hance Park", is_primary: true }],
      }),
    ] as any[];

    const results = await submitFestivals(festivals, TEST_ENV, true);

    expect(results).toHaveLength(1);
    expect(results[0].action).toBe("created");
    expect(results[0].venueResults).toHaveLength(1);
    expect(results[0].venueResults![0]).toMatchObject({
      name: "Hance Park",
      action: "linked",
      venueId: 5,
    });

    const venueLinkCalls = fetchCalls.filter(
      (c) => c.method === "POST" && /\/festivals\/\d+\/venues$/.test(c.url),
    );
    expect(venueLinkCalls).toHaveLength(1);
    expect(venueLinkCalls[0].body).toMatchObject({
      venue_id: 5,
      is_primary: true,
    });
  });

  test("dry-run does not make mutation API calls", async () => {
    setupNoMatchMocks();

    const festivals = [
      validFestival({
        artists: [{ name: "SomeArtist", billing_tier: "headliner" }],
      }),
    ] as any[];

    const results = await submitFestivals(festivals, TEST_ENV, false);

    // Dry-run should produce no results (only skips are added in dry-run)
    // Creates/updates only happen with --confirm
    expect(results).toHaveLength(0);

    // No POST or PUT calls (only GET for duplicate check)
    const mutationCalls = fetchCalls.filter(
      (c) => c.method === "POST" || c.method === "PUT",
    );
    expect(mutationCalls).toHaveLength(0);
  });

  test("skipped festival still links artists and venues", async () => {
    setupExistingFestivalMock();

    // GET /artists/search resolves artist names
    addMockRoute("GET", /\/artists\/search/, (url) => {
      const urlObj = new URL(url);
      const q = urlObj.searchParams.get("q") || "";
      if (q.toLowerCase().includes("khruangbin")) {
        return {
          artists: [{ id: 10, name: "Khruangbin", slug: "khruangbin" }],
        };
      }
      if (q.toLowerCase().includes("japanese breakfast")) {
        return {
          artists: [
            { id: 20, name: "Japanese Breakfast", slug: "japanese-breakfast" },
          ],
        };
      }
      return { artists: [] };
    });

    // GET /venues/search resolves venue names
    addMockRoute("GET", /\/venues\/search/, (url) => {
      const urlObj = new URL(url);
      const q = urlObj.searchParams.get("q") || "";
      if (q.toLowerCase().includes("hance park")) {
        return {
          venues: [{ id: 5, name: "Margaret T. Hance Park", slug: "hance-park" }],
        };
      }
      return { venues: [] };
    });

    // POST /festivals/{id}/artists links artists
    addMockRoute("POST", /\/festivals\/\d+\/artists$/, () => ({ id: 1 }));

    // POST /festivals/{id}/venues links venues
    addMockRoute("POST", /\/festivals\/\d+\/venues$/, () => ({ id: 1 }));

    const festivals = [
      validFestival({
        artists: [
          { name: "Khruangbin", billing_tier: "headliner" },
          { name: "Japanese Breakfast", billing_tier: "sub_headliner" },
          { name: "Unknown Band", billing_tier: "undercard" },
        ],
        venues: [{ name: "Hance Park", is_primary: true }],
      }),
    ] as any[];

    const results = await submitFestivals(festivals, TEST_ENV, true);

    expect(results).toHaveLength(1);
    expect(results[0].action).toBe("skipped");
    expect(results[0].id).toBe(42);

    // Artists should be processed
    expect(results[0].artistResults).toHaveLength(3);
    expect(results[0].artistResults![0]).toMatchObject({
      name: "Khruangbin",
      action: "linked",
      artistId: 10,
    });
    expect(results[0].artistResults![1]).toMatchObject({
      name: "Japanese Breakfast",
      action: "linked",
      artistId: 20,
    });
    expect(results[0].artistResults![2]).toMatchObject({
      name: "Unknown Band",
      action: "not_found",
    });

    // Venues should be processed
    expect(results[0].venueResults).toHaveLength(1);
    expect(results[0].venueResults![0]).toMatchObject({
      name: "Hance Park",
      action: "linked",
      venueId: 5,
    });

    // Verify artist link API calls were made
    const artistLinkCalls = fetchCalls.filter(
      (c) => c.method === "POST" && /\/festivals\/42\/artists$/.test(c.url),
    );
    expect(artistLinkCalls).toHaveLength(2); // Only 2 — Unknown Band was not found

    // Verify venue link API calls were made
    const venueLinkCalls = fetchCalls.filter(
      (c) => c.method === "POST" && /\/festivals\/42\/venues$/.test(c.url),
    );
    expect(venueLinkCalls).toHaveLength(1);
  });

  test("already-linked artists are handled gracefully (409)", async () => {
    setupExistingFestivalMock();

    // Artist search resolves the artist
    addMockRoute("GET", /\/artists\/search/, () => ({
      artists: [{ id: 10, name: "Khruangbin", slug: "khruangbin" }],
    }));

    // POST /festivals/{id}/artists returns 409 Conflict (already linked)
    mockRoutes.push({
      method: "POST",
      pattern: /\/festivals\/\d+\/artists$/,
      handler: () => {
        // This handler won't be used because we override fetch for 409
        return {};
      },
    });

    // Override: we need the POST to /festivals/{id}/artists to return 409
    // Remove the generic mock we just added and handle it in fetch directly
    mockRoutes.pop();

    const savedFetch = globalThis.fetch;
    globalThis.fetch = (async (
      input: string | URL | Request,
      init?: RequestInit,
    ) => {
      const url = typeof input === "string" ? input : input.toString();
      const method = init?.method || "GET";
      const body = init?.body ? JSON.parse(init.body as string) : undefined;

      fetchCalls.push({ method, url, body });

      // Return 409 for artist linking
      if (method === "POST" && /\/festivals\/\d+\/artists$/.test(url)) {
        return new Response(
          JSON.stringify({ message: "Artist already linked to festival" }),
          { status: 409, headers: { "Content-Type": "application/json" } },
        );
      }

      // Fall through to normal mock routes
      for (const route of mockRoutes) {
        if (route.method === method && route.pattern.test(url)) {
          const responseBody = route.handler(url, body);
          return new Response(JSON.stringify(responseBody), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          });
        }
      }

      return new Response(
        JSON.stringify({ message: "Not found" }),
        { status: 404 },
      );
    }) as typeof fetch;

    const festivals = [
      validFestival({
        artists: [{ name: "Khruangbin", billing_tier: "headliner" }],
      }),
    ] as any[];

    const results = await submitFestivals(festivals, TEST_ENV, true);

    expect(results).toHaveLength(1);
    expect(results[0].action).toBe("skipped");
    expect(results[0].artistResults).toHaveLength(1);
    expect(results[0].artistResults![0]).toMatchObject({
      name: "Khruangbin",
      action: "already_linked",
      artistId: 10,
    });

    // Restore fetch
    globalThis.fetch = savedFetch;
  });

  test("skipped festival without artists/venues makes no mutation calls", async () => {
    setupExistingFestivalMock();

    const festivals = [validFestival()] as any[];
    const results = await submitFestivals(festivals, TEST_ENV, true);

    expect(results).toHaveLength(1);
    expect(results[0].action).toBe("skipped");
    expect(results[0].id).toBe(42);
    expect(results[0].artistResults).toEqual([]);
    expect(results[0].venueResults).toEqual([]);

    // No POST or PUT calls (only GET for duplicate check)
    const mutationCalls = fetchCalls.filter(
      (c) => c.method === "POST" || c.method === "PUT",
    );
    expect(mutationCalls).toHaveLength(0);
  });

  test("processes multiple festivals in one batch", async () => {
    setupNoMatchMocks();

    addMockRoute("POST", /\/festivals$/, (_url, body) => {
      const name = (body as Record<string, unknown>).name;
      return {
        id: name === "Fest A" ? 200 : 201,
        name,
      };
    });

    const festivals = [
      validFestival({ name: "Fest A", series_slug: "fest-a" }),
      validFestival({ name: "Fest B", series_slug: "fest-b" }),
    ] as any[];

    const results = await submitFestivals(festivals, TEST_ENV, true);

    expect(results).toHaveLength(2);
    expect(results[0].action).toBe("created");
    expect(results[0].name).toBe("Fest A");
    expect(results[1].action).toBe("created");
    expect(results[1].name).toBe("Fest B");
  });
});
