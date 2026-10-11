'use client'

import { useId, useState, type FocusEvent, type PointerEvent } from 'react'
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
import { cn } from '@/lib/utils'

import { COARSE_POINTER_HIT_AREA_CLASS } from './touchTarget'

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
 *   outside press, on Escape, and when focus moves past the trigger. Touch
 *   has no hover, and a Radix tooltip closes on pointer down, so the popover
 *   is the only surface a phone can reach.
 *
 * The popover never takes focus, even when its copy is pressed (which also
 * means a mouse cannot drag-select the copy): it holds no tabbable element,
 * and Radix's looping focus scope swallows Tab while focus sits on such a
 * container, so letting focus in would strand keyboard users. Since focus
 * never moves in, closing the popover restores nothing; Radix's default
 * return-focus is prevented, because focusing the trigger would reopen the
 * tooltip (and in Safari and Firefox on macOS a clicked button was never
 * focused to begin with). While the popover is open the trigger is described
 * by its copy, a description the closed tooltip does not provide.
 *
 * An Escape that closes the popover stops propagating, so a document-level
 * Escape handler on a surrounding panel does not close that panel too.
 *
 * `PopoverTrigger` wraps `TooltipTrigger`. `TooltipTrigger` spreads the props
 * it receives after its own `data-state`, so the button's `data-state` is the
 * popover's open/closed state, matching `aria-expanded`. Props set on the
 * `<button>` itself win over both triggers, even when undefined, so
 * `aria-describedby` is spread onto it only while the popover is open, which
 * is exactly when the tooltip sets none. The button's own handlers run before
 * the Radix ones, and Radix skips its handler for an event that is already
 * default-prevented.
 *
 * Tooltip opens are suppressed at the source, by default-preventing the event
 * Radix would open on, so a suppressed open never reaches Radix's provider:
 * - Hover (`pointermove`) is suppressed while the popover is open, so the
 *   tooltip never renders over it and none is left pending when it closes.
 *   The click that opens the popover closes any open tooltip itself.
 * - Focus is suppressed while the popover is open. The focus a tap gives
 *   the trigger is suppressed by `TooltipTrigger` itself, so a tap opens the
 *   popover without flashing the tooltip first.
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
  const popoverCopyId = useId()

  const handlePointerMove = (event: PointerEvent) => {
    if (popoverOpen) event.preventDefault()
  }
  const handleFocus = (event: FocusEvent) => {
    if (popoverOpen) event.preventDefault()
  }

  return (
    <TooltipProvider delayDuration={120}>
      <Popover open={popoverOpen} onOpenChange={setPopoverOpen}>
        <Tooltip open={tooltipOpen} onOpenChange={setTooltipOpen}>
          <PopoverTrigger asChild>
            <TooltipTrigger asChild>
              {/* select-none and the touch-callout reset keep a long press
                  from starting a text selection or the iOS callout menu.
                  The button is 18px (a 14px glyph in p-0.5); on coarse
                  pointers the invisible hit area takes the tap target to
                  24px without growing the glyph, the button or the row. */}
              <button
                type="button"
                aria-label={label}
                className={cn(
                  'inline-flex select-none items-center rounded-full p-0.5 text-muted-foreground transition-colors [-webkit-touch-callout:none] hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
                  COARSE_POINTER_HIT_AREA_CLASS
                )}
                data-testid={testId}
                {...(popoverOpen && { 'aria-describedby': popoverCopyId })}
                onPointerMove={handlePointerMove}
                onFocus={handleFocus}
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
            aria-label={label}
            onOpenAutoFocus={event => event.preventDefault()}
            onCloseAutoFocus={event => event.preventDefault()}
            onEscapeKeyDown={event => event.stopPropagation()}
            onMouseDown={event => event.preventDefault()}
            className="w-auto max-w-xs px-3 py-1.5 text-xs"
          >
            <span id={popoverCopyId}>{copy}</span>
          </PopoverContent>
        </Tooltip>
      </Popover>
    </TooltipProvider>
  )
}
