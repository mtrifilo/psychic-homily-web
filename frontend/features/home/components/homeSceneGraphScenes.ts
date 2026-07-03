import { compareScenesByActivity } from '@/features/scenes'
import type { SceneListItem } from '@/features/scenes'

/**
 * Scene-selection rules for the homepage graph section (PSY-1344).
 *
 * Pure and separately testable — the component owns fetching and state;
 * these own which scene the section shows.
 */

/**
 * The section's default scene: the liveliest one, via the shared
 * `compareScenesByActivity` ordering (the same rule the atlas globe's
 * labels, search results, and mobile list use — surfaces must not
 * disagree about which scene is "first").
 *
 * Geo-personalization (defaulting to the visitor's own scene) is
 * deliberately not attempted here — see PSY-1346.
 */
export function pickDefaultScene(
  scenes: readonly SceneListItem[],
): SceneListItem | null {
  if (scenes.length === 0) return null
  return [...scenes].sort(compareScenesByActivity)[0]
}

/**
 * "Surprise me": a random scene other than the current one, preferring
 * scenes with upcoming shows so the surprise never lands on a dead graph.
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
