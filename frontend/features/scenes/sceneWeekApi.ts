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
 * old one — keying the window on "was a week key supplied" would cache the live
 * week for a day. Callers that cannot yet know which kind of week they are
 * fetching must ask for the short window and let the response decide anything
 * longer.
 */
export const CURRENT_WEEK_REVALIDATE = 900
export const ARCHIVED_WEEK_REVALIDATE = 86400

/** Which surface a failure came from, so Sentry triage can tell them apart. */
type SceneWeekService = 'scene-week' | 'og-image'

/**
 * Accept a 200 body only if it is actually a week.
 *
 * A 200 is not proof of the right endpoint: a redirect, a CDN error page, or a
 * future API change can all answer 200 with something else, and every consumer
 * here reads `start_date` straight into date maths that throws on `undefined`.
 * Checking the two load-bearing fields turns that crash into the ordinary
 * "no data" path.
 */
function asSceneWeek(body: unknown): SceneWeekResponse | null {
  const week = body as SceneWeekResponse | null
  if (!week || typeof week !== 'object') return null
  if (typeof week.start_date !== 'string' || typeof week.end_date !== 'string') return null
  return week
}

/**
 * Fetch one scene-week.
 *
 * `week` is an ISO week key, or omitted for the scene's CURRENT week. Current
 * is resolved SERVER-side by the backend, in the scene's own timezone — a
 * reader in Berlin and a reader in Chicago must see the same Chicago week, so
 * the client must not compute it.
 *
 * Kept in its own leaf module, importing nothing but the API base and the
 * response type: the share card renders on the edge runtime, and reaching this
 * through the page module would drag the page view and its JSON-LD helpers into
 * that bundle for no reason.
 */
export async function fetchSceneWeek(
  slug: string,
  week: string | undefined,
  service: SceneWeekService
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
    // Always the SHORT window. The fetch cannot know whether the requested week
    // has ended — only the response can say — so it asks for the freshest data
    // and lets callers extend the life of what they build from it. The data
    // round-trip is the cheap half; the render is what wants long caching.
    const res = await fetch(path, { next: { revalidate: CURRENT_WEEK_REVALIDATE } })
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
