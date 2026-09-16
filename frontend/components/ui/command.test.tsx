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
  POPOVER_AVAILABLE_HEIGHT_BOUND,
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
 * contract the geometry rests on: which element carries the bound, that the
 * variable reaches the list by inheritance when an ancestor sets it, and that
 * nothing sets it on the dialog path.
 */
describe('Command bounded by the popover available height', () => {
  function renderInPopoverLike(availableHeight: string | null) {
    return render(
      <div
        data-testid="content"
        style={
          availableHeight === null
            ? undefined
            : ({ [AVAILABLE_HEIGHT_VAR]: availableHeight } as React.CSSProperties)
        }
      >
        <Command data-testid="command">
          <CommandInput placeholder="Search..." />
          <CommandList data-testid="list">
            <CommandGroup>
              <CommandItem>Item one</CommandItem>
            </CommandGroup>
          </CommandList>
        </Command>
      </div>
    )
  }

  it('bounds the command column, not the list, by the available height', () => {
    renderInPopoverLike('120px')
    expect(screen.getByTestId('command')).toHaveClass(
      POPOVER_AVAILABLE_HEIGHT_BOUND
    )
    // The list keeps its own cap, so a roomy popover is unchanged.
    expect(screen.getByTestId('list')).toHaveClass('max-h-[300px]')
    expect(screen.getByTestId('list')).not.toHaveClass(
      POPOVER_AVAILABLE_HEIGHT_BOUND
    )
  })

  it('lets the list shrink and the input row hold its height', () => {
    renderInPopoverLike('120px')
    expect(screen.getByTestId('list')).toHaveClass('min-h-0')
    expect(screen.getByTestId('list')).toHaveClass('overflow-y-auto')
    expect(
      screen.getByPlaceholderText('Search...').closest('[cmdk-input-wrapper]')
    ).toHaveClass('shrink-0')
  })

  it('reads the variable from the content element when a popover sets it', () => {
    renderInPopoverLike('120px')
    expect(
      screen
        .getByTestId('content')
        .style.getPropertyValue(AVAILABLE_HEIGHT_VAR)
    ).toBe('120px')
  })

  it('leaves the variable unset on the dialog path, so the 300px cap applies', () => {
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
    const list = screen.getByTestId('list')
    expect(list).toHaveClass('max-h-[300px]')
    for (
      let node: HTMLElement | null = list;
      node !== null;
      node = node.parentElement
    ) {
      expect(node.style.getPropertyValue(AVAILABLE_HEIGHT_VAR)).toBe('')
    }
  })
})
