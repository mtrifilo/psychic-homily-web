"use client"

import * as React from "react"
import { Popover as PopoverPrimitive } from "radix-ui"

import { cn } from "@/lib/utils"

const Popover = PopoverPrimitive.Root

const PopoverTrigger = PopoverPrimitive.Trigger

const PopoverAnchor = PopoverPrimitive.Anchor

/**
 * `--radix-popover-content-available-height` is the room from THIS element's
 * outer top edge down to the edge of the visual viewport, so the value already
 * accounts for a software keyboard. Capping anything nested inside leaves this
 * element's own border and padding to paint past that edge, which is why the
 * bound sits on the border box Radix positions rather than on the command
 * column (`command.tsx`).
 *
 * `flex flex-col` is what carries the cap inward: a block box overflows its own
 * `max-height`, while a column flex item shrinks into it, and `CommandList`'s
 * `overflow-y-auto` turns the difference into scroll.
 *
 * `:has([cmdk-root])` scopes all of it to popovers that frame a command column.
 * Every other popover keeps its natural, unbounded size.
 */
const COMMAND_COLUMN_BOUND =
  "has-[[cmdk-root]]:flex has-[[cmdk-root]]:flex-col has-[[cmdk-root]]:max-h-(--radix-popover-content-available-height)"

const PopoverContent = React.forwardRef<
  React.ElementRef<typeof PopoverPrimitive.Content>,
  React.ComponentPropsWithoutRef<typeof PopoverPrimitive.Content>
>(({ className, align = "start", sideOffset = 4, ...props }, ref) => (
  <PopoverPrimitive.Portal>
    <PopoverPrimitive.Content
      ref={ref}
      align={align}
      sideOffset={sideOffset}
      className={cn(
        "z-50 w-72 rounded-md border bg-popover p-0 text-popover-foreground outline-none data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95 data-[side=bottom]:slide-in-from-top-2 data-[side=left]:slide-in-from-right-2 data-[side=right]:slide-in-from-left-2 data-[side=top]:slide-in-from-bottom-2",
        COMMAND_COLUMN_BOUND,
        className
      )}
      {...props}
    />
  </PopoverPrimitive.Portal>
))
PopoverContent.displayName = PopoverPrimitive.Content.displayName

export { Popover, PopoverTrigger, PopoverContent, PopoverAnchor }
