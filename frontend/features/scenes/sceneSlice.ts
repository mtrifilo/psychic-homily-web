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

import type { SceneDayResponse, SceneDayShow } from './sceneDay'

/** One whole calendar date of the slice, already bucketed by the backend. */
export interface SceneSliceDay {
  /** `YYYY-MM-DD`, resolved in the scene's own zone by the backend. */
  date: string
  /**
   * Whether this date is the scene's CURRENT night, per the backend's 6am
   * boundary. Never derived from a clock here — the viewer's clock is the one
   * clock that is never the right one for a scene page.
   */
  isTonight: boolean
  shows: SceneDayShow[]
}

/** Everything the root's calendar slice renders. */
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
  days: SceneSliceDay[]
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

  const days: SceneSliceDay[] = [
    {
      date: tonight.date,
      isTonight: tonight.is_tonight,
      shows: tonight.shows ?? [],
    },
  ]

  // Guarded on the DATE rather than on the payload alone. `fetchScenePeriod`
  // resolves the current period when its key is falsy, so a `next` fetched with
  // an empty `next_date` would come back as tonight AGAIN and the slice would
  // print the same night twice under two headings. The caller is responsible for
  // not making that request; this is the check that makes the mistake
  // unrenderable rather than merely unlikely.
  if (next && next.date && next.date !== tonight.date) {
    days.push({
      date: next.date,
      isTonight: next.is_tonight,
      shows: next.shows ?? [],
    })
  }

  return {
    // The backend's own answer for the scene, not a vote over the rows — so a
    // scene with nothing booked still names its clock, which the old row-derived
    // resolution could not do.
    timezone: tonight.timezone || undefined,
    days,
  }
}

/** Total shows across the slice. The rows are the truth; this counts them. */
export function sceneSliceShowCount(slice: SceneSliceData): number {
  return slice.days.reduce((n, day) => n + day.shows.length, 0)
}

/**
 * Whether the slice has nothing to list.
 *
 * A slice with no DAYS and one whose days are all empty are the same answer to
 * the reader, and both must reach the quiet copy rather than rendering a run of
 * bare date rules.
 */
export function sceneSliceIsQuiet(slice: SceneSliceData): boolean {
  return sceneSliceShowCount(slice) === 0
}
