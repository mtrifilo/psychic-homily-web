import type { PlaceableScene } from './globeTypes'

/**
 * Weighted random scene pick for Atlas "Drift" (PSY-1308) — the radio.garden
 * "balloon ride" pattern: the system flies you somewhere alive and plays.
 *
 * Weight = upcoming_show_count + 1, so drifting usually lands on an active
 * scene but every scene stays reachable (a zero-show scene is never weight-0).
 * The currently-open scene is excluded so repeat-drifts always go somewhere
 * new — unless it's the only scene, in which case there's nowhere else to go
 * and we return null (the caller no-ops rather than "flying" in place).
 *
 * `rand` is injectable for deterministic tests; production callers omit it.
 */
export function pickDriftScene(
  scenes: PlaceableScene[],
  excludeSlug?: string,
  rand: () => number = Math.random,
): PlaceableScene | null {
  const candidates = excludeSlug
    ? scenes.filter((s) => s.slug !== excludeSlug)
    : scenes
  if (candidates.length === 0) return null

  const weights = candidates.map((s) =>
    Number.isFinite(s.upcoming_show_count) && s.upcoming_show_count > 0
      ? s.upcoming_show_count + 1
      : 1,
  )
  const total = weights.reduce((sum, w) => sum + w, 0)

  // rand() ∈ [0, 1) scaled onto the cumulative weight line; the final return
  // guards floating-point edge accumulation.
  let threshold = rand() * total
  for (let i = 0; i < candidates.length; i++) {
    threshold -= weights[i]
    if (threshold < 0) return candidates[i]
  }
  return candidates[candidates.length - 1]
}
