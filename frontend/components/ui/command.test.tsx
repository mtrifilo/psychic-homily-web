import { describe, it, expect, vi, beforeAll } from 'vitest'
import { createRef } from 'react'
import { render, screen } from '@testing-library/react'
import {
  Command,
  CommandDialog,
  CommandInput,
  CommandList,
  CommandEmpty,
  CommandGroup,
  CommandItem,
  CommandSeparator,
  CommandShortcut,
  CommandPopoverContent,
} from './command'
import { Popover, PopoverTrigger } from './popover'

/**
 * Radix publishes this on popover content. Custom properties inherit, so every
 * descendant of that element reads it; nothing outside a popover sets it.
 */
const AVAILABLE_HEIGHT_VAR = '--radix-popover-content-available-height'
const BOUND_CLASS = `max-h-(${AVAILABLE_HEIGHT_VAR})`

// jsdom does not implement scrollIntoView (required by cmdk).
beforeAll(() => {
  Element.prototype.scrollIntoView = vi.fn()
})

describe('Command', () => {
  it('renders the command list structure without throwing', () => {
    render(
      <Command>
        <CommandInput placeholder="Search..." />
        <CommandList>
          <CommandEmpty>No results</CommandEmpty>
          <CommandGroup heading="Group">
            <CommandItem>Item one</CommandItem>
            <CommandSeparator />
            <CommandItem>
              Item two
              <CommandShortcut>⌘K</CommandShortcut>
            </CommandItem>
          </CommandGroup>
        </CommandList>
      </Command>
    )
    expect(screen.getByPlaceholderText('Search...')).toBeInTheDocument()
    expect(screen.getByText('Item one')).toBeInTheDocument()
    expect(screen.getByText('Item two')).toBeInTheDocument()
    expect(screen.getByText('⌘K')).toBeInTheDocument()
  })

  it('merges a custom className on the root', () => {
    const { container } = render(<Command className="custom-class" />)
    expect(container.firstChild).toHaveClass('custom-class')
  })

  it('forwards a ref to the command root', () => {
    const ref = createRef<HTMLDivElement>()
    render(<Command ref={ref} />)
    expect(ref.current).toBeInstanceOf(HTMLDivElement)
  })

  it('forwards a ref to the command input', () => {
    const ref = createRef<HTMLInputElement>()
    render(
      <Command>
        <CommandInput ref={ref} placeholder="Search..." />
      </Command>
    )
    expect(ref.current).toBeInstanceOf(HTMLInputElement)
  })

  it('merges a custom className on the input', () => {
    render(
      <Command>
        <CommandInput className="custom-class" placeholder="Search..." />
      </Command>
    )
    expect(screen.getByPlaceholderText('Search...')).toHaveClass('custom-class')
  })
})

/**
 * jsdom has no layout, so the geometry these classes produce is asserted in
 * `e2e/pages/command-popover-keyboard.spec.ts`. What is checkable here is the
 * placement contract that geometry rests on: which element carries the cap and
 * which carries the floor, and that a consumer can replace either.
 */
describe('Command surface bounds', () => {
  function renderCommand(className?: string) {
    render(
      <Command data-testid="command" className={className}>
        <CommandInput placeholder="Search..." />
        <CommandList data-testid="list">
          <CommandGroup>
            <CommandItem>Item one</CommandItem>
          </CommandGroup>
        </CommandList>
      </Command>
    )
  }

  it('caps the popover frame and shrinks the column into it', () => {
    render(
      <Popover open>
        <PopoverTrigger>Open</PopoverTrigger>
        <CommandPopoverContent data-testid="frame">
          <Command data-testid="command">
            <CommandInput placeholder="Search..." />
            <CommandList data-testid="list">
              <CommandGroup>
                <CommandItem>Item one</CommandItem>
              </CommandGroup>
            </CommandList>
          </Command>
        </CommandPopoverContent>
      </Popover>
    )
    const frame = screen.getByTestId('frame')
    // The cap is on the frame because that is the box Radix measures the room
    // from: set inside it, the frame's own borders paint past the edge it was
    // measured from. Spelled out rather than imported from the component, so a
    // typo in the variable name fails here instead of matching itself.
    expect(frame).toHaveClass(BOUND_CLASS)
    expect(screen.getByTestId('command')).not.toHaveClass(BOUND_CLASS)
    expect(screen.getByTestId('list')).not.toHaveClass(BOUND_CLASS)
    // Flex is what carries the cap inward: a block frame would overflow it.
    expect(frame).toHaveClass('flex')
    expect(frame).toHaveClass('flex-col')
  })

  it('floors the column at the input row, not the list', () => {
    renderCommand()
    const column = screen.getByTestId('command')
    const list = screen.getByTestId('list')

    // The floor is the input row's own height token, so the two scale together
    // and the column can never clip the field being typed into.
    expect(column).toHaveClass('min-h-11')
    expect(screen.getByPlaceholderText('Search...')).toHaveClass('h-11')
    // `overflow-hidden` is what zeroes the column's automatic minimum size, so
    // it shrinks into the frame's cap instead of forcing the frame open.
    expect(column).toHaveClass('overflow-hidden')
    // The list keeps its own cap, so a roomy popover is unchanged, and scrolls
    // rather than overflowing once the frame is bounded.
    expect(list).toHaveClass('max-h-[300px]')
    expect(list).toHaveClass('overflow-y-auto')
  })

  it('lets a consumer replace either bound with its own', () => {
    // tailwind-merge keeps the last value in each group, so opting out of the
    // cap or the floor is silent.
    render(
      <Popover open>
        <PopoverTrigger>Open</PopoverTrigger>
        <CommandPopoverContent
          data-testid="frame"
          className="max-h-[400px]"
        >
          <Command data-testid="command" className="min-h-0" />
        </CommandPopoverContent>
      </Popover>
    )
    const frame = screen.getByTestId('frame')
    expect(frame).toHaveClass('max-h-[400px]')
    expect(frame).not.toHaveClass(BOUND_CLASS)
    const column = screen.getByTestId('command')
    expect(column).toHaveClass('min-h-0')
    expect(column).not.toHaveClass('min-h-11')
  })

  it('gives the dialog path the same column, with no cap of its own', () => {
    render(
      <CommandDialog open onOpenChange={() => {}}>
        <CommandInput placeholder="Search..." />
        <CommandList data-testid="list">
          <CommandGroup>
            <CommandItem>Item one</CommandItem>
          </CommandGroup>
        </CommandList>
      </CommandDialog>
    )
    // One column, two frames: the dialog frame publishes no available height,
    // so `CommandList`'s own 300px cap is what sizes this surface.
    const column = screen.getByTestId('list').closest('[cmdk-root]')
    expect(column).not.toHaveClass(BOUND_CLASS)
    expect(column).toHaveClass('min-h-11')
    expect(screen.getByTestId('list')).toHaveClass('max-h-[300px]')
  })
})
