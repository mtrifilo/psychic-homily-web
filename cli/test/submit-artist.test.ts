import { describe, test, expect, mock, beforeEach } from "bun:test";
import {
  parseArtistInput,
  displayArtistPreview,
  buildUpdateBody,
  submitArtists,
} from "../src/commands/submit-artist";
import type { DuplicateCheckResult } from "../src/lib/duplicates";
import type { APIClient } from "../src/lib/api";

// -- parseArtistInput tests --

describe("parseArtistInput", () => {
  test("parses a single JSON object", () => {
    const result = parseArtistInput('{"name": "Nina Hagen", "city": "Berlin"}');
    expect(result).toEqual([{ name: "Nina Hagen", city: "Berlin" }]);
  });

  test("parses a JSON array", () => {
    const result = parseArtistInput(
      '[{"name": "Nina Hagen"}, {"name": "Flower Travelin\' Band"}]',
    );
    expect(result).toHaveLength(2);
    expect(result[0].name).toBe("Nina Hagen");
    expect(result[1].name).toBe("Flower Travelin' Band");
  });

  test("wraps a single object into an array", () => {
    const result = parseArtistInput('{"name": "Test"}');
    expect(Array.isArray(result)).toBe(true);
    expect(result).toHaveLength(1);
  });

  test("throws on invalid JSON", () => {
    expect(() => parseArtistInput("not json")).toThrow();
  });

  test("throws on primitive JSON values", () => {
    expect(() => parseArtistInput('"just a string"')).toThrow(
      "Input must be a JSON object or array of objects",
    );
  });
});

// -- buildUpdateBody tests --

describe("buildUpdateBody", () => {
  test("includes only new_info fields", () => {
    const fields = [
      {
        field: "name",
        existing: "Test",
        proposed: "Test",
        status: "unchanged" as const,
      },
      {
        field: "city",
        existing: "",
        proposed: "Phoenix",
        status: "new_info" as const,
      },
      {
        field: "website",
        existing: "https://old.com",
        proposed: "https://new.com",
        status: "already_set" as const,
      },
    ];

    const body = buildUpdateBody(fields);
    expect(body).toEqual({ city: "Phoenix" });
    expect(body.name).toBeUndefined();
    expect(body.website).toBeUndefined();
  });

  test("returns empty object when no new_info fields", () => {
    const fields = [
      {
        field: "name",
        existing: "Test",
        proposed: "Test",
        status: "unchanged" as const,
      },
    ];

    const body = buildUpdateBody(fields);
    expect(body).toEqual({});
  });

  test("includes multiple new_info fields", () => {
    const fields = [
      {
        field: "city",
        existing: "",
        proposed: "Phoenix",
        status: "new_info" as const,
      },
      {
        field: "bandcamp_url",
        existing: "",
        proposed: "https://test.bandcamp.com",
        status: "new_info" as const,
      },
    ];

    const body = buildUpdateBody(fields);
    expect(body).toEqual({
      city: "Phoenix",
      bandcamp_url: "https://test.bandcamp.com",
    });
  });
});

// -- Helper: mock APIClient --

function createMockClient(overrides?: {
  get?: (path: string, params?: Record<string, string>) => Promise<unknown>;
  post?: (path: string, body?: unknown) => Promise<unknown>;
  patch?: (path: string, body?: unknown) => Promise<unknown>;
}): APIClient {
  return {
    get: overrides?.get ?? mock(() => Promise.resolve({ artists: [] })),
    post: overrides?.post ?? mock(() => Promise.resolve({ artist: { id: 1, name: "Test" } })),
    patch: overrides?.patch ?? mock(() => Promise.resolve({})),
  } as unknown as APIClient;
}

// -- submitArtists tests --

describe("submitArtists", () => {
  test("single artist create — no duplicate found", async () => {
    const postFn = mock(() =>
      Promise.resolve({ artist: { id: 42, name: "Nina Hagen" } }),
    );
    const client = createMockClient({
      get: mock(() => Promise.resolve({ artists: [] })),
      post: postFn,
    });

    const artists = [{ name: "Nina Hagen", city: "Berlin" }];
    const results = await submitArtists(client, artists, { confirm: true });

    expect(results).toHaveLength(1);
    expect(results[0].action).toBe("created");
    expect(results[0].id).toBe(42);
    expect(results[0].name).toBe("Nina Hagen");
    expect(postFn).toHaveBeenCalledTimes(1);
  });

  test("single artist update — duplicate found with new info", async () => {
    const patchFn = mock(() => Promise.resolve({}));
    const client = createMockClient({
      get: mock(() =>
        Promise.resolve({
          artists: [
            {
              id: 10,
              name: "Nina Hagen",
              slug: "nina-hagen",
              city: "",
              state: "",
              country: "",
              website: "",
              bandcamp_url: "",
              spotify_url: "",
              instagram_url: "",
              description: "",
            },
          ],
        }),
      ),
      patch: patchFn,
    });

    const artists = [
      {
        name: "Nina Hagen",
        city: "Berlin",
        bandcamp_url: "https://ninahagen.bandcamp.com",
      },
    ];
    const results = await submitArtists(client, artists, { confirm: true });

    expect(results).toHaveLength(1);
    expect(results[0].action).toBe("updated");
    expect(results[0].id).toBe(10);
    expect(patchFn).toHaveBeenCalledTimes(1);

    // Verify that only new_info fields were sent
    const patchCall = patchFn.mock.calls[0] as unknown as [string, Record<string, string>];
    const patchBody = patchCall[1];
    expect(patchBody.city).toBe("Berlin");
    expect(patchBody.bandcamp_url).toBe("https://ninahagen.bandcamp.com");
    expect(patchBody.name).toBeUndefined(); // name is unchanged, not new_info
  });

  test("single artist skip — duplicate found, no new info", async () => {
    const postFn = mock(() => Promise.resolve({}));
    const patchFn = mock(() => Promise.resolve({}));
    const client = createMockClient({
      get: mock(() =>
        Promise.resolve({
          artists: [
            {
              id: 10,
              name: "Nina Hagen",
              slug: "nina-hagen",
              city: "Berlin",
              state: "",
              country: "",
              website: "",
              bandcamp_url: "",
              spotify_url: "",
              instagram_url: "",
              description: "",
            },
          ],
        }),
      ),
      post: postFn,
      patch: patchFn,
    });

    // Only providing name and city, both already exist
    const artists = [{ name: "Nina Hagen", city: "Berlin" }];
    const results = await submitArtists(client, artists, { confirm: true });

    expect(results).toHaveLength(1);
    expect(results[0].action).toBe("skipped");
    expect(results[0].id).toBe(10);
    expect(postFn).not.toHaveBeenCalled();
    expect(patchFn).not.toHaveBeenCalled();
  });

  test("array of mixed creates/updates/skips", async () => {
    const getFn = mock((_path: string, params?: Record<string, string>) => {
      const q = params?.q || "";
      if (q === "New Artist") {
        return Promise.resolve({ artists: [] });
      }
      if (q === "Existing Artist") {
        return Promise.resolve({
          artists: [
            {
              id: 20,
              name: "Existing Artist",
              slug: "existing-artist",
              city: "",
              state: "",
              country: "",
              website: "",
              bandcamp_url: "",
              spotify_url: "",
              instagram_url: "",
              description: "",
            },
          ],
        });
      }
      if (q === "Complete Artist") {
        return Promise.resolve({
          artists: [
            {
              id: 30,
              name: "Complete Artist",
              slug: "complete-artist",
              city: "Phoenix",
              state: "AZ",
              country: "US",
              website: "https://complete.com",
              bandcamp_url: "",
              spotify_url: "",
              instagram_url: "",
              description: "",
            },
          ],
        });
      }
      return Promise.resolve({ artists: [] });
    });
    const postFn = mock(() =>
      Promise.resolve({ artist: { id: 99, name: "New Artist" } }),
    );
    const patchFn = mock(() => Promise.resolve({}));

    const client = createMockClient({
      get: getFn,
      post: postFn,
      patch: patchFn,
    });

    const artists = [
      { name: "New Artist", city: "Austin" }, // create
      { name: "Existing Artist", city: "Phoenix" }, // update (city is new)
      { name: "Complete Artist", city: "Phoenix" }, // skip (city already set)
    ];

    const results = await submitArtists(client, artists, { confirm: true });

    expect(results).toHaveLength(3);
    expect(results[0].action).toBe("created");
    expect(results[1].action).toBe("updated");
    expect(results[2].action).toBe("skipped");

    expect(postFn).toHaveBeenCalledTimes(1);
    expect(patchFn).toHaveBeenCalledTimes(1);
  });

  test("validation error — missing name", async () => {
    const client = createMockClient();

    const artists = [{ city: "Phoenix" }]; // missing name
    const results = await submitArtists(client, artists, { confirm: true });

    expect(results).toHaveLength(1);
    expect(results[0].action).toBe("error");
    expect(results[0].error).toContain("name");
  });

  test("dry-run mode — no API calls made", async () => {
    const postFn = mock(() => Promise.resolve({}));
    const patchFn = mock(() => Promise.resolve({}));
    const client = createMockClient({
      get: mock(() => Promise.resolve({ artists: [] })),
      post: postFn,
      patch: patchFn,
    });

    const artists = [{ name: "Test Artist" }];
    const results = await submitArtists(client, artists, { confirm: false });

    expect(results).toHaveLength(1);
    // In dry-run, the action should reflect what would happen
    expect(results[0].action).toBe("created");
    // No POST or PATCH calls should be made
    expect(postFn).not.toHaveBeenCalled();
    expect(patchFn).not.toHaveBeenCalled();
  });

  test("confirm mode — API calls are made", async () => {
    const postFn = mock(() =>
      Promise.resolve({ artist: { id: 1, name: "Test Artist" } }),
    );
    const client = createMockClient({
      get: mock(() => Promise.resolve({ artists: [] })),
      post: postFn,
    });

    const artists = [{ name: "Test Artist" }];
    const results = await submitArtists(client, artists, { confirm: true });

    expect(results).toHaveLength(1);
    expect(results[0].action).toBe("created");
    expect(postFn).toHaveBeenCalledTimes(1);
  });

  test("API error during create is handled gracefully", async () => {
    const client = createMockClient({
      get: mock(() => Promise.resolve({ artists: [] })),
      post: mock(() => Promise.reject(new Error("500: Internal Server Error"))),
    });

    const artists = [{ name: "Error Artist" }];
    const results = await submitArtists(client, artists, { confirm: true });

    expect(results).toHaveLength(1);
    expect(results[0].action).toBe("error");
    expect(results[0].error).toContain("500");
  });

  test("API error during update is handled gracefully", async () => {
    const client = createMockClient({
      get: mock(() =>
        Promise.resolve({
          artists: [
            {
              id: 10,
              name: "Existing",
              slug: "existing",
              city: "",
              state: "",
              country: "",
              website: "",
              bandcamp_url: "",
              spotify_url: "",
              instagram_url: "",
              description: "",
            },
          ],
        }),
      ),
      patch: mock(() => Promise.reject(new Error("403: Forbidden"))),
    });

    const artists = [{ name: "Existing", city: "Phoenix" }];
    const results = await submitArtists(client, artists, { confirm: true });

    expect(results).toHaveLength(1);
    expect(results[0].action).toBe("error");
    expect(results[0].error).toContain("403");
  });

  test("multiple validation errors are all reported", async () => {
    const client = createMockClient();

    const artists = [
      { city: "Phoenix" }, // missing name
      { state: "AZ" }, // also missing name
    ];
    const results = await submitArtists(client, artists, { confirm: true });

    expect(results).toHaveLength(2);
    expect(results[0].action).toBe("error");
    expect(results[1].action).toBe("error");
  });
});

// -- displayArtistPreview tests (smoke tests — verifies no crashes) --

describe("displayArtistPreview", () => {
  test("handles create action without errors", () => {
    const dupResult: DuplicateCheckResult = {
      action: "create",
      match: "none",
      fields: [],
      confidence: 0,
    };

    // Should not throw
    displayArtistPreview({ name: "New Artist", city: "Austin" }, dupResult, 0);
  });

  test("handles update action without errors", () => {
    const dupResult: DuplicateCheckResult = {
      action: "update",
      match: "exact",
      existingId: 10,
      existingName: "Existing Artist",
      fields: [
        {
          field: "city",
          existing: "",
          proposed: "Phoenix",
          status: "new_info",
        },
      ],
      confidence: 1.0,
    };

    displayArtistPreview({ name: "Existing Artist", city: "Phoenix" }, dupResult, 0);
  });

  test("handles skip action without errors", () => {
    const dupResult: DuplicateCheckResult = {
      action: "skip",
      match: "exact",
      existingId: 10,
      existingName: "Existing Artist",
      fields: [],
      confidence: 1.0,
    };

    displayArtistPreview({ name: "Existing Artist" }, dupResult, 0);
  });
});
