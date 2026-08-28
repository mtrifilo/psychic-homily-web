/**
 * Sentry visibility for client-side HTTP 429s.
 *
 * WHY THIS EXISTS
 *
 * Rate-limited reads were completely invisible in Sentry. Nothing captured a
 * 429, so a page block dying on one looked exactly like a page block that
 * loaded fine, and the only evidence was a user reporting a broken section.
 * Now that 429s are retried silently (see `./query-retry-policy`), the absence of
 * telemetry gets WORSE rather than better: a successful retry leaves no
 * user-visible trace at all. Without this module the retry would hide the
 * problem it works around, and the request-amplification and anonymous-ceiling
 * follow-ups would have no data to aim at.
 *
 * TWO SIGNALS, DELIBERATELY DIFFERENT
 *
 *   - `recordRateLimitHit` fires for every 429 seen at the fetch boundary. It
 *     is the volume signal: how often the budget is hit, and on which endpoint
 *     families. Reported at `warning`, because a retried 429 is not a
 *     user-visible defect.
 *   - `reportRateLimitExhausted` fires only once the retry budget has run out
 *     and the user actually saw a failed block. Reported at `error`, because
 *     that one IS a defect.
 *
 * SAMPLING
 *
 * One entity page fans out into many parallel reads, so a single exhausted
 * budget produces a burst of 429s inside a few hundred milliseconds. An event
 * per 429 would turn one incident into a dozen near-identical events and burn
 * quota without adding information. Instead:
 *
 *   - EVERY 429 adds a breadcrumb. Breadcrumbs are not events, cost no quota,
 *     and ride along on whatever error eventually reports, so the full
 *     sequence survives for debugging.
 *   - The FIRST 429 in each cooldown window is promoted to an event, carrying a
 *     `suppressedSinceLastReport` count so the true volume survives the
 *     deduplication instead of being silently discarded.
 *
 * A breadcrumb alone would not be enough: the point of the retry is that most
 * 429s no longer produce any error, so a breadcrumb-only design would ship
 * nothing at all in exactly the common case we need to measure. Hence the
 * sampled event.
 *
 * The cooldown state is module-level, so it is per-tab in the browser and
 * per-instance on the server. That is intentional for a sampler; the
 * suppressed counters keep it honest either way.
 *
 * SCRUBBING
 *
 * Endpoints are reduced to a route shape before they leave the process and
 * QUERY STRINGS ARE DROPPED WHOLESALE. That is the load-bearing part: the query
 * string is where feed tokens, magic-link tokens and user search terms live.
 * Path segments that are numeric, UUID-shaped, or long and unhyphenated (that
 * is, token-shaped rather than slug-shaped) are replaced with placeholders.
 * Human-readable slugs are kept, because knowing WHICH endpoint family is
 * amplifying is the entire signal and those slugs are public identifiers on
 * every page of the site. No request or response bodies, no headers, and no
 * full URLs are attached, so the server `beforeSend` header scrub in
 * `sentry.server.config.ts` has nothing of ours left to filter.
 *
 * SCRUBBING THE SDK'S OWN BREADCRUMBS TOO. Scrubbing only what this module
 * builds would have been theatre. Sentry's default `breadcrumbsIntegration`
 * records the FULL fetch URL, query string included, for every request in the
 * tab, and those breadcrumbs ride along on every event. So the careful route
 * shaping here would have sat next to a raw `?token=...` in the same envelope.
 * That predates this module, but this module is what makes it matter: it mints
 * a regularly-firing event class whose trigger condition is "the user is
 * generating lots of requests", which is exactly when the breadcrumb trail is
 * richest. `toTelemetryPath` is therefore also wired into a global
 * `beforeBreadcrumb` in `instrumentation-client.ts`.
 *
 * Still NOT covered, and worth knowing: `httpContextIntegration` attaches the
 * current PAGE url, which on a filtered or searched route carries the user's
 * terms. That one needs a client `beforeSend` and a decision about how much
 * page context is worth keeping, so it is left alone rather than half-done.
 */

import * as Sentry from '@sentry/nextjs'
import { LIMITER_WINDOW_MS } from './query-retry-policy'

/**
 * Minimum gap between promoted 429 events for the same route shape. One
 * limiter window: long enough to collapse a whole page's burst into a single
 * event per endpoint, short enough that a sustained problem keeps reporting.
 *
 * Imported rather than restated so this and the retry budget cannot drift: it
 * is the same backend fact, and it was previously written as an independent
 * `60_000` here.
 */
export const RATE_LIMIT_REPORT_COOLDOWN_MS = LIMITER_WINDOW_MS

/** Longest telemetry path we will emit, and the segment cap that feeds it. */
const MAX_TELEMETRY_PATH_LENGTH = 120
const MAX_TELEMETRY_PATH_SEGMENTS = 8

const UUID_PATTERN =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i

/**
 * Length at or above which a segment must LOOK LIKE A SLUG to survive. Short
 * segments are left alone; anything longer is either one of our slugs or an
 * identifier we should not be shipping.
 */
const OPAQUE_SEGMENT_MIN_LENGTH = 24

/**
 * What a slug looks like: lowercase alphanumerics in hyphen-separated runs,
 * which is what `GenerateSlug` produces.
 *
 * Long segments are matched against this ALLOWLIST rather than against a
 * denylist of token shapes, and that direction is the point. A denylist has to
 * anticipate the next credential format, and the obvious candidates already
 * defeat the one this replaced: it required a segment to have no hyphen and to
 * be `[A-Za-z0-9_]` only, so a base64url token (hyphens) or a JWT (dots) rode
 * through verbatim at any length. Failing closed costs at most a placeholder
 * where a mixed-case or accented slug used to be readable.
 */
const SLUG_SHAPE = /^[a-z0-9]+(?:-[a-z0-9]+)*$/

/**
 * Credential prefixes the backend mints, scrubbed on sight regardless of
 * length. Today's tokens (`phcal_` plus 64 hex characters, `phk_` likewise)
 * are long enough that the length heuristic below would catch them anyway, so
 * this is belt and braces: it means a future short-token scheme cannot quietly
 * start shipping credentials into Sentry because a threshold tuned for slugs
 * happened to be the only thing stopping it.
 */
const TOKEN_SEGMENT_PREFIXES = ['phcal_', 'phk_']

function scrubSegment(segment: string): string {
  if (segment === '') return segment
  if (/^\d+$/.test(segment)) return ':id'
  if (UUID_PATTERN.test(segment)) return ':uuid'
  if (TOKEN_SEGMENT_PREFIXES.some(prefix => segment.startsWith(prefix))) {
    return ':token'
  }
  if (
    segment.length >= OPAQUE_SEGMENT_MIN_LENGTH &&
    !SLUG_SHAPE.test(segment)
  ) {
    return ':opaque'
  }
  return segment
}

/**
 * Reduce an API endpoint to a route shape safe to ship to Sentry.
 *
 * Accepts either the already-relative path `lib/api.ts` computes or a full
 * URL; in both cases only the path survives.
 */
export function toTelemetryPath(endpoint: string | undefined): string {
  if (!endpoint) return 'unknown'

  // Drop query and fragment first: everything secret rides there.
  const pathOnly = endpoint.split(/[?#]/, 1)[0]
  // Strip an absolute prefix (scheme plus host) without URL parsing, which
  // would throw on the relative paths this is usually given. The scheme is
  // optional so a protocol-relative `//host/path` loses its host too.
  const withoutOrigin = pathOnly.replace(
    /^(?:[a-z][a-z0-9+.-]*:)?\/\/[^/]*/i,
    ''
  )
  if (withoutOrigin === '') return 'unknown'

  const segments = withoutOrigin.split('/')
  const truncated = segments.length > MAX_TELEMETRY_PATH_SEGMENTS
  const kept = segments
    .slice(0, MAX_TELEMETRY_PATH_SEGMENTS)
    .map(scrubSegment)
    .join('/')

  const shaped = truncated ? `${kept}/...` : kept
  return shaped.length > MAX_TELEMETRY_PATH_LENGTH
    ? `${shaped.slice(0, MAX_TELEMETRY_PATH_LENGTH)}...`
    : shaped
}

const runtimeTag = (): 'browser' | 'server' =>
  typeof window === 'undefined' ? 'server' : 'browser'

/**
 * Run a reporting call so that a failure inside it can never propagate.
 *
 * Both entry points are invoked from error paths, where a thrown telemetry
 * error would be mistaken for the application error it was reporting on.
 * Swallowing is the right call here: there is nowhere useful to report a
 * failure of the reporting system, and a lost 429 sample is not worth breaking
 * a request over.
 */
function reportSafely(report: () => void): void {
  try {
    report()
  } catch {
    // Deliberately silent. See above.
  }
}

/** Per-kind cooldown state for the sampler described in the module comment. */
type Sampler = { lastReportAt: number; suppressed: number }

/**
 * Samplers are keyed by ROUTE SHAPE, not shared across the whole process.
 *
 * A single global cooldown looked like it collapsed one page's burst, and in
 * practice it collapsed the dimension the signal exists to measure: whichever
 * endpoint 429'd first each minute claimed the only slot, and every other
 * endpoint in that window was reduced to an anonymous increment. With 60s
 * pollers in the app (notifications, radio) the winner was also biased toward
 * whichever poller happened to fire, so the reported `endpoint` described the
 * race rather than the amplification. Keying per path means each endpoint
 * family reports once per window on its own merits.
 */
const MAX_SAMPLED_PATHS = 64

/**
 * Bounded so a pathological path space (an unscrubbed identifier shape we did
 * not anticipate) cannot grow this map without limit in a long-lived tab or
 * server process. Past the cap everything shares one overflow bucket, which
 * degrades to the old global behaviour rather than to a leak.
 */
const OVERFLOW_PATH = ':overflow'

function samplerFor(samplers: Map<string, Sampler>, path: string): Sampler {
  const key = samplers.has(path)
    ? path
    : samplers.size >= MAX_SAMPLED_PATHS
      ? OVERFLOW_PATH
      : path
  let sampler = samplers.get(key)
  if (!sampler) {
    sampler = { lastReportAt: 0, suppressed: 0 }
    samplers.set(key, sampler)
  }
  return sampler
}

const hitSamplers = new Map<string, Sampler>()
const exhaustedSamplers = new Map<string, Sampler>()

/**
 * Decide whether this occurrence is promoted to an event. Returns the
 * suppressed count to attach when it is, or `null` when it is being folded
 * into the current window.
 */
function claimReportSlot(sampler: Sampler, now: number): number | null {
  // A clock that moved BACKWARDS (NTP correction, sleep/resume, VM migration,
  // or a test installing fake timers) makes the elapsed subtraction negative,
  // which reads as "still inside the cooldown" and would suppress every report
  // until wall time caught back up. That is the one way this sampler can fail
  // without a bound, so a rewind re-anchors and reports instead.
  const rewound = now < sampler.lastReportAt
  // The `!== 0` arm states "never reported yet" explicitly rather than relying
  // on wall-clock arithmetic to imply it. A real `Date.now()` dwarfs the
  // cooldown so the subtraction would also let the first occurrence through,
  // but that is an accident of the epoch, not something to depend on.
  if (
    sampler.lastReportAt !== 0 &&
    !rewound &&
    now - sampler.lastReportAt < RATE_LIMIT_REPORT_COOLDOWN_MS
  ) {
    sampler.suppressed += 1
    return null
  }
  const suppressed = sampler.suppressed
  sampler.lastReportAt = now
  sampler.suppressed = 0
  return suppressed
}

export interface RateLimitHit {
  /** API endpoint path or URL. Scrubbed to a route shape before it is sent. */
  endpoint?: string
  /** Parsed `Retry-After` in seconds, when the response carried one. */
  retryAfter?: number
  /** Backend request id, for correlation with server-side logs. */
  requestId?: string
}

/**
 * Record a 429 observed at the fetch boundary. Always breadcrumbs; promotes to
 * a sampled `warning` event per the cooldown above.
 */
export function recordRateLimitHit(hit: RateLimitHit): void {
  // Fail-safe: this runs on the error path in `lib/api.ts`, immediately before
  // the ApiError is thrown. If Sentry throws (transport, a serialization
  // failure, a misconfigured client) that error would replace the ApiError the
  // caller is waiting for, and since it carries no `.status`, the retry policy
  // would read it as a network error and retry it on the 5xx curve. Telemetry
  // must never be able to change the error the application sees.
  reportSafely(() => recordRateLimitHitUnguarded(hit))
}

function recordRateLimitHitUnguarded(hit: RateLimitHit): void {
  const path = toTelemetryPath(hit.endpoint)
  const runtime = runtimeTag()

  Sentry.addBreadcrumb({
    category: 'rate-limit',
    type: 'http',
    level: 'warning',
    message: `429 ${path}`,
    data: {
      endpoint: path,
      retryAfterSeconds: hit.retryAfter,
      requestId: hit.requestId,
      runtime,
    },
  })

  const suppressed = claimReportSlot(
    samplerFor(hitSamplers, path),
    Date.now()
  )
  if (suppressed === null) return

  Sentry.captureMessage('Client rate limited (HTTP 429)', {
    level: 'warning',
    tags: {
      error_type: 'rate_limited',
      status: 429,
      runtime,
      // Whether the backend told us when to come back. In production it never
      // does (CORS does not expose the header), so this doubles as the alarm
      // that would fire if that were ever fixed, and as the filter for the
      // development and SSR cases where the header IS readable.
      has_retry_after: hit.retryAfter != null,
    },
    extra: {
      endpoint: path,
      retryAfterSeconds: hit.retryAfter,
      requestId: hit.requestId,
      suppressedSinceLastReport: suppressed,
    },
  })
}

export interface RateLimitExhaustion extends RateLimitHit {
  /** Number of attempts made before the query gave up. */
  attempts: number
  /**
   * Leading segments of the query key, so the failing block is identifiable
   * without shipping the whole key (which can carry search terms).
   */
  queryFamily?: string
}

/**
 * Record a 429 that outlived the retry budget, the case the user actually sees
 * as a broken block. Sampled on its own cooldown so a burst collapses, but
 * reported at `error` because this one is a real user-visible failure.
 *
 * The caller for this signal is the query cache, which knows the query key but
 * not the URL, so `endpoint` is normally absent and omitted rather than sent as
 * a placeholder. The specific paths are still recoverable: every 429 in the run
 * up to this failure left a breadcrumb carrying its scrubbed path, and
 * breadcrumbs ride along on this event.
 */
export function reportRateLimitExhausted(
  exhaustion: RateLimitExhaustion
): void {
  reportSafely(() => reportRateLimitExhaustedUnguarded(exhaustion))
}

function reportRateLimitExhaustedUnguarded(
  exhaustion: RateLimitExhaustion
): void {
  const path = exhaustion.endpoint
    ? toTelemetryPath(exhaustion.endpoint)
    : undefined
  // Keyed on the query family when there is no path, so two different families
  // dying in the same minute both report instead of one silently inheriting
  // the other's identity.
  const suppressed = claimReportSlot(
    samplerFor(exhaustedSamplers, path ?? exhaustion.queryFamily ?? 'unknown'),
    Date.now()
  )
  if (suppressed === null) return

  Sentry.captureMessage('Rate limit retries exhausted (HTTP 429)', {
    level: 'error',
    tags: {
      error_type: 'rate_limit_exhausted',
      status: 429,
      runtime: runtimeTag(),
    },
    extra: {
      endpoint: path,
      queryFamily: exhaustion.queryFamily,
      attempts: exhaustion.attempts,
      retryAfterSeconds: exhaustion.retryAfter,
      requestId: exhaustion.requestId,
      suppressedSinceLastReport: suppressed,
    },
  })
}
