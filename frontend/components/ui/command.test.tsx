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
} from './command'

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
 * placement contract that geometry rests on: which element carries the bound
 * and its floor, and that a consumer can replace both.
 */
describe('Command bounded by the popover available height', () => {
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

  it('bounds the command column, floored at the input row, not the list', () => {
    renderCommand()
    const column = screen.getByTestId('command')
    const list = screen.getByTestId('list')

    // Spelled out rather than imported from the component, so a typo in the
    // variable name fails here instead of matching itself.
    expect(column).toHaveClass(BOUND_CLASS)
    expect(list).not.toHaveClass(BOUND_CLASS)
    // The floor is the input row's own height token, so the two scale together
    // and the column can never clip the field being typed into.
    expect(column).toHaveClass('min-h-11')
    expect(screen.getByPlaceholderText('Search...')).toHaveClass('h-11')
    // The list keeps its own cap, so a roomy popover is unchanged, and scrolls
    // rather than overflowing once the column is bounded.
    expect(list).toHaveClass('max-h-[300px]')
    expect(list).toHaveClass('overflow-y-auto')
  })

  it('lets a consumer replace the bound with its own max-height', () => {
    renderCommand('max-h-[400px]')
    // tailwind-merge keeps the last max-height, so opting out is silent. The
    // Cmd+K palette relies on the same precedence for its own list.
    const column = screen.getByTestId('command')
    expect(column).toHaveClass('max-h-[400px]')
    expect(column).not.toHaveClass(BOUND_CLASS)
  })

  it('gives the dialog path the same column, with no bound of its own', () => {
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
    // One code path, not two: the dialog carries the same class, and whether
    // the variable is set is what decides. Radix sets it on popover content
    // only, so here `max-height` resolves to `none`. That resolution is a
    // browser fact; jsdom evaluates no custom properties.
    expect(screen.getByTestId('list').closest('[cmdk-root]')).toHaveClass(
      BOUND_CLASS
    )
  })
})
