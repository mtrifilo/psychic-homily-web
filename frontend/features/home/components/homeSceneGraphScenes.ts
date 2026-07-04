// Deep imports (not the '@/features/scenes' barrel) — the barrel pulls the
// scenes component tree into any static importer's bundle; see the note in
// HomeSceneGraph.tsx.
import { compareScenesByActivity } from '@/features/scenes/components/globeScale'
import type { SceneListItem } from '@/features/scenes/types'
import type { GeoLocation } from '@/lib/geo-default'
import { matchByGeo } from '@/lib/geo-client'

/**
 * Scene-selection rules for the homepage graph section (PSY-1344).
 *
 * Pure and separately testable — the component owns fetching and state;
 * these own which scene the section shows.
 */

/**
 * The section's default scene.
 *
 * With a geo suggestion (the visitor's IP-derived `{city,state}`+coords from
 * `/api/geo`, PSY-1346), prefer the visitor's OWN scene so a newcomer lands on
 * their local graph — the same personalization Upcoming Shows already has. The
 * match runs through the shared two-tier `matchByGeo` (exact city/state, else
 * nearest scene by haversine over the scene centroids — PSY-981) so it agrees
 * with the shows city default about what "the visitor's place" means.
 *
 * The matched scene wins ONLY if it's active (`upcoming_show_count > 0`),
 * mirroring `pickSurpriseScene`'s guard: the homepage graph is a newcomer's
 * first impression, so it must not default onto a dead/empty scene when a
 * lively one exists. (Activity is a venue-keyed proxy for liveliness, not a
 * graph-density guarantee — the empty-graph fallback stays load-bearing for the
 * residual case of an active scene with a sparse roster.) An inactive or absent
 * match falls back to the liveliest scene via the shared
 * `compareScenesByActivity` ordering — the same rule the atlas globe's labels,
 * search results, and mobile list use, so surfaces never disagree about which
 * scene is "first".
 *
 * `geo` is a SUGGESTION only: the component keeps it overridable — a
 * "Surprise me" pick (or any scene the visitor already selected) wins over it.
 */
export function pickDefaultScene(
  scenes: readonly SceneListItem[],
  geo?: GeoLocation | null,
): SceneListItem | null {
  if (scenes.length === 0) return null
  if (geo) {
    const local = matchByGeo(scenes, geo, {
      city: s => s.city,
      state: s => s.state,
      lat: s => s.latitude,
      lng: s => s.longitude,
    })
    if (local && local.upcoming_show_count > 0) return local
  }
  return [...scenes].sort(compareScenesByActivity)[0]
}

/**
 * "Surprise me": a random scene other than the current one, preferring
 * scenes with upcoming shows so the surprise rarely lands on a dead graph
 * (upcoming shows are venue-keyed while graph nodes are based-here roster
 * artists — PSY-1255 — so a shows-but-no-roster scene can still land on
 * the empty-graph fallback; that fallback is load-bearing, not dead code).
 * Falls back to any other scene when none have upcoming shows; returns
 * null when there is nothing to rotate to (0–1 scenes total).
 *
 * `random` is injectable for tests; callers use the default.
 */
export function pickSurpriseScene(
  scenes: readonly SceneListItem[],
  currentSlug: string | null,
  random: () => number = Math.random,
): SceneListItem | null {
  const others = scenes.filter(s => s.slug !== currentSlug)
  const active = others.filter(s => s.upcoming_show_count > 0)
  const pool = active.length > 0 ? active : others
  if (pool.length === 0) return null
  // Clamp so an inclusive random() === 1 (possible with injected sources)
  // can't index one past the end.
  const index = Math.min(pool.length - 1, Math.floor(random() * pool.length))
  return pool[index]
}
