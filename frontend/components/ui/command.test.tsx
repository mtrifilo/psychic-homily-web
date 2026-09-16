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

/** The variable Radix publishes on popover content, and only there. */
const AVAILABLE_HEIGHT_VAR = '--radix-popover-content-available-height'

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
 * placement contract that geometry rests on: which element carries the bound,
 * which element is free to give way, and that the dialog path carries no bound
 * at all.
 */
describe('Command bounded by the popover available height', () => {
  it('bounds the command column and leaves the list free to shrink', () => {
    render(
      <Command data-testid="command">
        <CommandInput placeholder="Search..." />
        <CommandList data-testid="list">
          <CommandGroup>
            <CommandItem>Item one</CommandItem>
          </CommandGroup>
        </CommandList>
      </Command>
    )
    const column = screen.getByTestId('command')
    const list = screen.getByTestId('list')

    // Spelled out rather than imported from the component, so a typo in the
    // variable name fails here instead of matching itself.
    expect(column).toHaveClass(`max-h-(${AVAILABLE_HEIGHT_VAR})`)
    expect(list).not.toHaveClass(`max-h-(${AVAILABLE_HEIGHT_VAR})`)
    // The list keeps its own cap, so a roomy popover is unchanged, and can
    // shrink below its content and scroll when the column is bounded.
    expect(list).toHaveClass('max-h-[300px]')
    expect(list).toHaveClass('min-h-0')
    expect(list).toHaveClass('overflow-y-auto')
    // The input row is pinned, so the list is what gives way.
    expect(
      screen.getByPlaceholderText('Search...').closest('[cmdk-input-wrapper]')
    ).toHaveClass('shrink-0')
  })

  it('leaves the dialog path unbounded, so the 300px cap applies there', () => {
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
    // The bound is inert without the variable, which only a popover sets, so
    // the list's own cap is the only ceiling on this path.
    expect(screen.getByTestId('list')).toHaveClass('max-h-[300px]')
  })
})
