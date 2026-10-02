'use client'

import { useId, useRef, useState, type PointerEvent } from 'react'
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
 *   outside press, on Escape, and when focus moves past the trigger. Touch
 *   has no hover, and a Radix tooltip closes on pointer down, so the popover
 *   is the only surface a phone can reach.
 *
 * The popover never takes focus, even when its copy is pressed: it holds no
 * tabbable element, and Radix's looping focus scope swallows Tab while focus
 * sits on such a container, so letting focus in would strand keyboard users.
 * Since focus never moves in, closing the popover restores nothing; Radix's
 * default return-focus is prevented, because focusing the trigger would
 * reopen the tooltip (and in Safari and Firefox on macOS a clicked button
 * was never focused to begin with). While the popover is open the trigger is
 * described by its copy, since the forced-closed tooltip no longer does.
 *
 * `PopoverTrigger` wraps `TooltipTrigger`. `TooltipTrigger` spreads the props
 * it receives after its own `data-state`, so the button's `data-state` is the
 * popover's open/closed state, matching `aria-expanded`. Props set on the
 * `<button>` itself win over both triggers, even when undefined, so
 * `aria-describedby` is spread onto it only while the popover is open, which
 * is exactly when the tooltip sets none.
 *
 * The tooltip never renders while the popover is open: the click that opens
 * the popover also closes the tooltip (Radix's trigger click handler), and
 * open requests made while the popover is open are dropped rather than held,
 * so closing the popover never pops a stale tooltip.
 *
 * Tooltip open requests are also ignored while focus came from a touch:
 * Chromium focuses a tapped button after `pointerup`, which Radix reads as
 * keyboard focus and would flash the tooltip before the click opens the
 * popover. The flag lasts from a touch `pointerdown` until that
 * tap's click (or a cancel, a blur, or a non-touch pointer), so it never
 * outlives the tap on browsers that skip the focus.
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
  const focusedByTouchRef = useRef(false)
  const popoverCopyId = useId()

  const handleTooltipOpenChange = (next: boolean) => {
    if (next && (popoverOpen || focusedByTouchRef.current)) return
    setTooltipOpen(next)
  }
  const handlePointerDown = (event: PointerEvent) => {
    focusedByTouchRef.current = event.pointerType === 'touch'
  }
  const handlePointerMove = (event: PointerEvent) => {
    if (event.pointerType !== 'touch') focusedByTouchRef.current = false
  }
  const clearTouchFocus = () => {
    focusedByTouchRef.current = false
  }

  return (
    <TooltipProvider delayDuration={120}>
      <Popover open={popoverOpen} onOpenChange={setPopoverOpen}>
        <Tooltip
          open={tooltipOpen}
          onOpenChange={handleTooltipOpenChange}
        >
          <PopoverTrigger asChild>
            <TooltipTrigger asChild>
              {/* select-none and the touch-callout reset keep a long press
                  from starting a text selection or the iOS callout menu. */}
              <button
                type="button"
                aria-label={label}
                className="inline-flex select-none items-center rounded-full p-0.5 text-muted-foreground transition-colors [-webkit-touch-callout:none] hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                data-testid={testId}
                {...(popoverOpen && { 'aria-describedby': popoverCopyId })}
                onPointerDown={handlePointerDown}
                onPointerMove={handlePointerMove}
                onPointerCancel={clearTouchFocus}
                onClick={clearTouchFocus}
                onBlur={clearTouchFocus}
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
