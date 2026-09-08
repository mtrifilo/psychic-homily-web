import { X } from 'lucide-react'
import { Badge } from '@/components/ui/badge'

interface RemovableFilterChipProps {
  /** What the chip names. Also the accessible name of its remove control. */
  label: string
  onRemove: () => void
  'data-testid'?: string
  /** Test id for the remove control; defaults to the chip's own plus `-remove`. */
  removeTestId?: string
}

/**
 * One engaged filter, with the control that drops it.
 *
 * Distinct from `FilterChip`, which is a TOGGLE with an optional count and no
 * remove affordance. This is the "active filter" register: it renders only
 * while the filter is on, and its only action is to turn it off.
 *
 * Shared so the surfaces that show an engaged filter cannot drift apart
 * visually. `type="button"` because a chip lives wherever the filters do,
 * which one day is inside a form, and a bare button submits it.
 */
export function RemovableFilterChip({
  label,
  onRemove,
  'data-testid': testId,
  removeTestId,
}: RemovableFilterChipProps) {
  return (
    <Badge
      variant="secondary"
      className="flex items-center gap-1 px-2.5 py-1 text-xs font-medium cursor-default"
      data-testid={testId}
    >
      {label}
      <button
        type="button"
        onClick={onRemove}
        className="ml-0.5 rounded-full hover:bg-foreground/10 p-0.5 transition-colors"
        aria-label={`Remove ${label} filter`}
        data-testid={removeTestId ?? (testId ? `${testId}-remove` : undefined)}
      >
        <X className="h-3 w-3" />
      </button>
    </Badge>
  )
}
