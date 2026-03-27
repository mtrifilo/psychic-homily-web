import { describe, test, expect, mock, beforeEach } from "bun:test";
import { submitVenues } from "../src/commands/submit-venue";
import type { APIClient } from "../src/lib/api";

/** Create a mock API client with configurable responses. */
function createMockClient(overrides: {
  get?: (path: string, params?: Record<string, string>) => Promise<unknown>;
  post?: (path: string, body?: unknown) => Promise<unknown>;
  put?: (path: string, body?: unknown) => Promise<unknown>;
} = {}): APIClient {
  return {
    get: overrides.get ?? mock(() => Promise.resolve({ venues: [] })),
    post: overrides.post ?? mock(() => Promise.resolve({ id: 1, name: "Test Venue", slug: "test-venue" })),
    put: overrides.put ?? mock(() => Promise.resolve({ id: 1, name: "Test Venue", slug: "test-venue" })),
    patch: mock(() => Promise.resolve({})),
    delete: mock(() => Promise.resolve({})),
    healthCheck: mock(() => Promise.resolve(true)),
    verifyAuth: mock(() => Promise.resolve(null)),
  } as unknown as APIClient;
}

describe("submitVenues", () => {
  // Suppress stderr output during tests
  let originalWrite: typeof process.stderr.write;
  beforeEach(() => {
    originalWrite = process.stderr.write;
    process.stderr.write = mock(() => true) as unknown as typeof process.stderr.write;
  });

  // Restore after each test
  const restoreStderr = () => {
    process.stderr.write = originalWrite;
  };

  test("single venue create — no duplicates found", async () => {
    const client = createMockClient({
      get: mock(() => Promise.resolve({ venues: [] })),
    });

    const venues = [
      { name: "Crescent Ballroom", city: "Phoenix", state: "AZ", website: "https://crescentphx.com" },
    ];

    const result = await submitVenues(client, venues, false);
    restoreStderr();

    expect(result.creates).toBe(1);
    expect(result.updates).toBe(0);
    expect(result.skips).toBe(0);
    expect(result.errors).toBe(0);
    expect(result.results).toHaveLength(1);
    expect(result.results[0].action).toBe("create");
  });

  test("single venue create with --confirm calls POST /admin/venues", async () => {
    const postMock = mock(() =>
      Promise.resolve({ id: 5, name: "Crescent Ballroom", slug: "crescent-ballroom" }),
    );
    const client = createMockClient({
      get: mock(() => Promise.resolve({ venues: [] })),
      post: postMock,
    });

    const venues = [
      { name: "Crescent Ballroom", city: "Phoenix", state: "AZ" },
    ];

    const result = await submitVenues(client, venues, true);
    restoreStderr();

    expect(result.creates).toBe(1);
    expect(result.errors).toBe(0);
    expect(postMock).toHaveBeenCalledTimes(1);
    // Payload should contain only API-accepted fields (tags, entity_type, etc. stripped)
    expect(postMock).toHaveBeenCalledWith("/admin/venues", {
      name: "Crescent Ballroom",
      city: "Phoenix",
      state: "AZ",
    });
    expect(result.results[0].action).toBe("create");
    expect(result.results[0].message).toBe("Created successfully");
  });

  test("single venue update — existing match with new address info", async () => {
    const client = createMockClient({
      get: mock(() =>
        Promise.resolve({
          venues: [
            {
              id: 42,
              name: "Crescent Ballroom",
              slug: "crescent-ballroom",
              city: "Phoenix",
              state: "AZ",
              country: "",
              address: "",
              zip_code: "",
              website: "",
              capacity: "",
              description: "",
            },
          ],
        }),
      ),
    });

    const venues = [
      {
        name: "Crescent Ballroom",
        city: "Phoenix",
        state: "AZ",
        address: "308 N 2nd Ave",
        website: "https://crescentphx.com",
      },
    ];

    const result = await submitVenues(client, venues, false);
    restoreStderr();

    expect(result.updates).toBe(1);
    expect(result.creates).toBe(0);
    expect(result.skips).toBe(0);
    expect(result.errors).toBe(0);
    expect(result.results[0].action).toBe("update");
  });

  test("single venue update with --confirm calls PUT /venues/{id} with new_info fields only", async () => {
    const putMock = mock(() =>
      Promise.resolve({ id: 42, name: "Crescent Ballroom", slug: "crescent-ballroom" }),
    );
    const client = createMockClient({
      get: mock(() =>
        Promise.resolve({
          venues: [
            {
              id: 42,
              name: "Crescent Ballroom",
              slug: "crescent-ballroom",
              city: "Phoenix",
              state: "AZ",
              country: "",
              address: "",
              zip_code: "",
              website: "https://existing.com",
              capacity: "",
              description: "",
            },
          ],
        }),
      ),
      put: putMock,
    });

    const venues = [
      {
        name: "Crescent Ballroom",
        city: "Phoenix",
        state: "AZ",
        address: "308 N 2nd Ave",
        website: "https://crescentphx.com",
      },
    ];

    const result = await submitVenues(client, venues, true);
    restoreStderr();

    expect(result.updates).toBe(1);
    expect(putMock).toHaveBeenCalledTimes(1);
    // Should only send address (new_info), not website (already_set)
    expect(putMock).toHaveBeenCalledWith("/venues/42", { address: "308 N 2nd Ave" });
  });

  test("single venue skip — exact duplicate, no new info", async () => {
    const client = createMockClient({
      get: mock(() =>
        Promise.resolve({
          venues: [
            {
              id: 10,
              name: "The Van Buren",
              slug: "the-van-buren",
              city: "Phoenix",
              state: "AZ",
              country: "US",
              address: "401 W Van Buren St",
              zip_code: "85003",
              website: "https://thevanburenphx.com",
              capacity: "1800",
              description: "Live music venue in downtown Phoenix",
            },
          ],
        }),
      ),
    });

    const venues = [
      {
        name: "The Van Buren",
        city: "Phoenix",
        state: "AZ",
      },
    ];

    const result = await submitVenues(client, venues, false);
    restoreStderr();

    expect(result.skips).toBe(1);
    expect(result.creates).toBe(0);
    expect(result.updates).toBe(0);
    expect(result.errors).toBe(0);
    expect(result.results[0].action).toBe("skip");
  });

  test("validation error — missing city", async () => {
    const client = createMockClient();

    const venues = [
      { name: "Test Venue", state: "AZ" },
    ];

    const result = await submitVenues(client, venues, false);
    restoreStderr();

    expect(result.errors).toBe(1);
    expect(result.creates).toBe(0);
    expect(result.results[0].action).toBe("error");
    expect(result.results[0].message).toContain("city");
  });

  test("validation error — missing state", async () => {
    const client = createMockClient();

    const venues = [
      { name: "Test Venue", city: "Phoenix" },
    ];

    const result = await submitVenues(client, venues, false);
    restoreStderr();

    expect(result.errors).toBe(1);
    expect(result.results[0].action).toBe("error");
    expect(result.results[0].message).toContain("state");
  });

  test("validation error — missing name", async () => {
    const client = createMockClient();

    const venues = [
      { city: "Phoenix", state: "AZ" },
    ];

    const result = await submitVenues(client, venues, false);
    restoreStderr();

    expect(result.errors).toBe(1);
    expect(result.results[0].message).toContain("name");
  });

  test("validation error — not an object", async () => {
    const client = createMockClient();

    const venues = ["not an object" as unknown as Record<string, unknown>];

    const result = await submitVenues(client, venues, false);
    restoreStderr();

    expect(result.errors).toBe(1);
    expect(result.results[0].action).toBe("error");
  });

  test("dry-run mode does not make API calls", async () => {
    const postMock = mock(() => Promise.resolve({}));
    const putMock = mock(() => Promise.resolve({}));

    const client = createMockClient({
      get: mock(() => Promise.resolve({ venues: [] })),
      post: postMock,
      put: putMock,
    });

    const venues = [
      { name: "New Venue", city: "Phoenix", state: "AZ" },
    ];

    const result = await submitVenues(client, venues, false);
    restoreStderr();

    expect(result.creates).toBe(1);
    expect(postMock).not.toHaveBeenCalled();
    expect(putMock).not.toHaveBeenCalled();
    expect(result.results[0].message).toContain("Dry run");
  });

  test("confirm mode makes API calls for creates", async () => {
    const postMock = mock(() =>
      Promise.resolve({ id: 1, name: "New Venue", slug: "new-venue" }),
    );

    const client = createMockClient({
      get: mock(() => Promise.resolve({ venues: [] })),
      post: postMock,
    });

    const venues = [
      { name: "New Venue", city: "Phoenix", state: "AZ" },
    ];

    const result = await submitVenues(client, venues, true);
    restoreStderr();

    expect(result.creates).toBe(1);
    expect(postMock).toHaveBeenCalledTimes(1);
  });

  test("multiple venues with mixed actions", async () => {
    const client = createMockClient({
      get: mock((path: string, params?: Record<string, string>) => {
        // Simulate: first venue matches nothing, second matches existing
        const q = params?.q ?? "";
        if (q === "Brand New Venue") {
          return Promise.resolve({ venues: [] });
        }
        if (q === "Existing Place") {
          return Promise.resolve({
            venues: [
              {
                id: 99,
                name: "Existing Place",
                slug: "existing-place",
                city: "Phoenix",
                state: "AZ",
                country: "",
                address: "",
                zip_code: "",
                website: "",
                capacity: "",
                description: "",
              },
            ],
          });
        }
        return Promise.resolve({ venues: [] });
      }),
    });

    const venues = [
      { name: "Brand New Venue", city: "Tempe", state: "AZ" },
      { name: "Existing Place", city: "Phoenix", state: "AZ" },
    ];

    const result = await submitVenues(client, venues, false);
    restoreStderr();

    expect(result.creates).toBe(1);
    expect(result.skips).toBe(1);
    expect(result.errors).toBe(0);
    expect(result.results).toHaveLength(2);
  });

  test("API error during confirm is handled gracefully", async () => {
    const client = createMockClient({
      get: mock(() => Promise.resolve({ venues: [] })),
      post: mock(() => Promise.reject(new Error("Server error: 500"))),
    });

    const venues = [
      { name: "Failing Venue", city: "Phoenix", state: "AZ" },
    ];

    const result = await submitVenues(client, venues, true);
    restoreStderr();

    expect(result.errors).toBe(1);
    expect(result.creates).toBe(0);
    expect(result.results[0].action).toBe("error");
    expect(result.results[0].message).toContain("500");
  });

  test("skip with --confirm logs skip without API calls", async () => {
    const postMock = mock(() => Promise.resolve({}));
    const putMock = mock(() => Promise.resolve({}));

    const client = createMockClient({
      get: mock(() =>
        Promise.resolve({
          venues: [
            {
              id: 10,
              name: "The Van Buren",
              slug: "the-van-buren",
              city: "Phoenix",
              state: "AZ",
              country: "",
              address: "",
              zip_code: "",
              website: "",
              capacity: "",
              description: "",
            },
          ],
        }),
      ),
      post: postMock,
      put: putMock,
    });

    const venues = [
      { name: "The Van Buren", city: "Phoenix", state: "AZ" },
    ];

    const result = await submitVenues(client, venues, true);
    restoreStderr();

    expect(result.skips).toBe(1);
    expect(postMock).not.toHaveBeenCalled();
    expect(putMock).not.toHaveBeenCalled();
  });
});
