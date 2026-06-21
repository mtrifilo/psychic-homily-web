import { describe, test, expect, mock } from "bun:test";
import {
  parseEntity,
  listStale,
  registerSource,
  recordRefresh,
  type SourceConfigInfo,
} from "../src/commands/sources";
import type { APIClient } from "../src/lib/api";

function sample(overrides: Partial<SourceConfigInfo> = {}): SourceConfigInfo {
  return {
    id: 1,
    entity_type: "label",
    entity_id: 5,
    source_url: "https://x.com",
    last_refreshed_at: null,
    last_content_hash: null,
    consecutive_failures: 0,
    created_at: "2026-06-21T00:00:00Z",
    updated_at: "2026-06-21T00:00:00Z",
    ...overrides,
  };
}

function createMockClient(overrides: {
  get?: (path: string, params?: Record<string, string>) => Promise<unknown>;
  put?: (path: string, body?: unknown) => Promise<unknown>;
  post?: (path: string, body?: unknown) => Promise<unknown>;
} = {}): APIClient {
  return {
    get: overrides.get ?? mock(() => Promise.resolve({ sources: [], count: 0 })),
    put: overrides.put ?? mock(() => Promise.resolve(sample())),
    post: overrides.post ?? mock(() => Promise.resolve(sample())),
  } as unknown as APIClient;
}

describe("parseEntity", () => {
  test("accepts valid venue/label", () => {
    expect(parseEntity("venue", "7")).toEqual({ entityType: "venue", entityId: 7 });
    expect(parseEntity("label", "5")).toEqual({ entityType: "label", entityId: 5 });
  });

  test("rejects unknown entity type", () => {
    expect(() => parseEntity("artist", "1")).toThrow(/Invalid entity type/);
  });

  test("rejects non-positive / non-integer id", () => {
    expect(() => parseEntity("label", "0")).toThrow(/positive integer/);
    expect(() => parseEntity("label", "-3")).toThrow(/positive integer/);
    expect(() => parseEntity("label", "abc")).toThrow(/positive integer/);
  });
});

describe("listStale", () => {
  test("passes limit + max_failures query params", async () => {
    const getFn = mock(() => Promise.resolve({ sources: [sample()], count: 1 }));
    const client = createMockClient({ get: getFn });

    const out = await listStale(client, { limit: "10", maxFailures: "5" });

    expect(getFn).toHaveBeenCalledWith("/admin/sources", { limit: "10", max_failures: "5" });
    expect(out).toHaveLength(1);
  });

  test("omits empty params", async () => {
    const getFn = mock(() => Promise.resolve({ sources: [], count: 0 }));
    const client = createMockClient({ get: getFn });

    await listStale(client, {});

    expect(getFn).toHaveBeenCalledWith("/admin/sources", {});
  });

  test("tolerates a missing sources field", async () => {
    const client = createMockClient({ get: mock(() => Promise.resolve({})) });
    const out = await listStale(client, {});
    expect(out).toEqual([]);
  });
});

describe("registerSource", () => {
  test("PUTs entity + source_url", async () => {
    const putFn = mock(() => Promise.resolve(sample()));
    const client = createMockClient({ put: putFn });

    await registerSource(client, "label", "5", "https://sacredbonesrecords.com/pages/artists");

    expect(putFn).toHaveBeenCalledWith("/admin/sources", {
      entity_type: "label",
      entity_id: 5,
      source_url: "https://sacredbonesrecords.com/pages/artists",
    });
  });

  test("omits source_url when not provided", async () => {
    const putFn = mock(() => Promise.resolve(sample({ source_url: null })));
    const client = createMockClient({ put: putFn });

    await registerSource(client, "venue", "9");

    expect(putFn).toHaveBeenCalledWith("/admin/sources", { entity_type: "venue", entity_id: 9 });
  });

  test("rejects invalid entity type before calling the API", async () => {
    const putFn = mock(() => Promise.resolve(sample()));
    const client = createMockClient({ put: putFn });

    await expect(registerSource(client, "artist", "1")).rejects.toThrow(/Invalid entity type/);
    expect(putFn).not.toHaveBeenCalled();
  });
});

describe("recordRefresh", () => {
  test("POSTs entity + content_hash", async () => {
    const postFn = mock(() => Promise.resolve(sample()));
    const client = createMockClient({ post: postFn });

    await recordRefresh(client, "venue", "7", "deadbeef");

    expect(postFn).toHaveBeenCalledWith("/admin/sources/refresh", {
      entity_type: "venue",
      entity_id: 7,
      content_hash: "deadbeef",
    });
  });

  test("omits content_hash when not provided", async () => {
    const postFn = mock(() => Promise.resolve(sample()));
    const client = createMockClient({ post: postFn });

    await recordRefresh(client, "label", "5");

    expect(postFn).toHaveBeenCalledWith("/admin/sources/refresh", { entity_type: "label", entity_id: 5 });
  });
});
