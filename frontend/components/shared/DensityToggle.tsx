'use client'

import { cn } from '@/lib/utils'
import { type Density } from '@/lib/hooks/common/useDensity'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'

const DENSITY_OPTIONS: { value: Density; label: string }[] = [
  { value: 'compact', label: 'Compact' },
  { value: 'comfortable', label: 'Comfortable' },
  { value: 'expanded', label: 'Expanded' },
]

export interface DensityToggleProps {
  /** Current density value (from parent's useDensity hook) */
  density: Density
  /** Density setter (from parent's useDensity hook) */
  onDensityChange: (value: Density) => void
  /** Additional CSS classes */
  className?: string
  /**
   * Visually present the radios but disable interaction. The persisted
   * selection is preserved so re-enabling restores the previous choice
   * (PSY-556). Use this when the surrounding view doesn't apply density
   * (e.g. a list layout) but the parent still wants the control visible
   * to avoid layout shift on view-mode switch.
   */
  disabled?: boolean
  /**
   * Tooltip text shown on hover/focus when {@link disabled} is true.
   * Ignored when not disabled.
   */
  disabledTooltip?: string
}

/**
 * A toggle control for switching between compact, comfortable, and expanded density modes.
 * The parent component owns the density state via useDensity() and passes it down.
 *
 * Usage:
 *   const { density, setDensity } = useDensity('shows')
 *   <DensityToggle density={density} onDensityChange={setDensity} />
 */
export function DensityToggle({
  density,
  onDensityChange,
  className,
  disabled = false,
  disabledTooltip,
}: DensityToggleProps) {
  const group = (
    <div
      className={cn(
        'inline-flex items-center rounded-lg border border-border/50 bg-muted/30 p-0.5',
        disabled && 'opacity-50',
        className
      )}
      role="radiogroup"
      aria-label="Display density"
      aria-disabled={disabled || undefined}
    >
      {DENSITY_OPTIONS.map(option => (
        <button
          key={option.value}
          type="button"
          role="radio"
          aria-checked={density === option.value}
          disabled={disabled}
          onClick={() => onDensityChange(option.value)}
          data-testid={`density-${option.value}`}
          className={cn(
            'px-2.5 py-1 text-xs font-medium rounded-md transition-colors duration-100',
            density === option.value
              ? 'bg-background text-foreground shadow-sm'
              : 'text-muted-foreground hover:text-foreground',
            disabled && 'cursor-not-allowed hover:text-muted-foreground'
          )}
        >
          {option.label}
        </button>
      ))}
    </div>
  )

  // When disabled with a tooltip, wrap the group in a span trigger so
  // hover/focus still register (Radix Tooltip won't fire pointer events
  // on disabled buttons themselves).
  if (disabled && disabledTooltip) {
    return (
      <TooltipProvider delayDuration={300}>
        <Tooltip>
          <TooltipTrigger asChild>
            <span tabIndex={0} className="inline-flex">
              {group}
            </span>
          </TooltipTrigger>
          <TooltipContent>{disabledTooltip}</TooltipContent>
        </Tooltip>
      </TooltipProvider>
    )
  }

  return group
}
