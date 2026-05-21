import { describe, it, expect } from 'vitest'
import { createRef } from 'react'
import { render, screen } from '@testing-library/react'
import { Button, buttonVariants } from './button'

describe('Button', () => {
  it('renders its children without throwing', () => {
    render(<Button>Click me</Button>)
    expect(screen.getByRole('button', { name: 'Click me' })).toBeInTheDocument()
  })

  it('merges a custom className', () => {
    render(<Button className="custom-class">Go</Button>)
    expect(screen.getByRole('button')).toHaveClass('custom-class')
  })

  it('forwards a ref to the underlying button element', () => {
    const ref = createRef<HTMLButtonElement>()
    render(<Button ref={ref}>Ref</Button>)
    expect(ref.current).toBeInstanceOf(HTMLButtonElement)
  })

  it('applies variant and size classes', () => {
    render(
      <Button variant="destructive" size="sm">
        Delete
      </Button>
    )
    const button = screen.getByRole('button')
    expect(button).toHaveClass('bg-destructive') // variant=destructive
    expect(button).toHaveClass('h-8') // size=sm
  })

  it('honors the disabled prop', () => {
    render(<Button disabled>Disabled</Button>)
    expect(screen.getByRole('button')).toBeDisabled()
  })

  it('renders as a child element when asChild is set', () => {
    render(
      <Button asChild>
        <a href="/somewhere">Link button</a>
      </Button>
    )
    const link = screen.getByRole('link', { name: 'Link button' })
    expect(link).toHaveAttribute('href', '/somewhere')
    // The asChild Slot still applies button variant classes.
    expect(link).toHaveClass('inline-flex')
  })

  it('exposes a buttonVariants helper that reflects variant/size', () => {
    const cls = buttonVariants({ variant: 'outline', size: 'lg' })
    expect(cls).toContain('border')
    expect(cls).toContain('h-10')
  })
})
