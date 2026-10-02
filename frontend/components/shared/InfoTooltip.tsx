'use client'

import { useState } from 'react'
import { Info } from 'lucide-react'

import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'

export interface InfoTooltipProps {
  /** Explainer copy shown in both the hover tooltip and the tap popover. */
  copy: string
  /** Accessible name for the trigger button. */
  label: string
  /** Placement relative to the glyph for both surfaces (default 'top'). */
  side?: 'top' | 'bottom' | 'left' | 'right'
  /** testid passthrough so call sites keep their existing test hooks. */
  testId?: string
}

/**
 * Shared ⓘ-glyph explainer. One trigger button reveals the same copy two ways:
 *
 * - Hover and keyboard focus show a Radix tooltip (pointer users skim it).
 * - Click, tap, Enter and Space toggle a DS popover, which closes on an
 *   outside press and on Escape. Touch has no hover, and a Radix tooltip
 *   closes on pointer down, so the popover is the only surface a phone can
 *   reach.
 *
 * `PopoverTrigger` wraps `TooltipTrigger`: an outer Slot's props override the
 * inner trigger's defaults, so the button's `data-state` is the popover's
 * open/closed state (matching `aria-expanded`), and the popover toggles
 * before the tooltip's own click handler closes the tooltip.
 *
 * The tooltip is suppressed while the popover is open, so the two never
 * render at once (a pointer re-entering the glyph over an open popover asks
 * the tooltip to open; that request is held until the popover closes).
 *
 * `select-none` and `-webkit-touch-callout: none` keep a long press on the
 * glyph from starting a text selection or the iOS callout menu.
 *
 * Owns its own `TooltipProvider` so it drops in anywhere without a
 * surrounding provider.
 *
 * Import via the subpath (`@/components/shared/InfoTooltip`), NOT the feature
 * barrel: a barrel import re-bloats the Turbopack global shared chunk on
 * browse-route-reachable call sites.
 *
 * Usage:
 *   <InfoTooltip copy={text} label="How tag filtering works" testId="…" />
 */
export function InfoTooltip({
  copy,
  label,
  side = 'top',
  testId,
}: InfoTooltipProps) {
  const [popoverOpen, setPopoverOpen] = useState(false)
  const [tooltipOpen, setTooltipOpen] = useState(false)

  return (
    <TooltipProvider delayDuration={120}>
      <Popover open={popoverOpen} onOpenChange={setPopoverOpen}>
        <Tooltip
          open={tooltipOpen && !popoverOpen}
          onOpenChange={setTooltipOpen}
        >
          <PopoverTrigger asChild>
            <TooltipTrigger asChild>
              <button
                type="button"
                aria-label={label}
                className="inline-flex select-none items-center rounded-full p-0.5 text-muted-foreground transition-colors [-webkit-touch-callout:none] hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                data-testid={testId}
              >
                <Info className="h-3.5 w-3.5" aria-hidden />
              </button>
            </TooltipTrigger>
          </PopoverTrigger>
          <TooltipContent side={side} className="max-w-xs text-xs">
            {copy}
          </TooltipContent>
          <PopoverContent
            side={side}
            align="center"
            className="w-auto max-w-xs px-3 py-1.5 text-xs"
          >
            {copy}
          </PopoverContent>
        </Tooltip>
      </Popover>
    </TooltipProvider>
  )
}
