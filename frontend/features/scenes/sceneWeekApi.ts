/**
 * Reading scene-weeks from the API — the week surface's binding of the shared
 * period fetch, and nothing else.
 *
 * A leaf module by design: the share card renders on the edge runtime, and
 * reaching this through the page module would drag the page view and its
 * JSON-LD helpers into that bundle for no reason.
 */
import { API_BASE_URL } from '@/lib/api-base'
import { fetchScenePeriod, type ScenePeriodService } from './scenePeriodApi'
import type { SceneWeekResponse } from './sceneWeek'

/** Which surface a failure came from, so Sentry triage can tell them apart. */
type SceneWeekService = Extract<ScenePeriodService, 'scene-week' | 'og-image'>

function weekSpec(slug: string, service: SceneWeekService) {
  return {
    label: 'Scene week',
    slug,
    // Both segments are attacker-controlled: Next decodes route params before
    // the handler sees them, so a slug of `chicago-il?x` or `chicago-il#x`
    // would truncate this path at the query/fragment and silently send the
    // request to a DIFFERENT backend endpoint — one that answers 200 with a
    // shape this code then trips over. `proxy.ts` encodes for the same reason.
    buildUrl: (week: string | undefined) =>
      week
        ? `${API_BASE_URL}/scenes/${encodeURIComponent(slug)}/week/${encodeURIComponent(week)}`
        : `${API_BASE_URL}/scenes/${encodeURIComponent(slug)}/week`,
    // Every field here is one a consumer reads WITHOUT a null guard: the dates
    // go straight into date maths, `city` into string measurement, and
    // `slug`/`iso_week` into the canonical and share-image URLs.
    requiredFields: ['start_date', 'end_date', 'city', 'slug', 'iso_week'] as const,
    // `=== true` rather than a truthy test, because this reads an untrusted
    // wire payload. The type says boolean; a body that says anything else must
    // not be allowed to freeze a live week for a day.
    isFrozen: (week: SceneWeekResponse) => week.is_past_week === true,
    service,
  }
}

/**
 * Fetch one scene-week, cached for exactly as long as that week is allowed to
 * be frozen.
 *
 * `week` is an ISO week key, or omitted for the scene's CURRENT week. Current
 * is resolved SERVER-side by the backend, in the scene's own timezone — a
 * reader in Berlin and a reader in Chicago must see the same Chicago week, so
 * the client must not compute it.
 */
export function fetchSceneWeek(
  slug: string,
  week: string | undefined,
  service: SceneWeekService
): Promise<SceneWeekResponse | null> {
  return fetchScenePeriod<SceneWeekResponse>(weekSpec(slug, service), week)
}
