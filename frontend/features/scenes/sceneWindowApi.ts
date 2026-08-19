/**
 * Reading a MULTI-week window from the API.
 *
 * The window pages that span more than one week compose the existing week
 * endpoint rather than asking for a new one. That is a correctness decision
 * before it is a scope decision: `GET /scenes/{slug}/shows` bounds its lower
 * edge at `event_date >= now()` as a UTC instant, and a date-only show is
 * stored at UTC midnight, so tonight's date-only shows have already fallen out
 * of it (verified against a live scene, PSY-1849). The week endpoint bounds on
 * scene-local calendar dates and is the only source here that answers "what is
 * on tonight" correctly.
 */
import { fetchSceneWeek } from './sceneWeekApi'
import type { SceneWeekResponse } from './sceneWeek'

/**
 * `count` consecutive weeks, starting from the scene's CURRENT week.
 *
 * The chain FOLLOWS each payload's own `next_week` rather than computing the
 * keys. ISO week arithmetic has to know which years carry 53 weeks, and getting
 * that wrong at a year boundary would silently skip or repeat a week — the
 * backend already owns that calendar maths and publishes the answer, so this
 * asks it instead of keeping a second implementation that can drift.
 *
 * The cost is that the requests are SERIAL: each key is only known once the
 * previous payload lands. Acceptable because every one of them is a
 * `next: { revalidate }` data-cache read that the whole scene family shares —
 * the current week is already fetched by the scene page and by `/week`, so a
 * warm cache makes this a handful of cache hits.
 *
 * Returns null only when the FIRST week fails, which is the case that means
 * "this scene does not exist" and must reach `notFound()`. A later week failing
 * returns the shorter chain instead: a blip four weeks out is not a reason to
 * 404 a page whose first three weeks loaded, and the header names the span it
 * actually rendered, so a short chain understates rather than lies.
 */
export async function fetchSceneWeekChain(
  slug: string,
  count: number
): Promise<SceneWeekResponse[] | null> {
  const first = await fetchSceneWeek(slug, undefined, 'scene-week')
  if (!first) return null

  const weeks: SceneWeekResponse[] = [first]
  // Keys already walked. `next_week` is an untrusted wire value, and a payload
  // that points at itself — or back at a week already in the chain — would
  // otherwise repeat those days N times: duplicate dates flow into the window,
  // render under duplicate React keys, and print the same night twice. Stopping
  // at the first repeat degrades to a shorter, still-correct window, which the
  // header then names honestly.
  const seen = new Set<string>([first.iso_week])
  for (let i = 1; i < count; i++) {
    const nextKey = weeks[weeks.length - 1].next_week
    if (!nextKey || seen.has(nextKey)) break
    seen.add(nextKey)
    const next = await fetchSceneWeek(slug, nextKey, 'scene-week')
    if (!next) break
    weeks.push(next)
  }
  return weeks
}
