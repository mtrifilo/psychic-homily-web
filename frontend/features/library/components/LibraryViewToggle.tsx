'use client'

import { cn } from '@/lib/utils'
import { BracketLink } from '@/components/shared'
import type { LibraryView } from '../hooks/useLibraryView'

export interface LibraryViewToggleProps {
  view: LibraryView
  onViewChange: (view: LibraryView) => void
  className?: string
}

/**
 * Dense mono linkbox: `view [ table ] [ wall ]` (PSY-1429 / Figma State G).
 * Active option uses BracketLink `active`; inactive stays muted.
 */
export function LibraryViewToggle({
  view,
  onViewChange,
  className,
}: LibraryViewToggleProps) {
  return (
    <div
      role="radiogroup"
      aria-label="Library view"
      className={cn(
        'inline-flex items-baseline gap-x-1.5 font-mono text-[11px] tabular-nums text-muted-foreground',
        className
      )}
    >
      <span aria-hidden="true">view</span>
      <BracketLink
        label="table"
        active={view === 'table'}
        aria-checked={view === 'table'}
        role="radio"
        onClick={() => onViewChange('table')}
        className="font-mono text-[11px]"
        ariaLabel="Table view"
        data-testid="library-view-table"
      />
      <BracketLink
        label="wall"
        active={view === 'wall'}
        aria-checked={view === 'wall'}
        role="radio"
        onClick={() => onViewChange('wall')}
        className="font-mono text-[11px]"
        ariaLabel="Wall view"
        data-testid="library-view-wall"
      />
    </div>
  )
}
