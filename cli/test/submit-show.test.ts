import { describe, test, expect, mock, beforeEach } from "bun:test";
import {
  parseShowInput,
  resolveArtists,
  resolveVenues,
  buildShowPayload,
  submitShows,
  type ShowPlan,
} from "../src/commands/submit-show";
import { APIClient } from "../src/lib/api";
import { checkShowDuplicate } from "../src/lib/duplicates";

// -- Mock helpers ------------------------------------------------------------

function createMockClient(overrides: {
  get?: (path: string, params?: Record<string, string>) => Promise<unknown>;
  post?: (path: string, body?: unknown) => Promise<unknown>;
} = {}): APIClient {
  const client = Object.create(APIClient.prototype) as APIClient;

  if (overrides.get) {
    (client as unknown as Record<string, unknown>).get = overrides.get;
  } else {
    (client as unknown as Record<string, unknown>).get = async () => ({});
  }

  if (overrides.post) {
    (client as unknown as Record<string, unknown>).post = overrides.post;
  } else {
    (client as unknown as Record<string, unknown>).post = async () => ({ id: 1 });
  }

  return client;
}

// -- parseShowInput ----------------------------------------------------------

describe("parseShowInput", () => {
  test("parses a single show object", () => {
    const input = JSON.stringify({
      event_date: "2026-04-15",
      city: "Phoenix",
      state: "AZ",
      artists: [{ name: "Nina Hagen" }],
      venues: [{ name: "Crescent Ballroom", city: "Phoenix", state: "AZ" }],
    });

    const result = parseShowInput(input);
    expect(result).toHaveLength(1);
    expect(result[0].event_date).toBe("2026-04-15");
    expect(result[0].artists[0].name).toBe("Nina Hagen");
  });

  test("parses an array of shows", () => {
    const input = JSON.stringify([
      {
        event_date: "2026-04-15",
        city: "Phoenix",
        state: "AZ",
        artists: [{ name: "Nina Hagen" }],
        venues: [{ name: "Crescent Ballroom", city: "Phoenix", state: "AZ" }],
      },
      {
        event_date: "2026-04-16",
        city: "Tucson",
        state: "AZ",
        artists: [{ name: "Nina Hagen" }],
        venues: [{ name: "191 Toole", city: "Tucson", state: "AZ" }],
      },
    ]);

    const result = parseShowInput(input);
    expect(result).toHaveLength(2);
    expect(result[0].city).toBe("Phoenix");
    expect(result[1].city).toBe("Tucson");
  });

  test("throws on invalid JSON", () => {
    expect(() => parseShowInput("not json")).toThrow("Invalid JSON input");
  });
});

// -- resolveArtists ----------------------------------------------------------

describe("resolveArtists", () => {
  test("marks artist as existing when found in search", async () => {
    const client = createMockClient({
      get: async (path: string) => {
        if (path.includes("/artists/search")) {
          return {
            artists: [{ id: 42, name: "Nina Hagen", slug: "nina-hagen" }],
          };
        }
        return {};
      },
    });

    const result = await resolveArtists(client, [{ name: "Nina Hagen" }]);
    expect(result).toHaveLength(1);
    expect(result[0].status).toBe("existing");
    expect(result[0].id).toBe(42);
    expect(result[0].name).toBe("Nina Hagen");
  });

  test("marks artist as new when not found in search", async () => {
    const client = createMockClient({
      get: async () => ({ artists: [] }),
    });

    const result = await resolveArtists(client, [{ name: "Unknown Band" }]);
    expect(result).toHaveLength(1);
    expect(result[0].status).toBe("new");
    expect(result[0].name).toBe("Unknown Band");
    expect(result[0].id).toBeUndefined();
  });

  test("preserves is_headliner flag", async () => {
    const client = createMockClient({
      get: async () => ({
        artists: [{ id: 1, name: "Headliner", slug: "headliner" }],
      }),
    });

    const result = await resolveArtists(client, [
      { name: "Headliner", is_headliner: true },
    ]);
    expect(result[0].is_headliner).toBe(true);
  });

  test("treats search failure as new artist", async () => {
    const client = createMockClient({
      get: async () => { throw new Error("Network error"); },
    });

    const result = await resolveArtists(client, [{ name: "Test" }]);
    expect(result).toHaveLength(1);
    expect(result[0].status).toBe("new");
  });
});

// -- resolveVenues -----------------------------------------------------------

describe("resolveVenues", () => {
  test("marks venue as existing when found in search", async () => {
    const client = createMockClient({
      get: async (path: string) => {
        if (path.includes("/venues/search")) {
          return {
            venues: [
              { id: 10, name: "Crescent Ballroom", slug: "crescent-ballroom", city: "Phoenix", state: "AZ" },
            ],
          };
        }
        return {};
      },
    });

    const result = await resolveVenues(client, [
      { name: "Crescent Ballroom", city: "Phoenix", state: "AZ" },
    ]);
    expect(result).toHaveLength(1);
    expect(result[0].status).toBe("existing");
    expect(result[0].id).toBe(10);
  });

  test("marks venue as new when not found", async () => {
    const client = createMockClient({
      get: async () => ({ venues: [] }),
    });

    const result = await resolveVenues(client, [
      { name: "New Venue", city: "Phoenix", state: "AZ" },
    ]);
    expect(result).toHaveLength(1);
    expect(result[0].status).toBe("new");
    expect(result[0].name).toBe("New Venue");
  });

  test("treats search failure as new venue", async () => {
    const client = createMockClient({
      get: async () => { throw new Error("Network error"); },
    });

    const result = await resolveVenues(client, [
      { name: "Test Venue", city: "Phoenix", state: "AZ" },
    ]);
    expect(result).toHaveLength(1);
    expect(result[0].status).toBe("new");
  });
});

// -- buildShowPayload --------------------------------------------------------

describe("buildShowPayload", () => {
  test("builds payload with existing artist ID", () => {
    const plan: ShowPlan = {
      input: {
        event_date: "2026-04-15",
        city: "Phoenix",
        state: "AZ",
        artists: [{ name: "Nina Hagen" }],
        venues: [{ name: "Crescent Ballroom", city: "Phoenix", state: "AZ" }],
      },
      artists: [{ id: 42, name: "Nina Hagen", status: "existing" }],
      venues: [{ id: 10, name: "Crescent Ballroom", status: "existing" }],
      valid: true,
      errors: [],
    };

    const payload = buildShowPayload(plan);
    expect(payload.event_date).toBe("2026-04-15T20:00:00Z");
    expect(payload.city).toBe("Phoenix");

    const artists = payload.artists as Array<Record<string, unknown>>;
    expect(artists[0].id).toBe(42);
    expect(artists[0].name).toBeUndefined();

    const venues = payload.venues as Array<Record<string, unknown>>;
    expect(venues[0].id).toBe(10);
    expect(venues[0].name).toBeUndefined();
  });

  test("builds payload with new artist name", () => {
    const plan: ShowPlan = {
      input: {
        event_date: "2026-04-15",
        city: "Phoenix",
        state: "AZ",
        artists: [{ name: "New Band" }],
        venues: [{ name: "New Venue", city: "Phoenix", state: "AZ" }],
      },
      artists: [{ name: "New Band", status: "new" }],
      venues: [{ name: "New Venue", city: "Phoenix", state: "AZ", status: "new" }],
      valid: true,
      errors: [],
    };

    const payload = buildShowPayload(plan);
    const artists = payload.artists as Array<Record<string, unknown>>;
    expect(artists[0].name).toBe("New Band");
    expect(artists[0].id).toBeUndefined();

    const venues = payload.venues as Array<Record<string, unknown>>;
    expect(venues[0].name).toBe("New Venue");
    expect(venues[0].city).toBe("Phoenix");
    expect(venues[0].state).toBe("AZ");
  });

  test("includes optional fields when provided", () => {
    const plan: ShowPlan = {
      input: {
        event_date: "2026-04-15",
        city: "Phoenix",
        state: "AZ",
        title: "Special Show",
        price: 25,
        age_requirement: "21+",
        description: "A great show",
        artists: [{ name: "Test" }],
        venues: [{ name: "Test Venue" }],
      },
      artists: [{ name: "Test", status: "new" }],
      venues: [{ name: "Test Venue", status: "new" }],
      valid: true,
      errors: [],
    };

    const payload = buildShowPayload(plan);
    expect(payload.title).toBe("Special Show");
    expect(payload.price).toBe(25);
    expect(payload.age_requirement).toBe("21+");
    expect(payload.description).toBe("A great show");
  });
});

// -- submitShows (integration) -----------------------------------------------

describe("submitShows", () => {
  test("single show with existing artist and venue (resolved by search)", async () => {
    const getMock = async (path: string) => {
      if (path.includes("/artists/search")) {
        return { artists: [{ id: 42, name: "Nina Hagen", slug: "nina-hagen" }] };
      }
      if (path.includes("/venues/search")) {
        return {
          venues: [
            { id: 10, name: "Crescent Ballroom", slug: "crescent-ballroom", city: "Phoenix", state: "AZ" },
          ],
        };
      }
      return {};
    };

    const postMock = async (_path: string, body?: unknown) => {
      return { id: 100, slug: "2026-04-15-crescent-ballroom" };
    };

    const client = createMockClient({ get: getMock, post: postMock });

    const json = JSON.stringify({
      event_date: "2026-04-15",
      city: "Phoenix",
      state: "AZ",
      artists: [{ name: "Nina Hagen" }],
      venues: [{ name: "Crescent Ballroom", city: "Phoenix", state: "AZ" }],
    });

    const result = await submitShows(client, json, true);
    expect(result.created).toBe(1);
    expect(result.failed).toBe(0);
    expect(result.plans[0].artists[0].status).toBe("existing");
    expect(result.plans[0].artists[0].id).toBe(42);
    expect(result.plans[0].venues[0].status).toBe("existing");
    expect(result.plans[0].venues[0].id).toBe(10);
  });

  test("show with new artist (not found in search)", async () => {
    const getMock = async (path: string) => {
      if (path.includes("/artists/search")) {
        return { artists: [] }; // Not found
      }
      if (path.includes("/venues/search")) {
        return {
          venues: [
            { id: 10, name: "Crescent Ballroom", slug: "crescent-ballroom", city: "Phoenix", state: "AZ" },
          ],
        };
      }
      return {};
    };

    const client = createMockClient({
      get: getMock,
      post: async () => ({ id: 101 }),
    });

    const json = JSON.stringify({
      event_date: "2026-04-15",
      city: "Phoenix",
      state: "AZ",
      artists: [{ name: "Brand New Band" }],
      venues: [{ name: "Crescent Ballroom", city: "Phoenix", state: "AZ" }],
    });

    const result = await submitShows(client, json, true);
    expect(result.created).toBe(1);
    expect(result.plans[0].artists[0].status).toBe("new");
    expect(result.plans[0].artists[0].name).toBe("Brand New Band");
  });

  test("tour announcement: array of shows with shared artist, different venues", async () => {
    const getMock = async (path: string) => {
      if (path.includes("/artists/search")) {
        return { artists: [{ id: 42, name: "Nina Hagen", slug: "nina-hagen" }] };
      }
      if (path.includes("/venues/search")) {
        return { venues: [] }; // All venues new for simplicity
      }
      return {};
    };

    let postCount = 0;
    const client = createMockClient({
      get: getMock,
      post: async () => ({ id: 200 + ++postCount }),
    });

    const json = JSON.stringify([
      {
        event_date: "2026-04-15",
        city: "Phoenix",
        state: "AZ",
        artists: [{ name: "Nina Hagen" }],
        venues: [{ name: "Crescent Ballroom", city: "Phoenix", state: "AZ" }],
      },
      {
        event_date: "2026-04-16",
        city: "Tucson",
        state: "AZ",
        artists: [{ name: "Nina Hagen" }],
        venues: [{ name: "191 Toole", city: "Tucson", state: "AZ" }],
      },
      {
        event_date: "2026-04-17",
        city: "Flagstaff",
        state: "AZ",
        artists: [{ name: "Nina Hagen" }],
        venues: [{ name: "The Orpheum", city: "Flagstaff", state: "AZ" }],
      },
    ]);

    const result = await submitShows(client, json, true);
    expect(result.created).toBe(3);
    expect(result.failed).toBe(0);
    expect(result.plans).toHaveLength(3);
    // All share same artist resolved as existing
    for (const plan of result.plans) {
      expect(plan.artists[0].status).toBe("existing");
      expect(plan.artists[0].id).toBe(42);
    }
  });

  test("validation error: missing event_date", async () => {
    const client = createMockClient();

    const json = JSON.stringify({
      city: "Phoenix",
      state: "AZ",
      artists: [{ name: "Test" }],
      venues: [{ name: "Test Venue" }],
    });

    const result = await submitShows(client, json, true);
    expect(result.created).toBe(0);
    expect(result.plans[0].valid).toBe(false);
    expect(result.plans[0].errors.some((e) => e.includes("event_date"))).toBe(true);
  });

  test("dry-run mode: does not call POST", async () => {
    let postCalled = false;
    const client = createMockClient({
      get: async () => ({ artists: [], venues: [] }),
      post: async () => {
        postCalled = true;
        return { id: 1 };
      },
    });

    const json = JSON.stringify({
      event_date: "2026-04-15",
      city: "Phoenix",
      state: "AZ",
      artists: [{ name: "Test" }],
      venues: [{ name: "Test Venue" }],
    });

    const result = await submitShows(client, json, false); // dry-run
    expect(result.created).toBe(0);
    expect(result.skipped).toBe(1);
    expect(postCalled).toBe(false);
  });

  test("confirm mode: calls POST and reports success", async () => {
    let postCalled = false;
    const client = createMockClient({
      get: async () => ({ artists: [], venues: [] }),
      post: async () => {
        postCalled = true;
        return { id: 99 };
      },
    });

    const json = JSON.stringify({
      event_date: "2026-04-15",
      city: "Phoenix",
      state: "AZ",
      artists: [{ name: "Test" }],
      venues: [{ name: "Test Venue" }],
    });

    const result = await submitShows(client, json, true); // confirm
    expect(result.created).toBe(1);
    expect(postCalled).toBe(true);
  });

  test("handles API error during creation", async () => {
    const client = createMockClient({
      get: async () => ({ artists: [], venues: [] }),
      post: async () => { throw new Error("Server error"); },
    });

    const json = JSON.stringify({
      event_date: "2026-04-15",
      city: "Phoenix",
      state: "AZ",
      artists: [{ name: "Test" }],
      venues: [{ name: "Test Venue" }],
    });

    const result = await submitShows(client, json, true);
    expect(result.created).toBe(0);
    expect(result.failed).toBe(1);
  });

  test("mixed valid and invalid shows in array", async () => {
    const client = createMockClient({
      get: async () => ({ artists: [], venues: [] }),
      post: async () => ({ id: 1 }),
    });

    const json = JSON.stringify([
      {
        event_date: "2026-04-15",
        city: "Phoenix",
        state: "AZ",
        artists: [{ name: "Test" }],
        venues: [{ name: "Test Venue" }],
      },
      {
        // Missing event_date, city, state
        artists: [{ name: "Test" }],
        venues: [{ name: "Test Venue" }],
      },
    ]);

    const result = await submitShows(client, json, true);
    expect(result.created).toBe(1);
    expect(result.plans[0].valid).toBe(true);
    expect(result.plans[1].valid).toBe(false);
  });
});

// -- checkShowDuplicate ------------------------------------------------------

describe("checkShowDuplicate", () => {
  test("returns no match when no venue IDs provided", async () => {
    const client = createMockClient();
    const result = await checkShowDuplicate(client, "2026-04-15", [], [42], ["Nina Hagen"]);
    expect(result.isDuplicate).toBe(false);
  });

  test("returns no match when no artist IDs or names provided", async () => {
    const client = createMockClient();
    const result = await checkShowDuplicate(client, "2026-04-15", [10], [], []);
    expect(result.isDuplicate).toBe(false);
  });

  test("detects duplicate by matching venue ID and artist ID", async () => {
    const client = createMockClient({
      get: async (path: string) => {
        if (path.includes("/shows")) {
          return [
            {
              id: 500,
              slug: "2026-04-15-crescent-ballroom",
              event_date: "2026-04-15T20:00:00Z",
              venues: [{ id: 10, name: "Crescent Ballroom" }],
              artists: [{ id: 42, name: "Nina Hagen" }],
            },
          ];
        }
        return {};
      },
    });

    const result = await checkShowDuplicate(client, "2026-04-15", [10], [42], ["Nina Hagen"]);
    expect(result.isDuplicate).toBe(true);
    expect(result.existingShowId).toBe(500);
    expect(result.existingShowSlug).toBe("2026-04-15-crescent-ballroom");
  });

  test("detects duplicate by matching venue ID and artist name (fuzzy)", async () => {
    const client = createMockClient({
      get: async (path: string) => {
        if (path.includes("/shows")) {
          return [
            {
              id: 501,
              slug: "2026-04-15-crescent-ballroom",
              event_date: "2026-04-15T20:00:00Z",
              venues: [{ id: 10, name: "Crescent Ballroom" }],
              artists: [{ id: 99, name: "Nina Hagen" }],
            },
          ];
        }
        return {};
      },
    });

    // Artist IDs don't match (different ID), but names match
    const result = await checkShowDuplicate(client, "2026-04-15", [10], [200], ["Nina Hagen"]);
    expect(result.isDuplicate).toBe(true);
    expect(result.existingShowId).toBe(501);
  });

  test("returns no match when venue does not match", async () => {
    const client = createMockClient({
      get: async (path: string) => {
        if (path.includes("/shows")) {
          return [
            {
              id: 502,
              event_date: "2026-04-15T20:00:00Z",
              venues: [{ id: 99, name: "Different Venue" }],
              artists: [{ id: 42, name: "Nina Hagen" }],
            },
          ];
        }
        return {};
      },
    });

    const result = await checkShowDuplicate(client, "2026-04-15", [10], [42], ["Nina Hagen"]);
    expect(result.isDuplicate).toBe(false);
  });

  test("returns no match when artist does not match", async () => {
    const client = createMockClient({
      get: async (path: string) => {
        if (path.includes("/shows")) {
          return [
            {
              id: 503,
              event_date: "2026-04-15T20:00:00Z",
              venues: [{ id: 10, name: "Crescent Ballroom" }],
              artists: [{ id: 99, name: "Totally Different Band" }],
            },
          ];
        }
        return {};
      },
    });

    const result = await checkShowDuplicate(client, "2026-04-15", [10], [42], ["Nina Hagen"]);
    expect(result.isDuplicate).toBe(false);
  });

  test("returns no match when no shows exist on that date", async () => {
    const client = createMockClient({
      get: async (path: string) => {
        if (path.includes("/shows")) {
          return [];
        }
        return {};
      },
    });

    const result = await checkShowDuplicate(client, "2026-04-15", [10], [42], ["Nina Hagen"]);
    expect(result.isDuplicate).toBe(false);
  });

  test("returns no match when API call fails", async () => {
    const client = createMockClient({
      get: async () => { throw new Error("Network error"); },
    });

    const result = await checkShowDuplicate(client, "2026-04-15", [10], [42], ["Nina Hagen"]);
    expect(result.isDuplicate).toBe(false);
  });

  test("handles full ISO date strings", async () => {
    const client = createMockClient({
      get: async (path: string) => {
        if (path.includes("/shows")) {
          return [
            {
              id: 504,
              event_date: "2026-04-15T20:00:00Z",
              venues: [{ id: 10, name: "Crescent Ballroom" }],
              artists: [{ id: 42, name: "Nina Hagen" }],
            },
          ];
        }
        return {};
      },
    });

    const result = await checkShowDuplicate(client, "2026-04-15T20:00:00Z", [10], [42], ["Nina Hagen"]);
    expect(result.isDuplicate).toBe(true);
    expect(result.existingShowId).toBe(504);
  });
});

// -- submitShows with deduplication ------------------------------------------

describe("submitShows deduplication", () => {
  test("skips duplicate show in confirm mode", async () => {
    let postCalled = false;
    const getMock = async (path: string) => {
      if (path.includes("/artists/search")) {
        return { artists: [{ id: 42, name: "Nina Hagen", slug: "nina-hagen" }] };
      }
      if (path.includes("/venues/search")) {
        return {
          venues: [
            { id: 10, name: "Crescent Ballroom", slug: "crescent-ballroom", city: "Phoenix", state: "AZ" },
          ],
        };
      }
      if (path.includes("/shows")) {
        return [
          {
            id: 500,
            slug: "2026-04-15-crescent-ballroom",
            event_date: "2026-04-15T20:00:00Z",
            venues: [{ id: 10, name: "Crescent Ballroom" }],
            artists: [{ id: 42, name: "Nina Hagen" }],
          },
        ];
      }
      return {};
    };

    const client = createMockClient({
      get: getMock,
      post: async () => {
        postCalled = true;
        return { id: 999 };
      },
    });

    const json = JSON.stringify({
      event_date: "2026-04-15",
      city: "Phoenix",
      state: "AZ",
      artists: [{ name: "Nina Hagen" }],
      venues: [{ name: "Crescent Ballroom", city: "Phoenix", state: "AZ" }],
    });

    const result = await submitShows(client, json, true);
    expect(result.created).toBe(0);
    expect(result.skipped).toBe(1);
    expect(result.plans[0].duplicate?.isDuplicate).toBe(true);
    expect(result.plans[0].duplicate?.existingShowId).toBe(500);
    expect(postCalled).toBe(false);
  });

  test("skips duplicate show in dry-run mode", async () => {
    const getMock = async (path: string) => {
      if (path.includes("/artists/search")) {
        return { artists: [{ id: 42, name: "Nina Hagen", slug: "nina-hagen" }] };
      }
      if (path.includes("/venues/search")) {
        return {
          venues: [
            { id: 10, name: "Crescent Ballroom", slug: "crescent-ballroom", city: "Phoenix", state: "AZ" },
          ],
        };
      }
      if (path.includes("/shows")) {
        return [
          {
            id: 500,
            slug: "2026-04-15-crescent-ballroom",
            event_date: "2026-04-15T20:00:00Z",
            venues: [{ id: 10, name: "Crescent Ballroom" }],
            artists: [{ id: 42, name: "Nina Hagen" }],
          },
        ];
      }
      return {};
    };

    const client = createMockClient({ get: getMock });

    const json = JSON.stringify({
      event_date: "2026-04-15",
      city: "Phoenix",
      state: "AZ",
      artists: [{ name: "Nina Hagen" }],
      venues: [{ name: "Crescent Ballroom", city: "Phoenix", state: "AZ" }],
    });

    const result = await submitShows(client, json, false);
    expect(result.created).toBe(0);
    expect(result.plans[0].duplicate?.isDuplicate).toBe(true);
  });

  test("creates new show when no duplicate found", async () => {
    const getMock = async (path: string) => {
      if (path.includes("/artists/search")) {
        return { artists: [{ id: 42, name: "Nina Hagen", slug: "nina-hagen" }] };
      }
      if (path.includes("/venues/search")) {
        return {
          venues: [
            { id: 10, name: "Crescent Ballroom", slug: "crescent-ballroom", city: "Phoenix", state: "AZ" },
          ],
        };
      }
      if (path.includes("/shows")) {
        return []; // No existing shows on this date
      }
      return {};
    };

    const client = createMockClient({
      get: getMock,
      post: async () => ({ id: 100 }),
    });

    const json = JSON.stringify({
      event_date: "2026-04-15",
      city: "Phoenix",
      state: "AZ",
      artists: [{ name: "Nina Hagen" }],
      venues: [{ name: "Crescent Ballroom", city: "Phoenix", state: "AZ" }],
    });

    const result = await submitShows(client, json, true);
    expect(result.created).toBe(1);
    expect(result.plans[0].duplicate?.isDuplicate).toBe(false);
  });

  test("mixed batch: one duplicate, one new", async () => {
    const getMock = async (path: string, params?: Record<string, string>) => {
      if (path.includes("/artists/search")) {
        return { artists: [{ id: 42, name: "Nina Hagen", slug: "nina-hagen" }] };
      }
      if (path.includes("/venues/search")) {
        return {
          venues: [
            { id: 10, name: "Crescent Ballroom", slug: "crescent-ballroom", city: "Phoenix", state: "AZ" },
          ],
        };
      }
      if (path.includes("/shows")) {
        // Only return existing show for April 15
        if (params?.from_date?.includes("2026-04-15")) {
          return [
            {
              id: 500,
              event_date: "2026-04-15T20:00:00Z",
              venues: [{ id: 10, name: "Crescent Ballroom" }],
              artists: [{ id: 42, name: "Nina Hagen" }],
            },
          ];
        }
        return []; // No shows on April 16
      }
      return {};
    };

    let postCount = 0;
    const client = createMockClient({
      get: getMock,
      post: async () => ({ id: 200 + ++postCount }),
    });

    const json = JSON.stringify([
      {
        event_date: "2026-04-15",
        city: "Phoenix",
        state: "AZ",
        artists: [{ name: "Nina Hagen" }],
        venues: [{ name: "Crescent Ballroom", city: "Phoenix", state: "AZ" }],
      },
      {
        event_date: "2026-04-16",
        city: "Phoenix",
        state: "AZ",
        artists: [{ name: "Nina Hagen" }],
        venues: [{ name: "Crescent Ballroom", city: "Phoenix", state: "AZ" }],
      },
    ]);

    const result = await submitShows(client, json, true);
    expect(result.created).toBe(1);
    expect(result.skipped).toBe(1);
    expect(result.plans[0].duplicate?.isDuplicate).toBe(true);
    expect(result.plans[1].duplicate?.isDuplicate).toBe(false);
  });
});
