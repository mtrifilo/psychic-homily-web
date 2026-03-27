import { describe, test, expect, mock, beforeEach } from "bun:test";
import { submitLabels, parseInput } from "../src/commands/submit-label";
import type { APIClient } from "../src/lib/api";

/** Create a mock API client with controllable get/post/put responses. */
function createMockClient(overrides: {
  getResponse?: unknown;
  postResponse?: unknown;
  putResponse?: unknown;
} = {}): APIClient {
  return {
    get: mock(() => Promise.resolve(overrides.getResponse ?? { labels: [] })),
    post: mock(() => Promise.resolve(overrides.postResponse ?? { success: true, label: { id: 1 } })),
    put: mock(() => Promise.resolve(overrides.putResponse ?? { success: true, label: { id: 1 } })),
  } as unknown as APIClient;
}

describe("submitLabels", () => {
  beforeEach(() => {
    // Silence display output during tests
    mock.module("../src/lib/display", () => ({
      header: () => {},
      success: () => {},
      warn: () => {},
      error: () => {},
      info: () => {},
      kv: () => {},
      table: () => {},
      fieldDiff: () => {},
      summary: () => {},
    }));
  });

  test("creates a single new label (dry-run)", async () => {
    const client = createMockClient();
    const items = [{ name: "Numero", city: "Chicago", state: "IL" }];

    const result = await submitLabels(items, client, false);

    expect(result.created).toBe(1);
    expect(result.updated).toBe(0);
    expect(result.skipped).toBe(0);
    expect(result.errors).toBe(0);
    // In dry-run mode, no POST calls should be made
    expect((client.post as ReturnType<typeof mock>).mock.calls.length).toBe(0);
  });

  test("creates a single new label (--confirm)", async () => {
    const client = createMockClient();
    const items = [{ name: "Numero", city: "Chicago", state: "IL" }];

    const result = await submitLabels(items, client, true);

    expect(result.created).toBe(1);
    expect(result.updated).toBe(0);
    expect(result.skipped).toBe(0);
    expect(result.errors).toBe(0);
    expect((client.post as ReturnType<typeof mock>).mock.calls.length).toBe(1);
    // Verify the POST payload
    const postCall = (client.post as ReturnType<typeof mock>).mock.calls[0];
    expect(postCall[0]).toBe("/labels");
    // Payload should contain only API-accepted fields (tags, entity_type, etc. stripped)
    expect(postCall[1]).toEqual({ name: "Numero", city: "Chicago", state: "IL" });
  });

  test("updates a label with new website info (--confirm)", async () => {
    const client = createMockClient({
      getResponse: {
        labels: [
          {
            id: 42,
            name: "Numero Group",
            slug: "numero-group",
            city: "Chicago",
            state: "IL",
            country: "",
            website: "",
            description: "",
            bandcamp_url: "",
          },
        ],
      },
    });

    const items = [
      {
        name: "Numero Group",
        city: "Chicago",
        state: "IL",
        website: "https://numerogroup.com",
      },
    ];

    const result = await submitLabels(items, client, true);

    expect(result.updated).toBe(1);
    expect(result.created).toBe(0);
    expect(result.skipped).toBe(0);
    expect(result.errors).toBe(0);
    // Should PUT only the new_info field (website)
    expect((client.put as ReturnType<typeof mock>).mock.calls.length).toBe(1);
    const putCall = (client.put as ReturnType<typeof mock>).mock.calls[0];
    expect(putCall[0]).toBe("/labels/42");
    expect(putCall[1]).toEqual({ website: "https://numerogroup.com" });
  });

  test("skips a label that already exists with no new info", async () => {
    const client = createMockClient({
      getResponse: {
        labels: [
          {
            id: 42,
            name: "Sacred Bones Records",
            slug: "sacred-bones-records",
            city: "Brooklyn",
            state: "NY",
            country: "",
            website: "https://sacredbonesrecords.com",
            description: "",
            bandcamp_url: "",
          },
        ],
      },
    });

    const items = [
      {
        name: "Sacred Bones Records",
        city: "Brooklyn",
        state: "NY",
        website: "https://sacredbonesrecords.com",
      },
    ];

    const result = await submitLabels(items, client, true);

    expect(result.skipped).toBe(1);
    expect(result.created).toBe(0);
    expect(result.updated).toBe(0);
    // No POST or PUT calls for skipped items
    expect((client.post as ReturnType<typeof mock>).mock.calls.length).toBe(0);
    expect((client.put as ReturnType<typeof mock>).mock.calls.length).toBe(0);
  });

  test("handles minimal label (name-only, WFMU-style)", async () => {
    const client = createMockClient();
    const items = [{ name: "Drag City" }];

    const result = await submitLabels(items, client, false);

    expect(result.created).toBe(1);
    expect(result.errors).toBe(0);
  });

  test("reports validation error for missing name", async () => {
    const client = createMockClient();
    const items = [{ city: "Chicago", state: "IL" }];

    const result = await submitLabels(items, client, false);

    expect(result.errors).toBe(1);
    expect(result.created).toBe(0);
  });

  test("processes array of mixed labels", async () => {
    const client = createMockClient({
      getResponse: { labels: [] },
    });

    const items = [
      { name: "Label A", city: "Phoenix", state: "AZ" },
      { name: "Label B" },
      { city: "Missing Name" }, // invalid
    ];

    const result = await submitLabels(items, client, false);

    expect(result.created).toBe(2);
    expect(result.errors).toBe(1);
  });

  test("dry-run does not call POST or PUT", async () => {
    const client = createMockClient({
      getResponse: {
        labels: [
          {
            id: 1,
            name: "Existing",
            slug: "existing",
            city: "",
            state: "",
            country: "",
            website: "",
            description: "",
            bandcamp_url: "",
          },
        ],
      },
    });

    const items = [
      { name: "New Label" },
      { name: "Existing", website: "https://new.com" },
    ];

    const result = await submitLabels(items, client, false);

    expect(result.created + result.updated).toBeGreaterThan(0);
    expect((client.post as ReturnType<typeof mock>).mock.calls.length).toBe(0);
    expect((client.put as ReturnType<typeof mock>).mock.calls.length).toBe(0);
  });

  test("handles API error during create gracefully", async () => {
    const client = createMockClient();
    (client.post as ReturnType<typeof mock>).mockImplementation(() =>
      Promise.reject(new Error("403 Forbidden")),
    );

    const items = [{ name: "Test Label" }];

    const result = await submitLabels(items, client, true);

    expect(result.errors).toBe(1);
    expect(result.created).toBe(0);
  });

  test("handles API error during update gracefully", async () => {
    const client = createMockClient({
      getResponse: {
        labels: [
          {
            id: 5,
            name: "Test Label",
            slug: "test-label",
            city: "",
            state: "",
            country: "",
            website: "",
            description: "",
            bandcamp_url: "",
          },
        ],
      },
    });
    (client.put as ReturnType<typeof mock>).mockImplementation(() =>
      Promise.reject(new Error("500 Internal Server Error")),
    );

    const items = [{ name: "Test Label", website: "https://new.com" }];

    const result = await submitLabels(items, client, true);

    expect(result.errors).toBe(1);
    expect(result.updated).toBe(0);
  });
});

describe("parseInput", () => {
  test("parses single JSON object from argument", async () => {
    const items = await parseInput('{"name": "Test"}');
    expect(items).toEqual([{ name: "Test" }]);
  });

  test("parses JSON array from argument", async () => {
    const items = await parseInput('[{"name": "A"}, {"name": "B"}]');
    expect(items).toHaveLength(2);
    expect(items[0]).toEqual({ name: "A" });
    expect(items[1]).toEqual({ name: "B" });
  });

  test("throws on invalid JSON", async () => {
    await expect(parseInput("not json")).rejects.toThrow();
  });

  test("throws on empty string", async () => {
    await expect(parseInput("")).rejects.toThrow("No JSON input provided");
  });
});
