/**
 * Reading scene-days from the API, and nothing else.
 *
 * A leaf module by design, mirroring `sceneWeekApi`: importing nothing but the
 * API base and the response type keeps it usable from any runtime without
 * dragging the page view and its JSON-LD helpers along.
 */
import * as Sentry from '@sentry/nextjs'
import { API_BASE_URL } from '@/lib/api-base'
import type { SceneDayResponse } from './sceneDay'

/**
 * How long a fetched day stays fresh.
 *
 * A day that can still gain shows — tonight, and any future date — has to stay
 * fresh; only a day that is over is immutable. Same product decision, and the
 * same two windows, as the week: a reader arriving from a search result for
 * "shows tonight" is the least forgiving audience for a stale count.
 *
 * Note what does NOT decide this: the URL shape. `/scenes/{slug}/{date}` is the
 * canonical URL for both day routes, so it serves tonight as often as an old
 * night. `fetchSceneDay` picks between these two from the payload itself.
 */
export const CURRENT_DAY_REVALIDATE = 900
export const ARCHIVED_DAY_REVALIDATE = 86400

/**
 * Accept a 200 body only if it is actually a day.
 *
 * A 200 is not proof of the right endpoint: a redirect, a CDN error page, or a
 * future API change can all answer 200 with something else. Every field checked
 * here is one a consumer reads WITHOUT a null guard — `date` goes straight into
 * date maths and `slug` into the canonical URL — so checking them turns a crash
 * into the ordinary "no data" path.
 */
function asSceneDay(body: unknown): SceneDayResponse | null {
  const day = body as SceneDayResponse | null
  if (!day || typeof day !== 'object') return null
  for (const field of ['date', 'city', 'slug', 'iso_week'] as const) {
    if (typeof day[field] !== 'string') return null
  }
  return day
}

/**
 * One request for one day, at one explicitly chosen freshness window.
 *
 * Private: choosing the window is the whole subtlety here, and `fetchSceneDay`
 * below is the only thing entitled to do it.
 */
async function fetchDayPayload(
  slug: string,
  date: string | undefined,
  revalidate: number
): Promise<SceneDayResponse | null> {
  // Both segments are attacker-controlled — Next decodes route params before
  // the handler sees them, so a slug of `phoenix-az?x` would truncate this path
  // at the query and silently send the request to a DIFFERENT endpoint, one
  // that answers 200 with a shape this code then trips over. `proxy.ts` encodes
  // for the same reason.
  const path = date
    ? `${API_BASE_URL}/scenes/${encodeURIComponent(slug)}/day/${encodeURIComponent(date)}`
    : `${API_BASE_URL}/scenes/${encodeURIComponent(slug)}/day`
  try {
    const res = await fetch(path, { next: { revalidate } })
    // `await` is load-bearing, not noise: `return res.json()` inside a try block
    // adopts the promise AFTER the block exits, so a malformed body would reject
    // past this catch and 500 the route instead of reaching the caller's
    // fallback.
    if (res.ok) return asSceneDay(await res.json())
    // 404 is the expected answer for an unknown slug, a below-threshold scene,
    // or an impossible date — not an error worth reporting.
    if (res.status >= 500) {
      Sentry.captureMessage(`Scene day: API returned ${res.status}`, {
        level: 'error',
        tags: { service: 'scene-day' },
        extra: { slug, date, status: res.status },
      })
    }
  } catch (error) {
    Sentry.captureException(error, {
      level: 'error',
      tags: { service: 'scene-day' },
      extra: { slug, date },
    })
  }
  return null
}

/**
 * Fetch one scene-day, cached for exactly as long as that day is allowed to be
 * frozen.
 *
 * `date` is an ISO calendar date, or omitted for the scene's current NIGHT.
 * Tonight is resolved SERVER-side, in the scene's own timezone and against its
 * 6am night boundary — a reader in Berlin and a reader in Phoenix must see the
 * same Phoenix night, so the client must not compute it. `is_past_day` is the
 * same answer from the same authority, and it is the only thing that decides
 * the window here.
 *
 * Why a dated URL sometimes costs two requests: `next: { revalidate }` has to be
 * supplied BEFORE the response exists, so the only way to learn which window a
 * day deserves is to have already asked for it. The long window goes first
 * deliberately — for a day that is over, which is the overwhelming majority of
 * dated URLs and the ones a crawler walks, that single request is the only one
 * ever made. Both asks address the same URL and share ONE data-cache entry;
 * Next scores staleness against the window the CALLER passes, so the short ask
 * re-reads the backend once that entry passes 15 minutes while the long ask
 * goes on hitting it. Identical shape to `fetchSceneWeek`, whose comment
 * carries the cache-behaviour verification.
 *
 * A stale probe can only err one way: if it still says "not over" for a day
 * that just ended, the fall-through costs one extra refresh; it can never
 * report a live day as frozen.
 */
export async function fetchSceneDay(
  slug: string,
  date?: string
): Promise<SceneDayResponse | null> {
  // No date: the caller is asking for tonight, which by definition can still
  // change. Nothing to probe for.
  if (!date) return fetchDayPayload(slug, undefined, CURRENT_DAY_REVALIDATE)

  const archived = await fetchDayPayload(slug, date, ARCHIVED_DAY_REVALIDATE)
  // `=== true` rather than a truthy test, because this reads an untrusted wire
  // payload — the same reason `asSceneDay` exists. It also gives the right
  // answer while a deploy has a newer frontend talking to a backend that does
  // not send the field yet: absent reads as "might still change", which costs a
  // short window and self-heals.
  if (!archived || archived.is_past_day === true) return archived

  // `?? archived` because the day demonstrably exists — the ask above returned
  // it. A blip on this second request must not turn a real page into a 404;
  // slightly older data is the better failure.
  return (await fetchDayPayload(slug, date, CURRENT_DAY_REVALIDATE)) ?? archived
}
