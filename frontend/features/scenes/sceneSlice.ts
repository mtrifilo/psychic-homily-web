/**
 * The scene root's calendar SLICE: the pure half.
 *
 * The root page is the front page of a scene, not a calendar window (PSY-1850,
 * re-locking PSY-1729). It shows tonight and the next full day, and points at
 * the window family for anything longer. This module turns the two day payloads
 * behind that into the one shape the view renders, so the decisions that matter
 * — which dates the page is entitled to claim, and when it has nothing to say —
 * are decidable without a DOM.
 *
 * ## Why two day payloads rather than one window fetch
 *
 * Neither of the other two calendar endpoints can answer this question honestly:
 *
 *  - `GET /scenes/{slug}/shows` (what the root used to read) bounds its lower
 *    edge at `event_date >= now()` as a UTC INSTANT. A show whose doors have
 *    opened is already gone from it, and between midnight and 06:00 the night
 *    named by the scene's 6am boundary is YESTERDAY, which a forward window can
 *    never contain. That is the PSY-1807 gap, and it is why the old root could
 *    not draw a TONIGHT bucket at all.
 *  - `GET /scenes/{slug}/week` is Monday-anchored, so "the next full day" falls
 *    outside it every Sunday, and it cannot say WHICH date is tonight without
 *    re-deriving the 6am boundary from a clock on this side.
 *
 * `GET /scenes/{slug}/day` is the endpoint `/scenes/{slug}/tonight` itself
 * renders. Reading it here makes "the root's tonight rows match /tonight's rows"
 * true BY CONSTRUCTION — same bucketing, same timezone, same night boundary,
 * same authority — rather than true by a mirrored constant that drifts the first
 * time `nightStartHour` moves on one side only.
 *
 * The cost is one extra serial round trip: `next_date` is only knowable once the
 * first payload has answered which date tonight is. Both are server-side and
 * data-cached, and they replace a 28-day/61-row CLIENT fetch, so the reader pays
 * less either way.
 */

import type { SceneDayResponse } from './sceneDay'
import { countWindowShows } from './sceneWindow'

/**
 * Everything the root's calendar slice renders.
 *
 * The days are the PAYLOADS, not a hand-written projection of them. A parallel
 * view-model here would be a type that can claim a field the API never sends —
 * the exact hazard `sceneDay.ts` derives its own types from the generated
 * schema to avoid — and its only work would have been renaming `is_tonight`.
 * Consumers read `day.is_tonight` and go through `dayShows()` for the
 * generator-nullable `shows`, which is what every other day surface already
 * does.
 */
export interface SceneSliceData {
  /**
   * The IANA zone the backend bucketed these dates in. Carried so the page can
   * name the clock its times are printed in without a second request, and
   * without the modal-vote guess the old client-side calendar had to make.
   */
  timezone?: string
  /**
   * The dates the slice actually answered for — at most two, and possibly one
   * when the next day is past the servable window or its fetch failed.
   *
   * A date is present ONLY when a payload answered for it. An absent date is
   * never synthesized as an empty bucket: drawing "nothing on our calendar" for
   * a date we did not check would publish a zero in our own voice that nobody
   * verified, which is the exact defect the old root's missing TONIGHT bucket
   * existed to avoid.
   */
  days: SceneDayResponse[]
}

/**
 * Assemble the slice from the two day payloads.
 *
 * Returns null when tonight itself could not be fetched — the caller renders
 * "we could not load this" rather than an honest zero, because a failed request
 * is not an empty calendar.
 *
 * `next` is tolerated as null in every case that can produce one: the servable
 * window's far edge (where the backend sends an empty `next_date`), and a blip
 * on the second request. A one-day slice is a smaller loss than a fabricated
 * second day.
 */
export function buildSceneSlice(
  tonight: SceneDayResponse | null,
  next: SceneDayResponse | null
): SceneSliceData | null {
  if (!tonight) return null

  // Guarded on the DATE rather than on the payload alone. `fetchScenePeriod`
  // resolves the current period when its key is falsy, so a `next` fetched with
  // an empty `next_date` would come back as tonight AGAIN and the slice would
  // print the same night twice under two headings. The caller is responsible for
  // not making that request; this is the check that makes the mistake
  // unrenderable rather than merely unlikely.
  const isDistinctDay = next !== null && Boolean(next.date) && next.date !== tonight.date

  return {
    // The backend's own answer for the scene, not a vote over the rows — so a
    // scene with nothing booked still names its clock, which the old row-derived
    // resolution could not do.
    timezone: tonight.timezone || undefined,
    days: isDistinctDay ? [tonight, next] : [tonight],
  }
}

/**
 * Whether the slice has nothing to list.
 *
 * A slice with no DAYS and one whose days are all empty are the same answer to
 * the reader, and both must reach the quiet copy rather than rendering a run of
 * bare date rules.
 *
 * Counts through `countWindowShows` rather than a second `reduce`: the window
 * pages ask the identical question of an identical day list, and two spellings
 * of "how many shows are in these days" is one that a later change reaches and
 * one it does not.
 */
export function sceneSliceIsQuiet(slice: SceneSliceData): boolean {
  return countWindowShows(slice.days) === 0
}
