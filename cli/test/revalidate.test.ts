import { describe, test, expect, beforeEach, afterEach } from "bun:test";
import {
  chunk,
  flushRevalidation,
  pendingRevalidationEntities,
  recordMutationResponse,
  recordTouchedEntity,
  resetRevalidationQueue,
  revalidationTarget,
} from "../src/lib/revalidate";

const ENV_KEYS = [
  "PH_FRONTEND_URL",
  "PH_INTERNAL_API_SECRET",
  "INTERNAL_API_SECRET",
] as const;

const savedEnv: Record<string, string | undefined> = {};
const realFetch = globalThis.fetch;

beforeEach(() => {
  resetRevalidationQueue();
  for (const key of ENV_KEYS) {
    savedEnv[key] = process.env[key];
    delete process.env[key];
  }
});

afterEach(() => {
  globalThis.fetch = realFetch;
  for (const key of ENV_KEYS) {
    if (savedEnv[key] === undefined) delete process.env[key];
    else process.env[key] = savedEnv[key];
  }
});

/** Record the requests a flush makes, answering each with `status`. */
function captureFetch(status = 200): Array<{ url: string; init: RequestInit }> {
  const calls: Array<{ url: string; init: RequestInit }> = [];
  globalThis.fetch = (async (url: string, init: RequestInit) => {
    calls.push({ url: String(url), init });
    return new Response(JSON.stringify({ revalidated: 1 }), { status });
  }) as unknown as typeof fetch;
  return calls;
}

function bodyEntities(init: RequestInit): Array<{ type: string; slug: string }> {
  return JSON.parse(String(init.body)).entities;
}

describe("recordTouchedEntity", () => {
  test("queues a well-formed entity", () => {
    recordTouchedEntity("show", "a-big-gig");
    expect(pendingRevalidationEntities()).toEqual([
      { type: "show", slug: "a-big-gig" },
    ]);
  });

  test("deduplicates repeats of the same entity", () => {
    recordTouchedEntity("show", "a-big-gig");
    recordTouchedEntity("show", "a-big-gig");
    recordTouchedEntity("artist", "a-big-gig");
    expect(pendingRevalidationEntities()).toHaveLength(2);
  });

  test("drops slugs that are not backend slug shape", () => {
    recordTouchedEntity("show", "../etc/passwd");
    recordTouchedEntity("show", "[slug]");
    recordTouchedEntity("show", "Caps");
    recordTouchedEntity("show", "");
    recordTouchedEntity("show", undefined);
    recordTouchedEntity("show", 42);
    expect(pendingRevalidationEntities()).toEqual([]);
  });
});

describe("recordMutationResponse", () => {
  test("records a created show and its billed artists", () => {
    recordMutationResponse("POST", "/shows", {
      id: 7,
      slug: "a-big-gig",
      artists: [{ id: 1, slug: "bright-eyes" }, { id: 2, slug: "the-band" }],
      venues: [{ id: 3, slug: "the-rebel-lounge" }],
    });

    expect(pendingRevalidationEntities()).toEqual([
      { type: "show", slug: "a-big-gig" },
      { type: "artist", slug: "bright-eyes" },
      { type: "artist", slug: "the-band" },
    ]);
  });

  test("records an updated show", () => {
    recordMutationResponse("PUT", "/shows/12", { slug: "a-big-gig" });
    expect(pendingRevalidationEntities()).toEqual([
      { type: "show", slug: "a-big-gig" },
    ]);
  });

  test("records admin artist create and update", () => {
    recordMutationResponse("POST", "/admin/artists", { slug: "bright-eyes" });
    recordMutationResponse("PATCH", "/admin/artists/4", { slug: "the-band" });
    expect(pendingRevalidationEntities()).toEqual([
      { type: "artist", slug: "bright-eyes" },
      { type: "artist", slug: "the-band" },
    ]);
  });

  test("records venues from both the admin create and the update route", () => {
    recordMutationResponse("POST", "/admin/venues", { slug: "the-rebel-lounge" });
    recordMutationResponse("PUT", "/venues/9", { slug: "valley-bar" });
    expect(pendingRevalidationEntities()).toEqual([
      { type: "venue", slug: "the-rebel-lounge" },
      { type: "venue", slug: "valley-bar" },
    ]);
  });

  test("unwraps an entity nested under its singular name", () => {
    recordMutationResponse("POST", "/labels", { label: { id: 2, slug: "numero" } });
    expect(pendingRevalidationEntities()).toEqual([
      { type: "label", slug: "numero" },
    ]);
  });

  test("records releases and their credited artists", () => {
    recordMutationResponse("POST", "/releases", {
      slug: "satori",
      artists: [{ slug: "flower-travellin-band" }],
    });
    expect(pendingRevalidationEntities()).toEqual([
      { type: "release", slug: "satori" },
      { type: "artist", slug: "flower-travellin-band" },
    ]);
  });

  test("ignores reads, unknown routes, and slugless bodies", () => {
    recordMutationResponse("GET", "/shows", { slug: "a-big-gig" });
    recordMutationResponse("POST", "/auth/login", { slug: "a-big-gig" });
    recordMutationResponse("POST", "/admin/sources/refresh", { slug: "x-y" });
    // Sub-resource routes carry no parent slug — the documented known gap.
    recordMutationResponse("POST", "/festivals/3/artists", { success: true });
    recordMutationResponse("POST", "/admin/labels/3/releases", { success: true });
    recordMutationResponse("DELETE", "/shows/12", {});
    recordMutationResponse("POST", "/shows", null);
    expect(pendingRevalidationEntities()).toEqual([]);
  });
});

describe("revalidationTarget", () => {
  test("is undefined until both env vars are set", () => {
    expect(revalidationTarget()).toBeUndefined();
    process.env.PH_FRONTEND_URL = "https://psychichomily.com";
    expect(revalidationTarget()).toBeUndefined();
    process.env.PH_INTERNAL_API_SECRET = "s".repeat(40);
    expect(revalidationTarget()).toEqual({
      url: "https://psychichomily.com/api/internal/revalidate",
      secret: "s".repeat(40),
    });
  });

  test("strips trailing slashes from the base URL", () => {
    process.env.PH_FRONTEND_URL = "http://localhost:3000///";
    process.env.PH_INTERNAL_API_SECRET = "s".repeat(40);
    expect(revalidationTarget()?.url).toBe(
      "http://localhost:3000/api/internal/revalidate",
    );
  });

  test("falls back to INTERNAL_API_SECRET", () => {
    process.env.PH_FRONTEND_URL = "http://localhost:3000";
    process.env.INTERNAL_API_SECRET = "fallback".repeat(5);
    expect(revalidationTarget()?.secret).toBe("fallback".repeat(5));
  });

  test("prefers PH_INTERNAL_API_SECRET over INTERNAL_API_SECRET", () => {
    process.env.PH_FRONTEND_URL = "http://localhost:3000";
    process.env.PH_INTERNAL_API_SECRET = "preferred".repeat(4);
    process.env.INTERNAL_API_SECRET = "fallback".repeat(5);
    expect(revalidationTarget()?.secret).toBe("preferred".repeat(4));
  });
});

describe("chunk", () => {
  test("splits into fixed-size batches, remainder last", () => {
    expect(chunk([1, 2, 3, 4, 5], 2)).toEqual([[1, 2], [3, 4], [5]]);
    expect(chunk([], 2)).toEqual([]);
    expect(chunk([1, 2], 5)).toEqual([[1, 2]]);
  });
});

describe("flushRevalidation", () => {
  test("does nothing when nothing was touched", async () => {
    const calls = captureFetch();
    await flushRevalidation();
    expect(calls).toHaveLength(0);
  });

  test("skips and clears the queue when unconfigured", async () => {
    const calls = captureFetch();
    recordTouchedEntity("show", "a-big-gig");
    await flushRevalidation();
    expect(calls).toHaveLength(0);
    expect(pendingRevalidationEntities()).toEqual([]);
  });

  test("POSTs the batch with the secret header", async () => {
    process.env.PH_FRONTEND_URL = "http://localhost:3000";
    process.env.PH_INTERNAL_API_SECRET = "s".repeat(40);
    const calls = captureFetch();

    recordTouchedEntity("show", "a-big-gig");
    recordTouchedEntity("artist", "bright-eyes");
    await flushRevalidation();

    expect(calls).toHaveLength(1);
    expect(calls[0].url).toBe("http://localhost:3000/api/internal/revalidate");
    expect(calls[0].init.method).toBe("POST");
    expect(
      (calls[0].init.headers as Record<string, string>)["x-internal-secret"],
    ).toBe("s".repeat(40));
    expect(bodyEntities(calls[0].init)).toEqual([
      { type: "show", slug: "a-big-gig" },
      { type: "artist", slug: "bright-eyes" },
    ]);
    expect(pendingRevalidationEntities()).toEqual([]);
  });

  test("chunks batches larger than the endpoint cap", async () => {
    process.env.PH_FRONTEND_URL = "http://localhost:3000";
    process.env.PH_INTERNAL_API_SECRET = "s".repeat(40);
    const calls = captureFetch();

    for (let i = 0; i < 250; i++) recordTouchedEntity("show", `gig-${i}`);
    await flushRevalidation();

    expect(calls.map((call) => bodyEntities(call.init).length)).toEqual([
      100, 100, 50,
    ]);
  });

  test("swallows a non-2xx response", async () => {
    process.env.PH_FRONTEND_URL = "http://localhost:3000";
    process.env.PH_INTERNAL_API_SECRET = "s".repeat(40);
    captureFetch(401);

    recordTouchedEntity("show", "a-big-gig");
    await flushRevalidation();
    expect(pendingRevalidationEntities()).toEqual([]);
  });

  test("swallows a transport failure", async () => {
    process.env.PH_FRONTEND_URL = "http://localhost:3000";
    process.env.PH_INTERNAL_API_SECRET = "s".repeat(40);
    globalThis.fetch = (async () => {
      throw new Error("ECONNREFUSED");
    }) as unknown as typeof fetch;

    recordTouchedEntity("show", "a-big-gig");
    await flushRevalidation();
    expect(pendingRevalidationEntities()).toEqual([]);
  });
});
