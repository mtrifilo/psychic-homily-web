/**
 * Reading scene-days from the API — the day surface's binding of the shared
 * period fetch, and nothing else.
 */
import { API_BASE_URL } from '@/lib/api-base'
import { fetchScenePeriod } from './scenePeriodApi'
import type { SceneDayResponse } from './sceneDay'

function daySpec(slug: string) {
  return {
    label: 'Scene day',
    slug,
    // Both segments are attacker-controlled — Next decodes route params before
    // the handler sees them, so a slug of `phoenix-az?x` would truncate this
    // path at the query and silently send the request to a DIFFERENT endpoint,
    // one that answers 200 with a shape this code then trips over. `proxy.ts`
    // encodes for the same reason.
    buildUrl: (date: string | undefined) =>
      date
        ? `${API_BASE_URL}/scenes/${encodeURIComponent(slug)}/day/${encodeURIComponent(date)}`
        : `${API_BASE_URL}/scenes/${encodeURIComponent(slug)}/day`,
    // Fields a consumer reads WITHOUT a null guard, split by what a blank one
    // would mean. `date` goes straight into `parseCalendarDate`, which splits
    // the string, and into the day permalink; `slug` and `iso_week` build the
    // canonical and the share-image URL; `city` is printed. None of them has a
    // blank form this surface can serve.
    identityFields: ['date', 'city', 'slug', 'iso_week'] as const,
    // `prev_date` and `next_date` name the adjacent days, and they are EMPTY at
    // the edges of the servable window: there, emptiness IS the answer "no
    // neighbour in this direction", so it must not fail the payload. Absence is
    // a different answer, and a body that omits them is not this payload.
    presenceFields: ['prev_date', 'next_date'] as const,
    // `=== true` rather than truthy: a wire value of anything else must not
    // freeze tonight's page in the CDN for a day. The backend guarantees this
    // is never true while `is_tonight` is (see dayHasEnded), which is what lets
    // the cache decision rest on this field alone.
    isFrozen: (day: SceneDayResponse) => day.is_past_day === true,
    service: 'scene-day' as const,
  }
}

/**
 * Fetch one scene-day. `date` is an ISO calendar date, or omitted for the
 * scene's current NIGHT.
 *
 * Tonight is resolved SERVER-side, in the scene's own timezone and against its
 * 6am night boundary — a reader in Berlin and a reader in Phoenix must see the
 * same Phoenix night, so the client must not compute it.
 */
export function fetchSceneDay(
  slug: string,
  date?: string
): Promise<SceneDayResponse | null> {
  return fetchScenePeriod<SceneDayResponse>(daySpec(slug), date)
}
