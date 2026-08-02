import * as display from "./display";

/**
 * Push ISR revalidation to the Next.js app after an ingest run (PSY-1691).
 *
 * The CLI writes straight to the Go API, so it bypasses the frontend proxy
 * that normally revalidates ISR pages after a mutation. Entity detail pages do
 * not time-revalidate, so without this an ingested show stays invisible on its
 * own page until the next deploy.
 *
 * Shape of the wiring:
 *   1. `APIClient` records every successful catalog mutation here (one choke
 *      point, so no submit command has to remember to call it).
 *   2. `cli.ts` calls `flushRevalidation()` once when the process finishes,
 *      POSTing the deduplicated batch to /api/internal/revalidate.
 *
 * Fire-and-forget, mirroring the backend's Discord/audit posture: every
 * failure is logged and swallowed. A revalidation problem must never turn a
 * successful ingest into a failed one — the data is already persisted, and the
 * worst case is the pre-existing staleness this feature is fixing.
 *
 * Known gap (follow-up): sub-resource mutations whose response carries no slug
 * — label roster links (POST /admin/labels/{id}/artists|releases), festival
 * lineup links, entity tagging — record nothing, because the parent slug is
 * not in the response. Those pages keep their previous staleness behaviour.
 */

/** Environment variable naming the Next.js app to notify. */
const FRONTEND_URL_ENV = "PH_FRONTEND_URL";

/**
 * Environment variables holding the shared secret, in precedence order.
 * `INTERNAL_API_SECRET` is the name the backend and Vercel already use, so it
 * is accepted as a fallback for operators who export it once.
 */
const SECRET_ENVS = ["PH_INTERNAL_API_SECRET", "INTERNAL_API_SECRET"] as const;

/** Must match MAX_ENTITIES_PER_REQUEST in frontend/lib/internal-revalidate.ts. */
const MAX_ENTITIES_PER_REQUEST = 100;

const REQUEST_TIMEOUT_MS = 15_000;

/**
 * Backend slug shape (backend/internal/utils/slug.go). Validated here as well
 * as on the endpoint so obvious junk never leaves the CLI.
 */
const SLUG_PATTERN = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;

const MUTATION_METHODS = new Set(["POST", "PUT", "PATCH", "DELETE"]);

/** One entity this run created or updated. */
export interface TouchedEntity {
  /** Singular entity type accepted by the revalidate endpoint. */
  type: string;
  slug: string;
}

/**
 * Backend mutation paths → the entity type they touch.
 *
 * Only routes whose response body carries the entity (and therefore its slug)
 * are listed; see the known gap in the module comment. Sub-resource routes
 * such as /festivals/{id}/artists deliberately do not match.
 */
const ENTITY_PATH_RULES: ReadonlyArray<{ pattern: RegExp; type: string }> = [
  { pattern: /^\/shows(?:\/\d+)?$/, type: "show" },
  { pattern: /^\/admin\/artists(?:\/\d+)?$/, type: "artist" },
  { pattern: /^\/admin\/venues(?:\/\d+)?$/, type: "venue" },
  { pattern: /^\/venues\/\d+$/, type: "venue" },
  { pattern: /^\/releases(?:\/\d+)?$/, type: "release" },
  { pattern: /^\/labels(?:\/\d+)?$/, type: "label" },
  { pattern: /^\/festivals(?:\/\d+)?$/, type: "festival" },
  { pattern: /^\/tags(?:\/\d+)?$/, type: "tag" },
];

/** Types whose response embeds a billed/credited artist list worth recording. */
const NESTS_ARTISTS = new Set(["show", "release"]);

// Keyed "type:slug" so the same entity touched by several commands in one run
// is sent once. A Map preserves insertion order, which keeps logs readable.
const queue = new Map<string, TouchedEntity>();

/** Queue one entity for revalidation. Unknown or malformed slugs are dropped. */
export function recordTouchedEntity(type: string, slug: unknown): void {
  if (typeof slug !== "string" || !SLUG_PATTERN.test(slug)) return;
  queue.set(`${type}:${slug}`, { type, slug });
}

/** Entities queued so far. Exposed for tests and for verbose reporting. */
export function pendingRevalidationEntities(): TouchedEntity[] {
  return [...queue.values()];
}

/** Drop everything queued. Test helper — the CLI queue is process-scoped. */
export function resetRevalidationQueue(): void {
  queue.clear();
}

/**
 * Record whatever a successful mutation response tells us about touched
 * entities. Called by APIClient for every 2xx mutation; unrecognised paths and
 * slugless bodies are silently ignored.
 */
export function recordMutationResponse(
  method: string,
  path: string,
  body: unknown,
): void {
  if (!MUTATION_METHODS.has(method.toUpperCase())) return;

  const rule = ENTITY_PATH_RULES.find((candidate) =>
    candidate.pattern.test(path),
  );
  if (!rule) return;

  // Responses are either the entity itself or the entity wrapped under its
  // singular name ({"artist": {...}}) — both shapes exist in this API.
  const entity = unwrapEntity(body, rule.type);
  if (!entity) return;

  recordTouchedEntity(rule.type, entity.slug);

  // A show's bill and a release's credits change those artists' detail pages
  // (both ISR-cache per-artist stats), so they are part of the same batch.
  if (NESTS_ARTISTS.has(rule.type)) {
    for (const artist of asArray(entity.artists)) {
      recordTouchedEntity("artist", asRecord(artist)?.slug);
    }
  }
}

/**
 * POST every queued entity to the frontend's internal revalidate endpoint,
 * then clear the queue.
 *
 * A no-op when nothing was touched, or when the endpoint is not configured
 * (the normal case in local development) — the skip is logged, never fatal.
 */
export async function flushRevalidation(): Promise<void> {
  const entities = pendingRevalidationEntities();
  resetRevalidationQueue();
  if (entities.length === 0) return;

  const target = revalidationTarget();
  if (!target) {
    display.info(
      `Skipping ISR revalidation for ${pluralize(entities.length)}: ` +
        `set ${FRONTEND_URL_ENV} and ${SECRET_ENVS[0]} to enable.`,
    );
    return;
  }

  for (const batch of chunk(entities, MAX_ENTITIES_PER_REQUEST)) {
    await postBatch(target, batch);
  }
}

/** The configured endpoint, or undefined when either env var is missing. */
export function revalidationTarget():
  | { url: string; secret: string }
  | undefined {
  const base = process.env[FRONTEND_URL_ENV]?.trim();
  const secret = SECRET_ENVS.map((name) => process.env[name]?.trim()).find(
    (value): value is string => !!value,
  );
  if (!base || !secret) return undefined;

  return {
    url: `${base.replace(/\/+$/, "")}/api/internal/revalidate`,
    secret,
  };
}

/** Split a list into fixed-size batches. */
export function chunk<T>(items: readonly T[], size: number): T[][] {
  const batches: T[][] = [];
  for (let i = 0; i < items.length; i += size) {
    batches.push(items.slice(i, i + size));
  }
  return batches;
}

async function postBatch(
  target: { url: string; secret: string },
  entities: TouchedEntity[],
): Promise<void> {
  try {
    const response = await fetch(target.url, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "x-internal-secret": target.secret,
        "User-Agent": "ph-cli/0.1.0",
      },
      body: JSON.stringify({ entities }),
      signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS),
    });

    if (!response.ok) {
      display.warn(
        `ISR revalidation failed for ${pluralize(entities.length)} (HTTP ${response.status}). ` +
          `Pages stay stale until their next revalidation.`,
      );
      return;
    }

    display.info(
      `Revalidated ${await describeOutcome(response)} for ${pluralize(entities.length)}.`,
    );
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    display.warn(
      `ISR revalidation request failed for ${pluralize(entities.length)}: ${message}`,
    );
  }
}

function pluralize(count: number): string {
  return `${count} entit${count === 1 ? "y" : "ies"}`;
}

/**
 * "N ISR paths" from the endpoint's own count, so the operator sees what the
 * endpoint DID rather than what the CLI asked for — the two differ when the
 * endpoint skips entries it cannot use. Degrades to a generic phrase if the
 * response is not the shape we expect.
 */
async function describeOutcome(response: Response): Promise<string> {
  try {
    const summary = asRecord(await response.json());
    if (typeof summary?.paths !== "number") return "ISR pages";
    const skipped =
      typeof summary.skipped === "number" && summary.skipped > 0
        ? `, ${summary.skipped} skipped`
        : "";
    return `${summary.paths} ISR path${summary.paths === 1 ? "" : "s"}${skipped}`;
  } catch {
    return "ISR pages";
  }
}

// -- Response shape helpers --------------------------------------------------

function asRecord(value: unknown): Record<string, unknown> | undefined {
  return typeof value === "object" && value !== null
    ? (value as Record<string, unknown>)
    : undefined;
}

function asArray(value: unknown): unknown[] {
  return Array.isArray(value) ? value : [];
}

/** The entity record itself, whether it is the body or nested under `type`. */
function unwrapEntity(
  body: unknown,
  type: string,
): Record<string, unknown> | undefined {
  const record = asRecord(body);
  if (!record) return undefined;
  if (typeof record.slug === "string") return record;
  return asRecord(record[type]);
}
