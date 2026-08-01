/**
 * Reading scene-days from the API — the day surface's binding of the shared
 * period fetch, and nothing else.
 */
import { API_BASE_URL } from '@/lib/api-base'
import { fetchScenePeriod } from './scenePeriodApi'
import type { SceneDayResponse } from './sceneDay'

const daySpec = {
  label: 'Scene day',
  // Both segments are attacker-controlled — Next decodes route params before
  // the handler sees them, so a slug of `phoenix-az?x` would truncate this path
  // at the query and silently send the request to a DIFFERENT endpoint, one
  // that answers 200 with a shape this code then trips over. `proxy.ts` encodes
  // for the same reason.
  buildUrl: (slug: string) => (date: string | undefined) =>
    date
      ? `${API_BASE_URL}/scenes/${encodeURIComponent(slug)}/day/${encodeURIComponent(date)}`
      : `${API_BASE_URL}/scenes/${encodeURIComponent(slug)}/day`,
  // `date` goes straight into date maths and `slug` into the canonical URL, so
  // a body missing either is not a day payload no matter what status it came
  // with.
  requiredFields: ['date', 'city', 'slug', 'iso_week'] as const,
  // `=== true` rather than truthy: a wire value of anything else must not
  // freeze tonight's page in the CDN for a day.
  isFrozen: (day: SceneDayResponse) => day.is_past_day === true,
  service: 'scene-day' as const,
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
  return fetchScenePeriod<SceneDayResponse>(
    { ...daySpec, buildUrl: daySpec.buildUrl(slug) },
    date
  )
}
