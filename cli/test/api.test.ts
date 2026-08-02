import { describe, test, expect, beforeEach, afterEach } from "bun:test";
import { APIClient, APIError } from "../src/lib/api";
import {
  pendingRevalidationEntities,
  resetRevalidationQueue,
} from "../src/lib/revalidate";

describe("APIClient", () => {
  test("constructs with environment config", () => {
    const client = new APIClient({
      url: "https://api.psychichomily.com",
      token: "phk_test123",
    });
    expect(client).toBeDefined();
  });

  test("strips trailing slash from base URL", () => {
    // We can't easily inspect the private baseUrl, but we can verify
    // the client doesn't crash on construction with trailing slashes
    const client = new APIClient({
      url: "https://api.psychichomily.com///",
      token: "phk_test",
    });
    expect(client).toBeDefined();
  });

  test("healthCheck returns false for unreachable host", async () => {
    const client = new APIClient({
      url: "http://localhost:19999",
      token: "phk_test",
    });
    const result = await client.healthCheck();
    expect(result).toBe(false);
  });
});

describe("APIClient ISR revalidation recording (PSY-1691)", () => {
  const realFetch = globalThis.fetch;

  beforeEach(() => {
    resetRevalidationQueue();
  });

  afterEach(() => {
    globalThis.fetch = realFetch;
    resetRevalidationQueue();
  });

  function respondWith(body: unknown, status = 200): void {
    globalThis.fetch = (async () =>
      new Response(JSON.stringify(body), {
        status,
        headers: { "content-type": "application/json" },
      })) as unknown as typeof fetch;
  }

  const client = new APIClient({
    url: "http://localhost:8080",
    token: "phk_test",
  });

  test("queues the entity a successful mutation returned", async () => {
    respondWith({ id: 7, slug: "a-big-gig", artists: [{ slug: "the-band" }] });

    await client.post("/shows", { title: "A Big Gig" });

    expect(pendingRevalidationEntities()).toEqual([
      { type: "show", slug: "a-big-gig" },
      { type: "artist", slug: "the-band" },
    ]);
  });

  test("queues nothing for a read", async () => {
    respondWith({ id: 7, slug: "a-big-gig" });
    await client.get("/shows/7");
    expect(pendingRevalidationEntities()).toEqual([]);
  });

  test("queues nothing when the mutation failed", async () => {
    respondWith({ message: "nope" }, 422);
    await expect(client.post("/shows", {})).rejects.toBeInstanceOf(APIError);
    expect(pendingRevalidationEntities()).toEqual([]);
  });
});

describe("APIError", () => {
  test("includes status and error code", () => {
    const err = new APIError(422, "validation_failed", "Name is required", "req_123");
    expect(err.status).toBe(422);
    expect(err.errorCode).toBe("validation_failed");
    expect(err.message).toBe("Name is required");
    expect(err.requestId).toBe("req_123");
    expect(err.name).toBe("APIError");
  });

  test("is an instance of Error", () => {
    const err = new APIError(500, undefined, "Server error");
    expect(err).toBeInstanceOf(Error);
    expect(err).toBeInstanceOf(APIError);
  });
});
