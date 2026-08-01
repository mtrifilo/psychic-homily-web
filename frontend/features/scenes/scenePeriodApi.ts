/**
 * Reading one PERIOD of a scene's calendar from the API — a week, a night —
 * and nothing else.
 *
 * A leaf module by design, importing nothing but the API base: the weekly share
 * card renders on the edge runtime, and reaching this through a page module
 * would drag the page view and its JSON-LD helpers into that bundle for no
 * reason.
 *
 * The two-phase freshness probe below is the subtlest thing in this feature,
 * and it rests on an observation about a specific Next version. It lives here,
 * once, so that observation has ONE place to be re-verified on an upgrade and
 * one place to be corrected — a second copy would go silently wrong the first
 * time only one of them was fixed.
 */
import * as Sentry from '@sentry/nextjs'

/**
 * How long a fetched period stays fresh.
 *
 * A product decision, not a tuning detail. A period that can still gain shows —
 * the current one, and any future one — has to stay fresh, because the count is
 * the whole point of the page. Only a period that has actually ended is
 * immutable.
 *
 * Note what does NOT decide this: the URL shape. The dated/keyed permalink is
 * the CANONICAL url for the rolling route too, so it serves the live period as
 * often as an old one. `fetchScenePeriod` picks between these two from the
 * payload itself.
 */
export const CURRENT_PERIOD_REVALIDATE = 900
export const ARCHIVED_PERIOD_REVALIDATE = 86400

/** Which surface a failure came from, so Sentry triage can tell them apart. */
export type ScenePeriodService = 'scene-week' | 'scene-day' | 'og-image'

interface ScenePeriodSpec<T> {
  /** Human label for the Sentry message, e.g. `Scene week`. */
  label: string
  /**
   * WHICH scene this is about — the first thing triage needs, and the one thing
   * the period key alone cannot say. Carried explicitly because the slug is
   * otherwise closed over inside `buildUrl` and would vanish from every report.
   */
  slug: string
  /** The API URL for this period key, or for the CURRENT period when omitted. */
  buildUrl: (key: string | undefined) => string
  /**
   * Fields a consumer reads WITHOUT a null guard. Each must be a string on the
   * wire or the body is not this payload at all.
   */
  requiredFields: readonly string[]
  /** Reads the payload's "this period has ended" flag. */
  isFrozen: (payload: T) => boolean
  service: ScenePeriodService
}

/**
 * Accept a 200 body only if it is actually the payload we asked for.
 *
 * A 200 is not proof of the right endpoint: a redirect, a CDN error page, or a
 * future API change can all answer 200 with something else. Checking the fields
 * a consumer dereferences blindly turns a crash into the ordinary "no data"
 * path; the rest of the payload is already optional-safe.
 */
function asPayload<T>(body: unknown, requiredFields: readonly string[]): T | null {
  if (!body || typeof body !== 'object') return null
  const record = body as Record<string, unknown>
  for (const field of requiredFields) {
    if (typeof record[field] !== 'string') return null
  }
  return body as T
}

/**
 * One request for one period, at one explicitly chosen freshness window.
 *
 * Private: choosing the window is the whole subtlety here, and
 * `fetchScenePeriod` below is the only thing entitled to do it.
 */
async function fetchPayload<T>(
  spec: ScenePeriodSpec<T>,
  key: string | undefined,
  revalidate: number
): Promise<T | null> {
  const url = spec.buildUrl(key)
  try {
    const res = await fetch(url, { next: { revalidate } })
    // `await` is load-bearing, not noise: `return res.json()` inside a try block
    // adopts the promise AFTER the block exits, so a malformed body would reject
    // past this catch and 500 the route instead of reaching the caller's
    // fallback.
    if (res.ok) return asPayload<T>(await res.json(), spec.requiredFields)
    // 404 is the expected answer for an unknown slug, a below-threshold scene,
    // or a key that does not exist (2025-W53, 2026-02-30) — not an error worth
    // reporting.
    if (res.status >= 500) {
      Sentry.captureMessage(`${spec.label}: API returned ${res.status}`, {
        level: 'error',
        tags: { service: spec.service },
        extra: { slug: spec.slug, key, status: res.status },
      })
    }
  } catch (error) {
    Sentry.captureException(error, {
      level: 'error',
      tags: { service: spec.service },
      extra: { slug: spec.slug, key },
    })
  }
  return null
}

/**
 * Fetch one period of a scene's calendar, cached for exactly as long as that
 * period is allowed to be frozen.
 *
 * `key` is the period's permalink key, or omitted for the scene's CURRENT
 * period. Current is resolved SERVER-side by the backend, in the scene's own
 * timezone — a reader in Berlin and a reader in Chicago must see the same
 * Chicago period, so the client must not compute it. The frozen flag is the
 * same answer from the same authority, and it is the only thing that decides
 * the window here.
 *
 * Why a key sometimes costs two requests. `next: { revalidate }` has to be
 * supplied BEFORE the response exists, so the only way to learn which window a
 * period deserves is to have already asked for it. The long window goes first
 * deliberately: for a period that has ended — the overwhelming majority of
 * keyed URLs, and the ones a crawler walks — that single request is the only
 * one ever made, so an archived period still costs one backend query a day.
 * Only a period that can still gain shows falls through to the second, short
 * ask.
 *
 * Both asks address the same URL and therefore share ONE data-cache entry. Next
 * decides staleness against the window the CALLER passes, not the one the entry
 * was stored with (verified against Next 16.1.4's incremental cache), so the
 * short ask re-reads the backend once that shared entry passes 15 minutes while
 * the long ask goes on hitting it. Net effect: a live period refreshes on the
 * 15-minute cadence whichever URL it is served from, an ended one on the daily
 * cadence. RE-VERIFY THIS on a Next upgrade — it is the one claim here that a
 * framework release can quietly invalidate.
 *
 * A stale probe can only err one way. If it still says "not ended" for a period
 * that just ended, the fall-through costs one extra refresh and the next
 * request sees the corrected flag; it can never report a live period as frozen.
 */
export async function fetchScenePeriod<T>(
  spec: ScenePeriodSpec<T>,
  key: string | undefined
): Promise<T | null> {
  // No key: the caller is asking for whatever period is live, which by
  // definition can still change. Nothing to probe for.
  if (!key) return fetchPayload(spec, undefined, CURRENT_PERIOD_REVALIDATE)

  const archived = await fetchPayload(spec, key, ARCHIVED_PERIOD_REVALIDATE)
  // The flag is read strictly, because this is an untrusted wire payload — the
  // same reason `asPayload` exists. It also gives the right answer while a
  // deploy has a newer frontend talking to a backend that does not send the
  // field yet: absent reads as "might still change", which costs a short window
  // and self-heals.
  if (!archived || spec.isFrozen(archived)) return archived

  // `?? archived` because the period demonstrably exists — the ask above
  // returned it. A blip on this second request must not turn a real page into a
  // 404; slightly older data is the better failure.
  return (await fetchPayload(spec, key, CURRENT_PERIOD_REVALIDATE)) ?? archived
}
