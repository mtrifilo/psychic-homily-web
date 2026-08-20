/**
 * Reading the scene root's calendar SLICE from the API — tonight and the next
 * full day — and nothing else.
 *
 * A leaf module, like `sceneDayApi` and `sceneWeekApi` beside it: it imports the
 * day fetch and the pure assembly, and no view. The sequencing below is the
 * non-obvious part of this feature, so it lives in ONE place that any consumer
 * (the route, a future OG card, a preview panel) can call, rather than being
 * re-derived at each call site along with the trap it has to avoid.
 */
import { fetchSceneDay } from './sceneDayApi'
import { buildSceneSlice, type SceneSliceData } from './sceneSlice'

/**
 * Fetch the root's slice: tonight, then the calendar day after it.
 *
 * SERIAL by necessity. `next_date` is the backend's own answer for the day
 * following tonight, and which date tonight IS depends on the scene's 6am night
 * boundary resolved in its own timezone — so the second request cannot be
 * issued until the first has answered. Deriving tomorrow from a clock here
 * instead would re-introduce exactly the mirrored-boundary drift that reading
 * this endpoint removes.
 *
 * The empty-`next_date` guard is load-bearing, not defensive noise:
 * `fetchScenePeriod` resolves the CURRENT period when its key is falsy, and the
 * backend sends an empty `next_date` at the far edge of the servable window
 * (`servableDateKey`, scene_day.go). A bare `fetchSceneDay(slug,
 * tonight.next_date)` there would quietly fetch TONIGHT a second time, and the
 * slice would print the same night twice under two different headings.
 * `buildSceneSlice` refuses that pair on the date as well, so the mistake is
 * caught at both layers rather than relied on being avoided at one.
 *
 * Returns null only when tonight itself could not be fetched — a failed request
 * is not an empty calendar, and the caller renders those differently.
 */
export async function fetchSceneSlice(slug: string): Promise<SceneSliceData | null> {
  const tonight = await fetchSceneDay(slug)
  if (!tonight) return null
  const next = tonight.next_date ? await fetchSceneDay(slug, tonight.next_date) : null
  return buildSceneSlice(tonight, next)
}
