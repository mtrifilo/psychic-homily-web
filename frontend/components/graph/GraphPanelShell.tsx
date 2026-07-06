'use client'

/**
 * GraphPanelShell (PSY-1360) — the shared floating-panel chrome for the graph
 * inspector cards (ArtistContextPanel, ConnectionPanel).
 *
 * Both panels are non-modal DOM inspectors floated over the canvas: same
 * `w-72` bordered/blurred card, same top-right X close button, same
 * `<section aria-label>` region landmark (the escLayering / PSY-1351 tests
 * query these panels by role="region"). This owns that invariant chrome; the
 * caller supplies the header content, the body, and the per-panel spacing
 * (padding / max-height / vertical rhythm differ) via `className`.
 *
 * Escape handling lives here via Radix `DismissableLayer` (PSY-1355): every graph
 * panel joins Radix's one global layer stack, so Escape dismisses the TOPMOST
 * layer only. That gives — with no custom escape stack — a ⌘K palette / dialog
 * stacked over the panel winning Escape (PSY-1355), the ego panel dismissing
 * before its enclosing Radix Dialog (PSY-1351), and two graph panels dismissing
 * innermost-first (PSY-1360). Radix preventDefaults on dismiss in the capture
 * phase, so the fullscreen overlay's bubble listener (which skips
 * defaultPrevented) still defers to the panel (AC3).
 */

import type { ReactNode } from 'react'
import { X } from 'lucide-react'
import { DismissableLayer } from '@radix-ui/react-dismissable-layer'

import { cn } from '@/lib/utils'

export interface GraphPanelShellProps {
  /** Accessible name for the region landmark (e.g. "About Lightning Bolt"). */
  ariaLabel: string
  /** Accessible name for the X close button (e.g. "Close connection details"). */
  closeLabel: string
  onClose: () => void
  /** Left side of the header row, opposite the close button. */
  header: ReactNode
  /** Per-panel spacing: padding, max-height, and space-y vary by panel. */
  className?: string
  children?: ReactNode
}

export function GraphPanelShell({
  ariaLabel,
  closeLabel,
  onClose,
  header,
  className,
  children,
}: GraphPanelShellProps) {
  return (
    <DismissableLayer
      asChild
      onDismiss={onClose}
      // Inspectors close only via Escape or the X button — never outside-click /
      // focus-out. Preventing both keeps onDismiss to Escape only, preserving the
      // pre-refactor behavior (PSY-1355 AC: pointer/focus contracts unchanged).
      onPointerDownOutside={e => e.preventDefault()}
      onFocusOutside={e => e.preventDefault()}
    >
      <section
        aria-label={ariaLabel}
        className={cn(
          'w-72 max-w-[calc(100%-1rem)] overflow-y-auto rounded-md border border-border/50',
          'bg-background/95 backdrop-blur-sm text-xs shadow-lg',
          className,
        )}
      >
        <div className="flex items-start justify-between gap-2">
          {header}
          <button
            type="button"
            onClick={onClose}
            aria-label={closeLabel}
            className="shrink-0 rounded-sm p-0.5 text-muted-foreground hover:text-foreground hover:bg-muted/50"
          >
            <X className="h-3.5 w-3.5" aria-hidden="true" />
          </button>
        </div>
        {children}
      </section>
    </DismissableLayer>
  )
}
