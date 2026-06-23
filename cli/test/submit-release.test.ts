import { describe, test, expect, mock, beforeEach } from "bun:test";
import {
  parseReleaseInput,
  planReleases,
  submitReleases,
  displayPreview,
  linkReleaseLabels,
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
      if (responses["releases:search"]) return responses["releases:search"];
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
      "releases:search": { releases: [] },
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
      "releases:search": {
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
      "releases:search": {
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
      "releases:search": { releases: [] },
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
      "releases:search": { releases: [] },
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
      "releases:search": { releases: [] },
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

  test("create path threads catalog_number through to the label-link POST", async () => {
    const origWrite = process.stderr.write;
    process.stderr.write = (() => true) as typeof process.stderr.write;

    const calls: Array<{ method: string; url: string; body: unknown }> = [];
    const json = (data: unknown) =>
      new Response(JSON.stringify(data), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });

    globalThis.fetch = (async (url: string | URL | Request, init?: RequestInit) => {
      const urlStr =
        typeof url === "string" ? url : url instanceof URL ? url.toString() : url.url;
      const method = (init?.method || "GET").toUpperCase();
      const body = init?.body ? JSON.parse(init.body as string) : undefined;
      calls.push({ method, url: urlStr, body });

      if (urlStr.includes("/artists/search")) {
        return json({ artists: [{ id: 42, name: "Nina Hagen Band", slug: "nina-hagen-band" }] });
      }
      if (urlStr.includes("/labels/search")) {
        return json({ labels: [{ id: 4, name: "Creation Records", slug: "creation-records" }] });
      }
      if (urlStr.includes("/admin/labels/")) {
        return json({ success: true });
      }
      // Release dup-check (GET) → no match; release create (POST) → new id
      if (urlStr.includes("/releases")) {
        return method === "POST" ? json({ release: { id: 555 } }) : json({ releases: [] });
      }
      return json({});
    }) as typeof globalThis.fetch;

    try {
      const result = await submitReleases(
        JSON.stringify({
          title: "Nunsexmonkrock",
          release_type: "lp",
          artists: [{ name: "Nina Hagen Band" }],
          labels: ["Creation Records"],
          catalog_number: "CRE001",
        }),
        { url: "http://test.local", token: "phk_test" },
        true,
      );

      expect(result.created).toBe(1);
      const link = calls.find(
        (c) => c.method === "POST" && /\/admin\/labels\/4\/releases$/.test(c.url),
      );
      expect(link).toBeDefined();
      expect(link!.body).toMatchObject({ release_id: 555, catalog_number: "CRE001" });
    } finally {
      process.stderr.write = origWrite;
      restoreFetch();
    }
  });

  test("skip path (backfill) threads catalog_number to the label-link POST", async () => {
    // The literal AC: re-ingesting an existing discography to backfill numbers.
    // Every release already exists → routes to SKIP, where the link still runs.
    const origWrite = process.stderr.write;
    process.stderr.write = (() => true) as typeof process.stderr.write;

    const calls: Array<{ method: string; url: string; body: unknown }> = [];
    const json = (data: unknown) =>
      new Response(JSON.stringify(data), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });

    globalThis.fetch = (async (url: string | URL | Request, init?: RequestInit) => {
      const urlStr =
        typeof url === "string" ? url : url instanceof URL ? url.toString() : url.url;
      const method = (init?.method || "GET").toUpperCase();
      const body = init?.body ? JSON.parse(init.body as string) : undefined;
      calls.push({ method, url: urlStr, body });

      if (urlStr.includes("/artists/search")) {
        return json({ artists: [{ id: 42, name: "Radiohead", slug: "radiohead" }] });
      }
      if (urlStr.includes("/labels/search")) {
        return json({ labels: [{ id: 4, name: "Creation Records", slug: "creation-records" }] });
      }
      if (urlStr.includes("/admin/labels/")) {
        return json({ success: true });
      }
      // Dup-check finds an already-complete release → action = skip
      if (urlStr.includes("/releases")) {
        return json({
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
        });
      }
      return json({});
    }) as typeof globalThis.fetch;

    try {
      const result = await submitReleases(
        JSON.stringify({
          title: "OK Computer",
          release_type: "lp",
          release_year: 1997,
          release_date: "1997-05-21",
          artists: [{ name: "Radiohead" }],
          labels: ["Creation Records"],
          catalog_number: "CRE001",
        }),
        { url: "http://test.local", token: "phk_test" },
        true,
      );

      expect(result.skipped).toBe(1);
      const link = calls.find(
        (c) => c.method === "POST" && /\/admin\/labels\/4\/releases$/.test(c.url),
      );
      expect(link).toBeDefined();
      expect(link!.body).toMatchObject({ release_id: 10, catalog_number: "CRE001" });
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

  test("shows the catalog number for create actions", () => {
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
            title: "'73 in '83",
            release_type: "single",
            artists: [{ name: "The Legend!" }],
            labels: ["Creation Records"],
            catalog_number: "CRE001",
          },
          action: "create",
          dupCheck: { action: "create", match: "none", fields: [], confidence: 0 },
          resolvedArtists: [{ artist_id: 1, name: "The Legend!", role: "main" }],
          unresolvedArtists: [],
        },
      ]);

      const combined = output.join("");
      expect(combined).toContain("CRE001");
    } finally {
      process.stderr.write = origWrite;
    }
  });
});

describe("linkReleaseLabels", () => {
  interface PostCall {
    path: string;
    body: unknown;
  }

  /** Mock client resolving every label name to a unique id, recording POSTs. */
  function createLabelMockClient(postCalls: PostCall[]): APIClient {
    const client = new APIClient({ url: "http://test.local", token: "phk_test" });
    const ids: Record<string, number> = {};
    let nextId = 100;

    client.get = mock(async (path: string, params?: Record<string, string>) => {
      if (path === "/labels/search" && params?.q) {
        const name = params.q;
        if (!(name in ids)) ids[name] = nextId++;
        return { labels: [{ id: ids[name], name, slug: name.toLowerCase() }] };
      }
      return {};
    }) as typeof client.get;

    client.post = mock(async (path: string, body?: unknown) => {
      postCalls.push({ path, body });
      return {};
    }) as typeof client.post;

    return client;
  }

  function releaseLinks(postCalls: PostCall[]): PostCall[] {
    return postCalls.filter((c) => /\/admin\/labels\/\d+\/releases$/.test(c.path));
  }

  async function quiet<T>(fn: () => Promise<T>): Promise<T> {
    const orig = process.stderr.write;
    process.stderr.write = (() => true) as typeof process.stderr.write;
    try {
      return await fn();
    } finally {
      process.stderr.write = orig;
    }
  }

  async function captureStderr<T>(
    fn: () => Promise<T>,
  ): Promise<{ result: T; output: string }> {
    const orig = process.stderr.write;
    const chunks: string[] = [];
    process.stderr.write = ((s: string) => {
      chunks.push(s);
      return true;
    }) as typeof process.stderr.write;
    try {
      const result = await fn();
      return { result, output: chunks.join("") };
    } finally {
      process.stderr.write = orig;
    }
  }

  test("applies catalog_number when there is exactly one label", async () => {
    const postCalls: PostCall[] = [];
    const client = createLabelMockClient(postCalls);

    await quiet(() =>
      linkReleaseLabels(client, 500, ["Creation Records"], [], "CRE001"),
    );

    const links = releaseLinks(postCalls);
    expect(links).toHaveLength(1);
    expect(links[0].body).toMatchObject({ release_id: 500, catalog_number: "CRE001" });
  });

  test("drops catalog_number when there are multiple labels (ambiguous)", async () => {
    const postCalls: PostCall[] = [];
    const client = createLabelMockClient(postCalls);

    await quiet(() =>
      linkReleaseLabels(client, 500, ["Label A", "Label B"], [], "CRE001"),
    );

    const links = releaseLinks(postCalls);
    expect(links).toHaveLength(2);
    for (const link of links) {
      expect((link.body as Record<string, unknown>).catalog_number).toBeUndefined();
    }
  });

  test("warns (does not silently drop) when a multi-label number is dropped", async () => {
    const postCalls: PostCall[] = [];
    const client = createLabelMockClient(postCalls);

    const { output } = await captureStderr(() =>
      linkReleaseLabels(client, 500, ["Label A", "Label B"], [], "CRE001"),
    );

    expect(output).toContain("not applied");
    expect(output).toContain("CRE001");
  });

  test("dedups case/whitespace-variant label names and keeps the catalog number", async () => {
    const postCalls: PostCall[] = [];
    const client = createLabelMockClient(postCalls);

    // One real label named twice (case/whitespace variant) must NOT be treated
    // as "multiple labels" — label resolution is case-insensitive exact-match.
    await quiet(() =>
      linkReleaseLabels(client, 500, ["Creation Records", " creation records "], [], "CRE001"),
    );

    const links = releaseLinks(postCalls);
    expect(links).toHaveLength(1);
    expect(links[0].body).toMatchObject({ release_id: 500, catalog_number: "CRE001" });
  });

  test("trims surrounding whitespace from the stored catalog number", async () => {
    const postCalls: PostCall[] = [];
    const client = createLabelMockClient(postCalls);

    await quiet(() =>
      linkReleaseLabels(client, 500, ["Creation Records"], [], "  CRE001  "),
    );

    const links = releaseLinks(postCalls);
    expect(links[0].body).toMatchObject({ release_id: 500, catalog_number: "CRE001" });
  });

  test("omits catalog_number when none is provided (no regression)", async () => {
    const postCalls: PostCall[] = [];
    const client = createLabelMockClient(postCalls);

    await quiet(() => linkReleaseLabels(client, 500, ["Creation Records"], []));

    const links = releaseLinks(postCalls);
    expect(links).toHaveLength(1);
    expect((links[0].body as Record<string, unknown>).catalog_number).toBeUndefined();
  });

  test("makes no calls when there are no labels", async () => {
    const postCalls: PostCall[] = [];
    const client = createLabelMockClient(postCalls);

    await quiet(() => linkReleaseLabels(client, 500, undefined, [], "CRE001"));
    await quiet(() => linkReleaseLabels(client, 500, [], [], "CRE001"));

    expect(postCalls).toHaveLength(0);
  });

  test("treats an empty-string catalog_number as absent", async () => {
    const postCalls: PostCall[] = [];
    const client = createLabelMockClient(postCalls);

    await quiet(() => linkReleaseLabels(client, 500, ["Creation Records"], [], ""));

    const links = releaseLinks(postCalls);
    expect(links).toHaveLength(1);
    expect((links[0].body as Record<string, unknown>).catalog_number).toBeUndefined();
  });

  test("treats a whitespace-only catalog_number as absent", async () => {
    const postCalls: PostCall[] = [];
    const client = createLabelMockClient(postCalls);

    await quiet(() => linkReleaseLabels(client, 500, ["Creation Records"], [], "   "));

    const links = releaseLinks(postCalls);
    expect(links).toHaveLength(1);
    expect((links[0].body as Record<string, unknown>).catalog_number).toBeUndefined();
  });

  test("treats a non-string catalog_number as absent (runtime guard)", async () => {
    const postCalls: PostCall[] = [];
    const client = createLabelMockClient(postCalls);

    // Input is JSON.parse'd + cast, so a non-string value can arrive at runtime.
    await quiet(() =>
      linkReleaseLabels(client, 500, ["Creation Records"], [], 123 as unknown as string),
    );

    const links = releaseLinks(postCalls);
    expect(links).toHaveLength(1);
    expect((links[0].body as Record<string, unknown>).catalog_number).toBeUndefined();
  });
});
