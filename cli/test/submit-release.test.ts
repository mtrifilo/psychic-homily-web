import { describe, test, expect, mock, beforeEach } from "bun:test";
import {
  parseReleaseInput,
  planReleases,
  submitReleases,
  displayPreview,
  type SubmitResult,
} from "../src/commands/submit-release";
import { APIClient } from "../src/lib/api";

// -- Helpers --

/** Create a mock APIClient that returns controlled responses. */
function createMockClient(
  responses: Record<string, unknown>,
): APIClient {
  const client = new APIClient({ url: "http://test.local", token: "phk_test" });

  // Override get/post/put with mock implementations
  client.get = mock(async (path: string, params?: Record<string, string>) => {
    // Artist search
    if (path === "/artists/search" && params?.q) {
      const key = `artist:${params.q}`;
      if (responses[key]) return responses[key];
      return { artists: [] };
    }
    // Release search (used by checkDuplicate)
    if (path === "/releases/search") {
      if (responses["releases:search"]) return responses["releases:search"];
      return { releases: [] };
    }
    // Release list (fallback used by checkDuplicate)
    if (path === "/releases") {
      if (responses["releases:list"]) return responses["releases:list"];
      return { releases: [] };
    }
    return {};
  }) as typeof client.get;

  client.post = mock(async (path: string, body?: unknown) => {
    if (responses["post:error"]) throw new Error(responses["post:error"] as string);
    return responses["post:result"] || { id: 1, title: "created" };
  }) as typeof client.post;

  client.put = mock(async (path: string, body?: unknown) => {
    if (responses["put:error"]) throw new Error(responses["put:error"] as string);
    return responses["put:result"] || { id: 1, title: "updated" };
  }) as typeof client.put;

  return client;
}

// -- Tests --

describe("parseReleaseInput", () => {
  test("parses a single release object", () => {
    const input = JSON.stringify({
      title: "Nunsexmonkrock",
      artists: [{ name: "Nina Hagen Band" }],
    });
    const releases = parseReleaseInput(input);
    expect(releases).toHaveLength(1);
    expect(releases[0].title).toBe("Nunsexmonkrock");
  });

  test("parses an array of releases", () => {
    const input = JSON.stringify([
      { title: "Album A", artists: [{ name: "Artist A" }] },
      { title: "Album B", artists: [{ name: "Artist B" }] },
    ]);
    const releases = parseReleaseInput(input);
    expect(releases).toHaveLength(2);
    expect(releases[0].title).toBe("Album A");
    expect(releases[1].title).toBe("Album B");
  });

  test("throws on invalid JSON", () => {
    expect(() => parseReleaseInput("not json")).toThrow();
  });
});

describe("planReleases", () => {
  test("plans a create when no duplicate found", async () => {
    const client = createMockClient({
      "artist:Nina Hagen Band": {
        artists: [{ id: 42, name: "Nina Hagen Band", slug: "nina-hagen-band" }],
      },
      "releases:list": { releases: [] },
    });

    const actions = await planReleases(client, [
      {
        title: "Nunsexmonkrock",
        release_type: "lp",
        release_year: 1982,
        artists: [{ name: "Nina Hagen Band" }],
        external_links: [{ platform: "bandcamp", url: "https://example.com" }],
      },
    ]);

    expect(actions).toHaveLength(1);
    expect(actions[0].action).toBe("create");
    expect(actions[0].resolvedArtists).toHaveLength(1);
    expect(actions[0].resolvedArtists[0].artist_id).toBe(42);
    expect(actions[0].resolvedArtists[0].name).toBe("Nina Hagen Band");
    expect(actions[0].unresolvedArtists).toHaveLength(0);
  });

  test("plans an update when duplicate exists with new info", async () => {
    const client = createMockClient({
      "artist:Radiohead": {
        artists: [{ id: 1, name: "Radiohead", slug: "radiohead" }],
      },
      "releases:list": {
        releases: [
          {
            id: 10,
            title: "OK Computer",
            slug: "ok-computer",
            release_type: "lp",
            release_year: null,
            bandcamp_url: "",
            spotify_url: "",
            description: "",
          },
        ],
      },
    });

    const actions = await planReleases(client, [
      {
        title: "OK Computer",
        release_type: "lp",
        release_year: 1997,
        artists: [{ name: "Radiohead" }],
      },
    ]);

    expect(actions).toHaveLength(1);
    expect(actions[0].action).toBe("update");
    expect(actions[0].dupCheck.existingId).toBe(10);
    expect(actions[0].dupCheck.existingName).toBe("OK Computer");
    // release_year should be identified as new_info
    const yearField = actions[0].dupCheck.fields.find(
      (f) => f.field === "release_year",
    );
    expect(yearField?.status).toBe("new_info");
  });

  test("plans a skip when duplicate is already complete", async () => {
    const client = createMockClient({
      "artist:Radiohead": {
        artists: [{ id: 1, name: "Radiohead", slug: "radiohead" }],
      },
      "releases:list": {
        releases: [
          {
            id: 10,
            title: "OK Computer",
            slug: "ok-computer",
            release_type: "lp",
            release_year: 1997,
            release_date: "1997-05-21",
            bandcamp_url: "",
            spotify_url: "",
            description: "",
          },
        ],
      },
    });

    const actions = await planReleases(client, [
      {
        title: "OK Computer",
        release_type: "lp",
        release_year: 1997,
        artists: [{ name: "Radiohead" }],
      },
    ]);

    expect(actions).toHaveLength(1);
    expect(actions[0].action).toBe("skip");
    expect(actions[0].dupCheck.existingId).toBe(10);
  });

  test("validation error skips the release", async () => {
    const client = createMockClient({});

    const actions = await planReleases(client, [
      {
        title: "",
        artists: [],
      } as any,
    ]);

    expect(actions).toHaveLength(1);
    expect(actions[0].action).toBe("skip");
    expect(actions[0].validationErrors).toBeDefined();
    expect(actions[0].validationErrors!.length).toBeGreaterThan(0);
    expect(
      actions[0].validationErrors!.some((e) => e.includes("title")),
    ).toBe(true);
  });

  test("resolves artists by name", async () => {
    const client = createMockClient({
      "artist:Deerhunter": {
        artists: [{ id: 5, name: "Deerhunter", slug: "deerhunter" }],
      },
      "artist:Atlas Sound": {
        artists: [{ id: 6, name: "Atlas Sound", slug: "atlas-sound" }],
      },
      "releases:list": { releases: [] },
    });

    const actions = await planReleases(client, [
      {
        title: "Halcyon Digest",
        release_type: "lp",
        release_year: 2010,
        artists: [
          { name: "Deerhunter" },
          { name: "Atlas Sound", role: "featured" },
        ],
      },
    ]);

    expect(actions).toHaveLength(1);
    expect(actions[0].resolvedArtists).toHaveLength(2);
    expect(actions[0].resolvedArtists[0].artist_id).toBe(5);
    expect(actions[0].resolvedArtists[0].role).toBe("main");
    expect(actions[0].resolvedArtists[1].artist_id).toBe(6);
    expect(actions[0].resolvedArtists[1].role).toBe("featured");
  });

  test("warns on unresolved artists", async () => {
    const client = createMockClient({
      "releases:list": { releases: [] },
    });

    const actions = await planReleases(client, [
      {
        title: "Unknown Album",
        artists: [{ name: "Nonexistent Band" }],
      },
    ]);

    expect(actions).toHaveLength(1);
    expect(actions[0].action).toBe("create");
    expect(actions[0].unresolvedArtists).toEqual(["Nonexistent Band"]);
    expect(actions[0].resolvedArtists).toHaveLength(0);
  });

  test("normalizes string artist array to objects", async () => {
    const client = createMockClient({
      "artist:Sonic Youth": {
        artists: [{ id: 3, name: "Sonic Youth", slug: "sonic-youth" }],
      },
      "releases:list": { releases: [] },
    });

    const actions = await planReleases(client, [
      {
        title: "Daydream Nation",
        artists: ["Sonic Youth"] as any,
      },
    ]);

    expect(actions).toHaveLength(1);
    expect(actions[0].resolvedArtists).toHaveLength(1);
    expect(actions[0].resolvedArtists[0].artist_id).toBe(3);
    expect(actions[0].resolvedArtists[0].role).toBe("main");
  });
});

describe("submitReleases", () => {
  let origFetch: typeof globalThis.fetch;

  beforeEach(() => {
    origFetch = globalThis.fetch;
  });

  function mockFetch(responses: Record<string, unknown>) {
    globalThis.fetch = (async (url: string | URL | Request) => {
      const urlStr = typeof url === "string" ? url : url instanceof URL ? url.toString() : url.url;

      // Release search/list for duplicate checking
      if (urlStr.includes("/releases")) {
        return new Response(JSON.stringify(responses["releases"] || { releases: [] }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }
      // Artist search for resolution
      if (urlStr.includes("/artists/search")) {
        return new Response(JSON.stringify(responses["artists"] || { artists: [] }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }
      return new Response("{}", { status: 200 });
    }) as typeof globalThis.fetch;
  }

  function restoreFetch() {
    globalThis.fetch = origFetch;
  }

  test("dry-run does not make API calls for create/update", async () => {
    const origWrite = process.stderr.write;
    process.stderr.write = (() => true) as typeof process.stderr.write;
    mockFetch({});

    try {
      const result = await submitReleases(
        JSON.stringify({
          title: "Nunsexmonkrock",
          release_type: "lp",
          release_year: 1982,
          artists: [{ name: "Nina Hagen Band" }],
        }),
        { url: "http://test.local", token: "phk_test" },
        false,
      );

      // In dry-run, created/updated should be 0
      expect(result.created).toBe(0);
      expect(result.updated).toBe(0);
    } finally {
      process.stderr.write = origWrite;
      restoreFetch();
    }
  });

  test("returns error count for invalid JSON", async () => {
    const origWrite = process.stderr.write;
    process.stderr.write = (() => true) as typeof process.stderr.write;

    try {
      const result = await submitReleases(
        "not valid json",
        { url: "http://test.local", token: "phk_test" },
        false,
      );

      expect(result.errors).toBe(1);
      expect(result.created).toBe(0);
    } finally {
      process.stderr.write = origWrite;
    }
  });

  test("returns empty result for empty array", async () => {
    const origWrite = process.stderr.write;
    process.stderr.write = (() => true) as typeof process.stderr.write;

    try {
      const result = await submitReleases(
        "[]",
        { url: "http://test.local", token: "phk_test" },
        false,
      );

      expect(result.created).toBe(0);
      expect(result.updated).toBe(0);
      expect(result.skipped).toBe(0);
      expect(result.errors).toBe(0);
    } finally {
      process.stderr.write = origWrite;
    }
  });
});

describe("displayPreview", () => {
  test("does not throw for create actions", () => {
    const origWrite = process.stderr.write;
    const output: string[] = [];
    process.stderr.write = ((s: string) => {
      output.push(s);
      return true;
    }) as typeof process.stderr.write;

    try {
      displayPreview([
        {
          release: {
            title: "Test Album",
            release_type: "lp",
            release_year: 2024,
            artists: [{ name: "Test Artist" }],
            external_links: [{ platform: "bandcamp", url: "https://example.com" }],
          },
          action: "create",
          dupCheck: { action: "create", match: "none", fields: [], confidence: 0 },
          resolvedArtists: [{ artist_id: 1, name: "Test Artist", role: "main" }],
          unresolvedArtists: [],
        },
      ]);

      const combined = output.join("");
      expect(combined).toContain("Test Album");
    } finally {
      process.stderr.write = origWrite;
    }
  });

  test("does not throw for update actions", () => {
    const origWrite = process.stderr.write;
    process.stderr.write = (() => true) as typeof process.stderr.write;

    try {
      displayPreview([
        {
          release: {
            title: "Test Album",
            artists: [{ name: "Test Artist" }],
          },
          action: "update",
          dupCheck: {
            action: "update",
            match: "exact",
            existingId: 10,
            existingName: "Test Album",
            fields: [
              { field: "release_year", existing: "", proposed: "2024", status: "new_info" },
            ],
            confidence: 1.0,
          },
          resolvedArtists: [{ artist_id: 1, name: "Test Artist", role: "main" }],
          unresolvedArtists: [],
        },
      ]);
    } finally {
      process.stderr.write = origWrite;
    }
  });

  test("does not throw for skip actions", () => {
    const origWrite = process.stderr.write;
    process.stderr.write = (() => true) as typeof process.stderr.write;

    try {
      displayPreview([
        {
          release: {
            title: "Test Album",
            artists: [{ name: "Test Artist" }],
          },
          action: "skip",
          dupCheck: {
            action: "skip",
            match: "exact",
            existingId: 10,
            existingName: "Test Album",
            fields: [],
            confidence: 1.0,
          },
          resolvedArtists: [],
          unresolvedArtists: [],
        },
      ]);
    } finally {
      process.stderr.write = origWrite;
    }
  });

  test("shows validation errors", () => {
    const origWrite = process.stderr.write;
    const output: string[] = [];
    process.stderr.write = ((s: string) => {
      output.push(s);
      return true;
    }) as typeof process.stderr.write;

    try {
      displayPreview([
        {
          release: {
            title: "",
            artists: [],
          } as any,
          action: "skip",
          dupCheck: { action: "skip", match: "none", fields: [], confidence: 0 },
          resolvedArtists: [],
          unresolvedArtists: [],
          validationErrors: ["title: title is required"],
        },
      ]);

      const combined = output.join("");
      expect(combined).toContain("title is required");
    } finally {
      process.stderr.write = origWrite;
    }
  });
});
