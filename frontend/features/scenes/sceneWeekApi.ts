/**
 * Reading scene-weeks from the API, and nothing else.
 *
 * A leaf module by design, importing nothing but the API base and the response
 * type: the share card renders on the edge runtime, and reaching this through
 * the page module would drag the page view and its JSON-LD helpers into that
 * bundle for no reason.
 */
import * as Sentry from '@sentry/nextjs'
import { API_BASE_URL } from '@/lib/api-base'
import type { SceneWeekResponse } from './sceneWeek'

/**
 * How long a fetched week stays fresh.
 *
 * A product decision, not a tuning detail. A week that can still gain shows —
 * the current one, and any future one — has to stay fresh, because the count on
 * a shared card is the whole point of the card. Only a week that has actually
 * ended is immutable.
 *
 * Note what does NOT decide this: the URL shape. `/scenes/{slug}/{iso-week}` is
 * the CANONICAL url for both routes, so it serves the live week as often as an
 * old one. `fetchSceneWeek` picks between these two from the week itself.
 */
export const CURRENT_WEEK_REVALIDATE = 900
export const ARCHIVED_WEEK_REVALIDATE = 86400

/** Which surface a failure came from, so Sentry triage can tell them apart. */
type SceneWeekService = 'scene-week' | 'og-image'

/**
 * Accept a 200 body only if it is actually a week.
 *
 * A 200 is not proof of the right endpoint: a redirect, a CDN error page, or a
 * future API change can all answer 200 with something else. Every field checked
 * here is one a consumer reads WITHOUT a null guard — the dates go straight
 * into date maths, `city` into string measurement, and `slug`/`iso_week` into
 * the canonical and share-image URLs. Checking them turns a crash into the
 * ordinary "no data" path; the rest of the payload is already optional-safe.
 */
function asSceneWeek(body: unknown): SceneWeekResponse | null {
  const week = body as SceneWeekResponse | null
  if (!week || typeof week !== 'object') return null
  for (const field of ['start_date', 'end_date', 'city', 'slug', 'iso_week'] as const) {
    if (typeof week[field] !== 'string') return null
  }
  return week
}

/**
 * One request for one week, at one explicitly chosen freshness window.
 *
 * Private: choosing the window is the whole subtlety here, and `fetchSceneWeek`
 * below is the only thing entitled to do it.
 */
async function fetchWeekPayload(
  slug: string,
  week: string | undefined,
  service: SceneWeekService,
  revalidate: number
): Promise<SceneWeekResponse | null> {
  // Both segments are attacker-controlled: Next decodes route params before the
  // handler sees them, so a slug of `chicago-il?x` or `chicago-il#x` would
  // truncate this path at the query/fragment and silently send the request to a
  // DIFFERENT backend endpoint — one that answers 200 with a shape this code
  // then trips over. `proxy.ts` encodes for the same reason.
  const path = week
    ? `${API_BASE_URL}/scenes/${encodeURIComponent(slug)}/week/${encodeURIComponent(week)}`
    : `${API_BASE_URL}/scenes/${encodeURIComponent(slug)}/week`
  try {
    const res = await fetch(path, { next: { revalidate } })
    // `await` is load-bearing, not noise: `return res.json()` inside a try
    // block adopts the promise AFTER the block exits, so a malformed body would
    // reject past this catch and 500 the route instead of reaching the caller's
    // fallback.
    if (res.ok) return asSceneWeek(await res.json())
    // 404 is the expected answer for an unknown slug, a below-threshold scene,
    // or a week key that does not exist (e.g. 2025-W53) — not an error worth
    // reporting.
    if (res.status >= 500) {
      Sentry.captureMessage(`Scene week: API returned ${res.status}`, {
        level: 'error',
        tags: { service },
        extra: { slug, week, status: res.status },
      })
    }
  } catch (error) {
    Sentry.captureException(error, {
      level: 'error',
      tags: { service },
      extra: { slug, week },
    })
  }
  return null
}

/**
 * Fetch one scene-week, cached for exactly as long as that week is allowed to
 * be frozen.
 *
 * `week` is an ISO week key, or omitted for the scene's CURRENT week. Current
 * is resolved SERVER-side by the backend, in the scene's own timezone — a
 * reader in Berlin and a reader in Chicago must see the same Chicago week, so
 * the client must not compute it. `is_past_week` is the same answer from the
 * same authority, and it is the only thing that decides the window here.
 *
 * Why a key sometimes costs two requests. `next: { revalidate }` has to be
 * supplied BEFORE the response exists, so the only way to learn which window a
 * week deserves is to have already asked for it. The long window goes first
 * deliberately: for a week that has ended — the overwhelming majority of keyed
 * URLs, and the ones a crawler walks — that single request is the only one ever
 * made, so an archived week still costs one backend query a day. Only a week
 * that can still gain shows falls through to the second, short-window ask.
 *
 * Both asks address the same URL and therefore share ONE data-cache entry.
 * Next decides staleness against the window the CALLER passes, not the one the
 * entry was stored with (verified against Next 16.1.4's incremental cache), so
 * the short ask re-reads the backend once that shared entry passes 15 minutes
 * while the long ask goes on hitting it. Net effect: a live week refreshes on
 * the 15-minute cadence whichever URL it is served from, an ended week on the
 * daily one.
 *
 * A stale probe can only err one way. If it still says "not ended" for a week
 * that just ended, the fall-through costs one extra refresh and the next
 * request sees the corrected flag; it can never report a live week as frozen.
 */
export async function fetchSceneWeek(
  slug: string,
  week: string | undefined,
  service: SceneWeekService
): Promise<SceneWeekResponse | null> {
  // No key: the caller is asking for whatever week is live, which by definition
  // can still change. Nothing to probe for. Tested exactly as `fetchWeekPayload`
  // tests it, so the branch that picks the window and the branch that picks the
  // URL can never disagree about what counts as "no key".
  if (!week) return fetchWeekPayload(slug, undefined, service, CURRENT_WEEK_REVALIDATE)

  const archived = await fetchWeekPayload(slug, week, service, ARCHIVED_WEEK_REVALIDATE)
  // `=== true` rather than a truthy test, because this reads an untrusted wire
  // payload — the same reason `asSceneWeek` exists. The type says boolean; a
  // body that says anything else must not be allowed to freeze a live week for
  // a day. It also gives the right answer while a deploy has a newer frontend
  // talking to a backend that does not send the field yet: absent reads as
  // "might still change", which costs a short window and self-heals.
  if (!archived || archived.is_past_week === true) return archived

  // `?? archived` because the week demonstrably exists — the ask above returned
  // it. A blip on this second request must not turn a real page into a 404;
  // slightly older data is the better failure.
  return (await fetchWeekPayload(slug, week, service, CURRENT_WEEK_REVALIDATE)) ?? archived
}
