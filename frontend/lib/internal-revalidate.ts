import { createHash, timingSafeEqual } from 'node:crypto'
import * as Sentry from '@sentry/nextjs'
import { safeRevalidatePath } from './revalidate-entity'
import { OUT_OF_BAND_SEGMENTS, outOfBandPages } from './revalidation-paths'

/**
 * Auth, validation and fan-out for the out-of-band ISR revalidation endpoint
 * (PSY-1691). The route handler at app/api/internal/revalidate/route.ts is a
 * thin shell over this module — Next.js route files may only export handlers
 * and segment config, so the testable pieces live here.
 *
 * Context: browser mutations go through app/api/[...path]/route.ts, which
 * revalidates affected pages via lib/proxy-revalidation.ts. Backend-originated
 * writes — the `ph` ingest CLI today, discovery/radio jobs later — talk to the
 * Go API directly, so nothing revalidates and entity detail pages (which do
 * not time-revalidate) serve pre-write HTML until the next deploy.
 *
 * Callers are fire-and-forget: they log failures and never fail the write that
 * triggered them. So this module is deliberately lenient about the CONTENTS of
 * a batch (unusable entries are skipped and counted, so one bad slug cannot
 * drop a whole ingest run's revalidation) and strict about its SHAPE (a
 * malformed envelope is a caller bug and answers 400).
 */

/** Header carrying the shared secret, matching the backend's admin bypass. */
export const SECRET_HEADER = 'x-internal-secret'

/**
 * Max entities per request. Callers chunk larger runs. Bounds the work one
 * request can queue and keeps the body small enough to read into memory.
 */
export const MAX_ENTITIES_PER_REQUEST = 100

/** Max request body bytes. 100 entries of {type, slug} is well under 16 KB. */
export const MAX_BODY_BYTES = 64 * 1024

/** Longest slug accepted. Backend slugs are far shorter; this is a guard. */
const MAX_SLUG_LENGTH = 200

/**
 * Backend slugs are lowercase alphanumerics joined by single hyphens, with no
 * leading or trailing hyphen (backend/internal/utils/slug.go). Enforcing that
 * shape is what keeps `/`, `.`, `%` and `[` out of the string that reaches
 * revalidatePath — no traversal to another route, and no way to smuggle a
 * `[slug]` route pattern in and blow away a whole route's cache.
 */
const SLUG_PATTERN = /^[a-z0-9]+(?:-[a-z0-9]+)*$/

/** One entity an out-of-band write touched. */
export interface TouchedEntity {
  /** Singular entity type: a key of OUT_OF_BAND_SEGMENTS. */
  type: string
  slug: string
}

export interface RevalidateResult {
  /** Entities accepted and resolved to paths. */
  revalidated: number
  /** Entities dropped as unusable (unknown type or malformed slug). */
  skipped: number
  /** Distinct paths revalidated. */
  paths: number
}

// ---------------------------------------------------------------------------
// Auth
// ---------------------------------------------------------------------------

/**
 * Constant-time check of the caller's secret against INTERNAL_API_SECRET.
 *
 * Mirrors the backend's `matchesInternalSecret`
 * (internal/api/handlers/catalog/artist.go): the same shared secret, the same
 * constant-time semantics, and an unconfigured secret NEVER matches — so this
 * endpoint is inert rather than open when the env var is missing.
 *
 * Both sides are SHA-256'd before comparison because `timingSafeEqual` throws
 * on length mismatch, and returning early on that would leak the secret's
 * length. Fixed-width digests make every comparison take the same path.
 */
export function isAuthorized(provided: string | null | undefined): boolean {
  const configured = process.env.INTERNAL_API_SECRET
  if (!configured) {
    Sentry.captureMessage(
      'internal-revalidate: INTERNAL_API_SECRET unset, rejecting',
      {
        level: 'warning',
        tags: { service: 'isr-revalidation', source: 'internal-revalidate' },
      }
    )
    return false
  }
  if (!provided) return false

  return timingSafeEqual(sha256(provided), sha256(configured))
}

function sha256(value: string): Buffer {
  return createHash('sha256').update(value, 'utf8').digest()
}

// ---------------------------------------------------------------------------
// Request parsing
// ---------------------------------------------------------------------------

/**
 * Shape-check the request envelope.
 *
 * Only the envelope is validated here; individual entries are filtered during
 * fan-out so that one malformed entity does not discard the rest of the batch.
 */
export function parseRevalidateBody(
  text: string
): { entities: unknown[] } | { error: string } {
  if (Buffer.byteLength(text, 'utf8') > MAX_BODY_BYTES) {
    return { error: `Body exceeds ${MAX_BODY_BYTES} bytes` }
  }

  let body: unknown
  try {
    body = JSON.parse(text)
  } catch {
    return { error: 'Invalid JSON body' }
  }

  const entities =
    typeof body === 'object' && body !== null
      ? (body as Record<string, unknown>).entities
      : undefined
  if (!Array.isArray(entities)) {
    return { error: 'Expected an "entities" array' }
  }
  if (entities.length > MAX_ENTITIES_PER_REQUEST) {
    return {
      error: `Too many entities (max ${MAX_ENTITIES_PER_REQUEST} per request)`,
    }
  }

  return { entities }
}

/** A usable {type, slug} pair, or undefined when the entry is unusable. */
function toTouchedEntity(value: unknown): TouchedEntity | undefined {
  if (typeof value !== 'object' || value === null) return undefined
  const { type, slug } = value as Record<string, unknown>

  if (
    typeof type !== 'string' ||
    !Object.hasOwn(OUT_OF_BAND_SEGMENTS, type)
  ) {
    return undefined
  }
  if (
    typeof slug !== 'string' ||
    slug.length > MAX_SLUG_LENGTH ||
    !SLUG_PATTERN.test(slug)
  ) {
    return undefined
  }

  return { type, slug }
}

// ---------------------------------------------------------------------------
// Revalidation
// ---------------------------------------------------------------------------

/**
 * Resolve every accepted entity to its stale pages and revalidate the union.
 *
 * Deduplicating first matters: an ingest run touching 40 artists resolves to
 * the same handful of list pages and route patterns 40 times over.
 */
export function revalidateEntities(
  entries: readonly unknown[]
): RevalidateResult {
  const paths = new Set<string>()
  let revalidated = 0
  let skipped = 0

  for (const entry of entries) {
    const entity = toTouchedEntity(entry)
    if (!entity) {
      skipped++
      continue
    }
    revalidated++
    for (const path of outOfBandPages(
      OUT_OF_BAND_SEGMENTS[entity.type],
      entity.slug
    )) {
      paths.add(path)
    }
  }

  if (skipped > 0) {
    Sentry.captureMessage('internal-revalidate: skipped unusable entities', {
      level: 'warning',
      tags: { service: 'isr-revalidation', source: 'internal-revalidate' },
      extra: { skipped, accepted: revalidated },
    })
  }

  // safeRevalidatePath never throws, so a single bad path cannot stop the rest.
  for (const path of paths) {
    safeRevalidatePath(path, 'internal-revalidate')
  }

  return { revalidated, skipped, paths: paths.size }
}
