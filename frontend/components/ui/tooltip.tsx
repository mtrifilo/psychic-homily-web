'use client'

import * as React from 'react'
import * as TooltipPrimitive from '@radix-ui/react-tooltip'

import { cn } from '@/lib/utils'

const TooltipProvider = TooltipPrimitive.Provider

const Tooltip = TooltipPrimitive.Root

/**
 * Radix tooltip trigger that does not open its tooltip on a touch or pen tap.
 *
 * Chromium focuses a tapped element after `pointerup`, when Radix has stopped
 * tracking the press, so Radix reads that focus as keyboard focus and opens
 * the tooltip on every tap. A touch or pen `pointerdown` arms a flag, and the
 * next focus consumes it and is default-prevented, which Radix's own focus
 * handler respects. The tap's click and a `pointercancel` also clear the flag
 * (a tap that never focuses, a touch cancelled by a scroll), and a mouse
 * `pointerdown` disarms it, so it suppresses at most the one focus that
 * follows a tap: hover and keyboard focus still open the tooltip.
 *
 * `revealOnTouch` turns the suppression off. It is for a trigger whose tap
 * has no other effect, because it wraps a non-interactive or disabled
 * control: there the tap's focus is the only way a touch user reaches the
 * tooltip, and the next tap on the trigger or a tap outside closes it.
 *
 * The caller's handlers run first, and the flag is updated even when one of
 * them default-prevents the event.
 */
const TooltipTrigger = React.forwardRef<
  React.ElementRef<typeof TooltipPrimitive.Trigger>,
  React.ComponentPropsWithoutRef<typeof TooltipPrimitive.Trigger> & {
    /** Open the tooltip on the focus a touch or pen tap gives the trigger. */
    revealOnTouch?: boolean
  }
>(
  (
    {
      revealOnTouch = false,
      onPointerDown,
      onPointerCancel,
      onFocus,
      onClick,
      ...props
    },
    ref
  ) => {
    const tapPressedRef = React.useRef(false)

    return (
      <TooltipPrimitive.Trigger
        ref={ref}
        {...props}
        onPointerDown={event => {
          onPointerDown?.(event)
          tapPressedRef.current =
            !revealOnTouch &&
            (event.pointerType === 'touch' || event.pointerType === 'pen')
        }}
        onPointerCancel={event => {
          onPointerCancel?.(event)
          tapPressedRef.current = false
        }}
        onFocus={event => {
          onFocus?.(event)
          if (tapPressedRef.current) event.preventDefault()
          tapPressedRef.current = false
        }}
        onClick={event => {
          onClick?.(event)
          tapPressedRef.current = false
        }}
      />
    )
  }
)
TooltipTrigger.displayName = TooltipPrimitive.Trigger.displayName

const TooltipContent = React.forwardRef<
  React.ElementRef<typeof TooltipPrimitive.Content>,
  React.ComponentPropsWithoutRef<typeof TooltipPrimitive.Content>
>(({ className, sideOffset = 4, ...props }, ref) => (
  <TooltipPrimitive.Portal>
    <TooltipPrimitive.Content
      ref={ref}
      sideOffset={sideOffset}
      className={cn(
        'z-50 overflow-hidden rounded-md border bg-popover px-3 py-1.5 text-sm text-popover-foreground animate-in fade-in-0 zoom-in-95 data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=closed]:zoom-out-95 data-[side=bottom]:slide-in-from-top-2 data-[side=left]:slide-in-from-right-2 data-[side=right]:slide-in-from-left-2 data-[side=top]:slide-in-from-bottom-2',
        className
      )}
      {...props}
    />
  </TooltipPrimitive.Portal>
))
TooltipContent.displayName = TooltipPrimitive.Content.displayName

export { Tooltip, TooltipTrigger, TooltipContent, TooltipProvider }
