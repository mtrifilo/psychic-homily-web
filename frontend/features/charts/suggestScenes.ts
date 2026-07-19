import type { ChartScene } from './types'

/**
 * Pick alternative scenes for a zero-result chart empty state (PSY-1433).
 *
 * `/charts/scenes` is already ordered busiest-first (show_count DESC) and is
 * already fetched for the scene switcher — reuse that payload, exclude the
 * current metro, take the top N. No proximity join: ChartScene omits centroids
 * (they exist server-side on MetroPrincipal only), and a second `/scenes` fetch
 * isn't justified for v1 global suggestions.
 */
export function suggestAlternativeScenes(
  scenes: readonly ChartScene[],
  currentMetro: string,
  limit = 3
): ChartScene[] {
  if (!currentMetro || limit <= 0) return []
  return scenes.filter(scene => scene.metro !== currentMetro).slice(0, limit)
}
