/**
 * Reading scene-weeks from the API — the week surface's binding of the shared
 * period fetch, and nothing else.
 *
 * A leaf module by design: the share card renders on the edge runtime, and
 * reaching this through the page module would drag the page view and its
 * JSON-LD helpers into that bundle for no reason.
 */
import { API_BASE_URL } from '@/lib/api-base'
import { looksLikeSlug } from '@/lib/entity-slug'
import { anyName, fetchScenePeriod, type ScenePeriodService } from './scenePeriodApi'
import { isCalendarDate, looksLikeISOWeek, type SceneWeekResponse } from './sceneWeek'

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
    // Every field here is one a consumer reads WITHOUT a null guard, each with
    // the shape it has to have: the range dates go straight into date maths,
    // `city` into string measurement, and `slug`/`iso_week` RAW into the
    // canonical and the share-image URL, so each of those two has to be one
    // path segment already.
    //
    // `isCalendarDate`, deliberately, and NOT `sceneDay`'s
    // `looksLikeCalendarDate`: that one also bounds the year to cap the URL
    // cache-key space, and a week's range dates address no URL and legitimately
    // fall outside those bounds. The first servable week starts 2014-12-29 and
    // the last one ends in the January after it. `iso_week` keeps the bounded
    // predicate because it DOES address a URL, one the week route rejects
    // outside exactly those bounds.
    identityFields: [
      ['start_date', isCalendarDate],
      ['end_date', isCalendarDate],
      ['city', anyName],
      ['slug', looksLikeSlug],
      ['iso_week', looksLikeISOWeek],
    ] as const,
    // Empty. `prev_week`/`next_week` are outside this contract: refusing the
    // whole payload over a neighbour would trade a missing chip for a dead
    // page, so the week view decides per chip whether it has a week key it can
    // link to, exactly as the day view does for its neighbouring dates.
    presenceFields: [] as const,
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
