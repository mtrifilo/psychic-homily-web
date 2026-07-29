import * as Sentry from '@sentry/nextjs'
import { API_BASE_URL } from '@/lib/api-base'
import type { SceneWeekResponse } from './sceneWeek'

/**
 * How long a fetched week stays fresh.
 *
 * A product decision, not a tuning detail. The rolling week is the URL that
 * actually gets posted, so a page — or a share card — showing a stale count is
 * worse than a slightly slower render. An archived week is immutable once past
 * and can cache hard.
 *
 * Defined once because the page and its share card MUST agree: two copies would
 * let the card advertise a different week than the page it previews, and
 * nothing in CI would catch the drift.
 */
export const CURRENT_WEEK_REVALIDATE = 900
export const ARCHIVED_WEEK_REVALIDATE = 86400

/** Which surface a failure came from, so Sentry triage can tell them apart. */
type SceneWeekService = 'scene-week' | 'og-image'

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
  const path = week
    ? `${API_BASE_URL}/scenes/${slug}/week/${week}`
    : `${API_BASE_URL}/scenes/${slug}/week`
  try {
    const res = await fetch(path, {
      next: { revalidate: week ? ARCHIVED_WEEK_REVALIDATE : CURRENT_WEEK_REVALIDATE },
    })
    if (res.ok) return res.json()
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
