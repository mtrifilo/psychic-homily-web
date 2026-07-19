import type { ChartScene } from '../types'

/**
 * Actionable alternatives when a scene-scoped chart yields no rows (PSY-1433).
 * Modest factual copy — chips call the existing scene switcher setter.
 */
export function ZeroResultSceneSuggestions({
  sceneLabel,
  suggestions,
  onSelect,
}: {
  sceneLabel: string
  suggestions: readonly ChartScene[]
  onSelect: (metro: string) => void
}) {
  if (suggestions.length === 0) return null

  return (
    <div
      data-testid="chart-zero-result-suggestions"
      className="border-y border-border py-4 text-sm text-muted-foreground"
    >
      <p>
        Nothing charting in {sceneLabel} this window — try{' '}
        {suggestions.map((scene, index) => (
          <span key={scene.metro}>
            {index > 0 ? (index === suggestions.length - 1 ? ', or ' : ', ') : null}
            <button
              type="button"
              onClick={() => onSelect(scene.metro)}
              className="text-primary hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              data-testid={`chart-suggest-scene-${scene.metro}`}
            >
              {scene.city}
            </button>
          </span>
        ))}
        .
      </p>
    </div>
  )
}
