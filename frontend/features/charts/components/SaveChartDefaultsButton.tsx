'use client'

import { Loader2 } from 'lucide-react'
import { useSetChartDefaults } from '@/features/auth'
import type { ChartDefaults, ChartDefaultWindow } from '@/features/auth/hooks/useChartDefaults'

interface SaveChartDefaultsButtonProps {
  window: ChartDefaultWindow
  scene: string | null
  savedDefaults: ChartDefaults | null
}

function defaultsEqual(
  window: ChartDefaultWindow,
  scene: string | null,
  saved: ChartDefaults | null
): boolean {
  if (!saved) return false
  return saved.window === window && (saved.scene ?? null) === (scene ?? null)
}

/**
 * Explicit save/clear affordance for /charts defaults (PSY-1423).
 * Mirrors SaveDefaultsButton styling used on /shows — text-xs link, no novel masthead control.
 */
export function SaveChartDefaultsButton({
  window,
  scene,
  savedDefaults,
}: SaveChartDefaultsButtonProps) {
  const setChartDefaults = useSetChartDefaults()

  const handleSave = () => {
    setChartDefaults.mutate({ window, scene })
  }

  const handleClear = () => {
    setChartDefaults.mutate(null)
  }

  if (setChartDefaults.isPending) {
    return (
      <span className="text-xs text-muted-foreground flex items-center gap-1 self-center">
        <Loader2 className="h-3 w-3 animate-spin" />
        Saving...
      </span>
    )
  }

  const isAnonymousDefault = window === 'quarter' && !scene
  const matchesSaved = defaultsEqual(window, scene, savedDefaults)

  if (matchesSaved) return null

  // At anonymous defaults with something saved → Clear
  if (isAnonymousDefault && savedDefaults) {
    return (
      <button
        type="button"
        onClick={handleClear}
        className="text-xs text-muted-foreground hover:text-primary hover:underline underline-offset-2 transition-colors self-center whitespace-nowrap"
      >
        Clear defaults
      </button>
    )
  }

  // Non-anonymous selection (or differs from saved) → Save
  if (!isAnonymousDefault) {
    return (
      <button
        type="button"
        onClick={handleSave}
        className="text-xs text-primary hover:text-primary/80 hover:underline underline-offset-2 transition-colors self-center whitespace-nowrap"
      >
        Save as default
      </button>
    )
  }

  return null
}
