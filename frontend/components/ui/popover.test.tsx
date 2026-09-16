import { describe, it, expect } from 'vitest'
import { createRef } from 'react'
import { render, screen } from '@testing-library/react'
import { Popover, PopoverTrigger, PopoverContent } from './popover'

describe('Popover', () => {
  it('renders the trigger without throwing', () => {
    render(
      <Popover>
        <PopoverTrigger>Open</PopoverTrigger>
        <PopoverContent>Panel</PopoverContent>
      </Popover>
    )
    expect(screen.getByRole('button', { name: 'Open' })).toBeInTheDocument()
  })

  it('renders portal content when open', () => {
    render(
      <Popover open>
        <PopoverTrigger>Open</PopoverTrigger>
        <PopoverContent>Panel content</PopoverContent>
      </Popover>
    )
    expect(screen.getByText('Panel content')).toBeInTheDocument()
  })

  it('merges a custom className on the content', () => {
    render(
      <Popover open>
        <PopoverTrigger>Open</PopoverTrigger>
        <PopoverContent className="custom-class">Panel content</PopoverContent>
      </Popover>
    )
    expect(screen.getByText('Panel content')).toHaveClass('custom-class')
  })

  it('forwards a ref to the content', () => {
    const ref = createRef<HTMLDivElement>()
    render(
      <Popover open>
        <PopoverTrigger>Open</PopoverTrigger>
        <PopoverContent ref={ref}>Panel content</PopoverContent>
      </Popover>
    )
    expect(ref.current).toBeInstanceOf(HTMLDivElement)
  })
})

/**
 * jsdom has no layout and evaluates no `:has()`, so what these classes do is
 * asserted in `e2e/pages/command-popover-keyboard.spec.ts`. What is checkable
 * here is where they sit: on the border box Radix positions, which is the box
 * `--radix-popover-content-available-height` is measured from.
 */
describe('PopoverContent framing a command column', () => {
  // Spelled out rather than imported from the component, so a typo in the
  // variable name or the scope fails here instead of matching itself.
  const SCOPED_BOUND = [
    'has-[[cmdk-root]]:max-h-(--radix-popover-content-available-height)',
    'has-[[cmdk-root]]:flex',
    'has-[[cmdk-root]]:flex-col',
  ]

  it('caps its own border box and carries the cap inward with flex', () => {
    render(
      <Popover open>
        <PopoverTrigger>Open</PopoverTrigger>
        <PopoverContent>Panel content</PopoverContent>
      </Popover>
    )
    const content = screen.getByText('Panel content')
    for (const className of SCOPED_BOUND) {
      expect(content).toHaveClass(className)
    }
    // Never unscoped: a bare cap would bound every popover on the site to the
    // room on screen, and a bare `flex` would relayout every popover's body.
    expect(content).not.toHaveClass(
      'max-h-(--radix-popover-content-available-height)'
    )
    expect(content).not.toHaveClass('flex')
  })
})
