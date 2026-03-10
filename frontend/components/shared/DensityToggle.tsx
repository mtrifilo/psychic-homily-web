'use client'

import { cn } from '@/lib/utils'
import { type Density, useDensity } from '@/lib/hooks/common/useDensity'

const DENSITY_OPTIONS: { value: Density; label: string }[] = [
  { value: 'compact', label: 'Compact' },
  { value: 'comfortable', label: 'Comfortable' },
  { value: 'expanded', label: 'Expanded' },
]

export interface DensityToggleProps {
  /** localStorage key suffix for the density preference (e.g., 'shows', 'artists') */
  storageKey?: string
  /** Additional CSS classes */
  className?: string
}

/**
 * A toggle control for switching between compact, comfortable, and expanded density modes.
 * Persists the preference in localStorage.
 *
 * Usage:
 *   <DensityToggle storageKey="shows" />
 */
export function DensityToggle({ storageKey, className }: DensityToggleProps) {
  const { density, setDensity } = useDensity(storageKey)

  return (
    <div
      className={cn('inline-flex items-center rounded-lg border border-border/50 bg-muted/30 p-0.5', className)}
      role="radiogroup"
      aria-label="Display density"
    >
      {DENSITY_OPTIONS.map(option => (
        <button
          key={option.value}
          type="button"
          role="radio"
          aria-checked={density === option.value}
          onClick={() => setDensity(option.value)}
          className={cn(
            'px-2.5 py-1 text-xs font-medium rounded-md transition-colors duration-100',
            density === option.value
              ? 'bg-background text-foreground shadow-sm'
              : 'text-muted-foreground hover:text-foreground'
          )}
        >
          {option.label}
        </button>
      ))}
    </div>
  )
}
