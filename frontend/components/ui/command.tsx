'use client'

import * as React from 'react'
import { type DialogProps } from '@radix-ui/react-dialog'
import { Command as CommandPrimitive } from 'cmdk'
import { Search } from 'lucide-react'

import { cn } from '@/lib/utils'
import { Dialog, DialogContent, DialogTitle } from '@/components/ui/dialog'
import { PopoverContent } from '@/components/ui/popover'

/**
 * This column carries a floor and no ceiling. The ceiling belongs to whichever
 * frame the column sits in, and `CommandPopoverContent` below is the one that
 * sets it.
 *
 * `min-h-11` is the floor, and it is `CommandInput`'s own `h-11` so the two
 * scale together with the reader's text size. Below it this column's
 * `overflow-hidden` would clip the field being typed into, which is reachable
 * at 200% text on a landscape phone with the keyboard up. The list reaching
 * zero there is the correct outcome: a row shown at that size would be under
 * the keyboard.
 *
 * `overflow-hidden` also zeroes this column's automatic minimum size, which is
 * what lets it shrink into its frame's cap instead of forcing the frame open.
 * The floor outranks that, and outranks the cap: where the room on screen falls
 * below one input row, the column holds at `min-h-11` and paints past the
 * frame's bottom edge, which nothing clips. Showing the field being typed into
 * is worth more than the frame staying whole at a size that fits neither.
 */
const Command = React.forwardRef<
  React.ElementRef<typeof CommandPrimitive>,
  React.ComponentPropsWithoutRef<typeof CommandPrimitive>
>(({ className, ...props }, ref) => (
  <CommandPrimitive
    ref={ref}
    className={cn(
      'flex h-full w-full min-h-11 flex-col overflow-hidden rounded-md bg-popover text-popover-foreground',
      className
    )}
    {...props}
  />
))
Command.displayName = CommandPrimitive.displayName

// PSY-1019: CommandDialog is consumed ONLY by the Cmd+K CommandPalette
// (CityFilters uses the bare Command primitives), so the editorial 656px frame
// per Figma 539:5 lives here — min() keeps side margins on 640–655px
// viewports where a flat 656px would run edge-to-edge. Group-heading styling
// is NOT set here: CommandGroup's own defaults + the palette's per-group
// groupClassName (CommandPalette.tsx) are the single source, merged on one
// element so there's no cross-element CSS-order ambiguity.
function CommandDialog({ children, ...props }: DialogProps) {
  return (
    <Dialog {...props}>
      <DialogContent className="overflow-hidden p-0 sm:max-w-[min(656px,calc(100vw-2rem))] [&>button:last-child]:hidden">
        <DialogTitle className="sr-only">Command palette</DialogTitle>
        <Command>{children}</Command>
      </DialogContent>
    </Dialog>
  )
}

/**
 * The popover frame for a command surface, and the mirror of `CommandDialog`
 * above: the framing a command surface needs lives here, so `popover.tsx` knows
 * nothing about cmdk.
 *
 * `PopoverContent` is both the border box Radix positions and the box
 * `--radix-popover-content-available-height` is measured from, so the cap
 * belongs on it. A cap on the column inside instead leaves this frame's own top
 * and bottom borders to paint past the edge that value was measured from: on a
 * landscape phone with the keyboard up, 2px of popover below the keyboard.
 *
 * `flex flex-col` is what carries the cap inward. A block box overflows its own
 * `max-height`; a column flex item shrinks into it, and `CommandList`'s
 * `overflow-y-auto` turns the difference into scroll.
 *
 * Pair `Command` with this, never with a bare `PopoverContent`: that pairing
 * renders a surface nothing bounds to the room on screen.
 */
const CommandPopoverContent = React.forwardRef<
  React.ElementRef<typeof PopoverContent>,
  React.ComponentPropsWithoutRef<typeof PopoverContent>
>(({ className, ...props }, ref) => (
  <PopoverContent
    ref={ref}
    className={cn(
      'flex max-h-(--radix-popover-content-available-height) flex-col',
      className
    )}
    {...props}
  />
))
CommandPopoverContent.displayName = 'CommandPopoverContent'

const CommandInput = React.forwardRef<
  React.ElementRef<typeof CommandPrimitive.Input>,
  React.ComponentPropsWithoutRef<typeof CommandPrimitive.Input>
>(({ className, ...props }, ref) => (
  <div className="flex items-center border-b border-border/50 px-3" cmdk-input-wrapper="">
    <Search className="mr-2 h-4 w-4 shrink-0 opacity-50" />
    <CommandPrimitive.Input
      ref={ref}
      className={cn(
        'flex h-11 w-full rounded-md bg-transparent py-3 text-sm outline-none placeholder:text-muted-foreground disabled:cursor-not-allowed disabled:opacity-50',
        className
      )}
      {...props}
    />
  </div>
))
CommandInput.displayName = CommandPrimitive.Input.displayName

const CommandList = React.forwardRef<
  React.ElementRef<typeof CommandPrimitive.List>,
  React.ComponentPropsWithoutRef<typeof CommandPrimitive.List>
>(({ className, ...props }, ref) => (
  <CommandPrimitive.List
    ref={ref}
    className={cn('max-h-[300px] overflow-y-auto overflow-x-hidden', className)}
    {...props}
  />
))
CommandList.displayName = CommandPrimitive.List.displayName

const CommandEmpty = React.forwardRef<
  React.ElementRef<typeof CommandPrimitive.Empty>,
  React.ComponentPropsWithoutRef<typeof CommandPrimitive.Empty>
>((props, ref) => (
  <CommandPrimitive.Empty
    ref={ref}
    className="py-6 text-center text-sm text-muted-foreground"
    {...props}
  />
))
CommandEmpty.displayName = CommandPrimitive.Empty.displayName

const CommandGroup = React.forwardRef<
  React.ElementRef<typeof CommandPrimitive.Group>,
  React.ComponentPropsWithoutRef<typeof CommandPrimitive.Group>
>(({ className, ...props }, ref) => (
  <CommandPrimitive.Group
    ref={ref}
    className={cn(
      'overflow-hidden p-1 text-foreground [&_[cmdk-group-heading]]:px-2 [&_[cmdk-group-heading]]:py-1.5 [&_[cmdk-group-heading]]:text-xs [&_[cmdk-group-heading]]:font-medium [&_[cmdk-group-heading]]:text-muted-foreground',
      className
    )}
    {...props}
  />
))
CommandGroup.displayName = CommandPrimitive.Group.displayName

const CommandSeparator = React.forwardRef<
  React.ElementRef<typeof CommandPrimitive.Separator>,
  React.ComponentPropsWithoutRef<typeof CommandPrimitive.Separator>
>(({ className, ...props }, ref) => (
  <CommandPrimitive.Separator
    ref={ref}
    className={cn('-mx-1 h-px bg-border', className)}
    {...props}
  />
))
CommandSeparator.displayName = CommandPrimitive.Separator.displayName

const CommandItem = React.forwardRef<
  React.ElementRef<typeof CommandPrimitive.Item>,
  React.ComponentPropsWithoutRef<typeof CommandPrimitive.Item>
>(({ className, ...props }, ref) => (
  <CommandPrimitive.Item
    ref={ref}
    className={cn(
      "relative flex cursor-default gap-2 select-none items-center rounded-sm px-2 py-1.5 text-sm outline-none data-[disabled=true]:pointer-events-none data-[selected='true']:bg-muted data-[selected='true']:text-foreground data-[disabled=true]:opacity-50 [&_svg]:pointer-events-none [&_svg]:size-4 [&_svg]:shrink-0",
      className
    )}
    {...props}
  />
))
CommandItem.displayName = CommandPrimitive.Item.displayName

const CommandShortcut = ({
  className,
  ...props
}: React.HTMLAttributes<HTMLSpanElement>) => {
  return (
    <span
      className={cn(
        'ml-auto text-xs tracking-widest text-muted-foreground',
        className
      )}
      {...props}
    />
  )
}
CommandShortcut.displayName = 'CommandShortcut'

export {
  Command,
  CommandDialog,
  CommandPopoverContent,
  CommandInput,
  CommandList,
  CommandEmpty,
  CommandGroup,
  CommandItem,
  CommandShortcut,
  CommandSeparator,
}
