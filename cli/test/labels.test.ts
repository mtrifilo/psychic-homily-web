import { describe, test, expect, mock } from "bun:test";
import {
  isPlaceholderLabelName,
  linkReleaseToLabel,
  placeholderLabelWarning,
  resolveAndLinkReleaseLabel,
} from "../src/lib/labels";
import { APIClient } from "../src/lib/api";

// -- Helpers --

interface PostCall {
  path: string;
  body: unknown;
}

/**
 * Mock APIClient that resolves `/labels/search` to a configurable list and
 * records every POST so tests can assert on the request bodies.
 */
function createMockClient(opts: {
  labels?: Array<{ id: number; name: string; slug: string }>;
  postCalls: PostCall[];
}): APIClient {
  const client = new APIClient({ url: "http://test.local", token: "phk_test" });

  client.get = mock(async (path: string) => {
    if (path === "/labels/search") {
      return { labels: opts.labels ?? [] };
    }
    return {};
  }) as typeof client.get;

  client.post = mock(async (path: string, body?: unknown) => {
    opts.postCalls.push({ path, body });
    return {};
  }) as typeof client.post;

  return client;
}

/** Run `fn` with stderr suppressed (display.* writes there). */
async function quiet<T>(fn: () => Promise<T>): Promise<T> {
  const orig = process.stderr.write;
  process.stderr.write = (() => true) as typeof process.stderr.write;
  try {
    return await fn();
  } finally {
    process.stderr.write = orig;
  }
}

// -- Tests --

describe("isPlaceholderLabelName", () => {
  test.each([
    "3245973 Records DK",
    "12345 Records",
    "3245973 records dk",
    "  3245973   Records   DK  ",
  ])("flags the distributor placeholder shape: %p", (name: string) => {
    expect(isPlaceholderLabelName(name)).toBe(true);
  });

  test.each([
    "4AD",
    "12XU",
    "1-800 Records",
    "Records DK",
    "2000 Records",
    "1234 Records DK",
    "3245973 Records DKX",
    "3245973 Records D1",
    "3245973 Recordings",
    "Label 3245973 Records",
  ])("does not flag a real or near-miss name: %p", (name: string) => {
    expect(isPlaceholderLabelName(name)).toBe(false);
  });
});

describe("placeholderLabelWarning", () => {
  test("names the label and the reason for a placeholder", () => {
    const warning = placeholderLabelWarning("3245973 Records DK");
    expect(warning).toContain('"3245973 Records DK"');
    expect(warning).toContain("distributor placeholder");
    expect(warning).toContain('five or more digits, then "Records"');
  });

  test("returns null for a real label or a non-string", () => {
    expect(placeholderLabelWarning("4AD")).toBeNull();
    expect(placeholderLabelWarning(12345)).toBeNull();
    expect(placeholderLabelWarning(undefined)).toBeNull();
  });
});

describe("linkReleaseToLabel", () => {
  test("includes catalog_number in the POST body when provided", async () => {
    const postCalls: PostCall[] = [];
    const client = createMockClient({ postCalls });

    await quiet(() => linkReleaseToLabel(client, 4, 100, "CRE001"));

    expect(postCalls).toHaveLength(1);
    expect(postCalls[0].path).toBe("/admin/labels/4/releases");
    expect(postCalls[0].body).toEqual({ release_id: 100, catalog_number: "CRE001" });
  });

  test("omits catalog_number when not provided", async () => {
    const postCalls: PostCall[] = [];
    const client = createMockClient({ postCalls });

    await quiet(() => linkReleaseToLabel(client, 4, 100));

    expect(postCalls).toHaveLength(1);
    expect(postCalls[0].body).toEqual({ release_id: 100 });
  });

  test("sends overwrite_catalog_number when overwrite is set with a catalog number", async () => {
    const postCalls: PostCall[] = [];
    const client = createMockClient({ postCalls });

    await quiet(() => linkReleaseToLabel(client, 4, 100, "CRE001", true));

    expect(postCalls).toHaveLength(1);
    expect(postCalls[0].body).toEqual({
      release_id: 100,
      catalog_number: "CRE001",
      overwrite_catalog_number: true,
    });
  });

  test("does not send overwrite_catalog_number without a catalog number (no-op overwrite)", async () => {
    const postCalls: PostCall[] = [];
    const client = createMockClient({ postCalls });

    await quiet(() => linkReleaseToLabel(client, 4, 100, undefined, true));

    expect(postCalls).toHaveLength(1);
    expect(postCalls[0].body).toEqual({ release_id: 100 });
  });

  test("omits overwrite_catalog_number when overwrite is false", async () => {
    const postCalls: PostCall[] = [];
    const client = createMockClient({ postCalls });

    await quiet(() => linkReleaseToLabel(client, 4, 100, "CRE001", false));

    expect(postCalls).toHaveLength(1);
    expect(postCalls[0].body).toEqual({ release_id: 100, catalog_number: "CRE001" });
  });
});

describe("resolveAndLinkReleaseLabel", () => {
  test("resolves the label by name and passes catalog_number through", async () => {
    const postCalls: PostCall[] = [];
    const client = createMockClient({
      labels: [{ id: 4, name: "Creation Records", slug: "creation-records" }],
      postCalls,
    });

    const result = await quiet(() =>
      resolveAndLinkReleaseLabel(client, "Creation Records", 100, [], "CRE001"),
    );

    expect(result).toBe(4);
    const releaseLink = postCalls.find((c) => c.path === "/admin/labels/4/releases");
    expect(releaseLink?.body).toEqual({ release_id: 100, catalog_number: "CRE001" });
  });

  test("returns null and makes no link POST when the label is not found", async () => {
    const postCalls: PostCall[] = [];
    const client = createMockClient({ labels: [], postCalls });

    const result = await quiet(() =>
      resolveAndLinkReleaseLabel(client, "Nonexistent Label", 100, [], "CRE001"),
    );

    expect(result).toBeNull();
    expect(postCalls).toHaveLength(0);
  });

  test("threads the overwrite flag through to the link POST", async () => {
    const postCalls: PostCall[] = [];
    const client = createMockClient({
      labels: [{ id: 4, name: "Creation Records", slug: "creation-records" }],
      postCalls,
    });

    await quiet(() =>
      resolveAndLinkReleaseLabel(client, "Creation Records", 100, [], "CRE001", true),
    );

    const releaseLink = postCalls.find((c) => c.path === "/admin/labels/4/releases");
    expect(releaseLink?.body).toEqual({
      release_id: 100,
      catalog_number: "CRE001",
      overwrite_catalog_number: true,
    });
  });
});
