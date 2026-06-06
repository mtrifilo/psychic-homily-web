import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { Badge, badgeVariants } from './badge'

describe('Badge', () => {
  it('renders its children without throwing', () => {
    render(<Badge>New</Badge>)
    expect(screen.getByText('New')).toBeInTheDocument()
  })

  it('merges a custom className', () => {
    render(<Badge className="custom-class">New</Badge>)
    expect(screen.getByText('New')).toHaveClass('custom-class')
  })

  it('applies the default variant classes', () => {
    render(<Badge>Default</Badge>)
    expect(screen.getByText('Default')).toHaveClass('bg-primary')
  })

  it('applies the destructive variant classes', () => {
    render(<Badge variant="destructive">Danger</Badge>)
    expect(screen.getByText('Danger')).toHaveClass('bg-destructive')
  })

  it('applies the accent variant classes', () => {
    render(<Badge variant="accent">Accent</Badge>)
    const badge = screen.getByText('Accent')
    expect(badge).toHaveClass('bg-primary/10')
    expect(badge).toHaveClass('text-primary')
    expect(badge).toHaveClass('border-primary/20')
  })

  it('exposes a badgeVariants helper that reflects the variant', () => {
    expect(badgeVariants({ variant: 'secondary' })).toContain('bg-secondary')
    expect(badgeVariants({ variant: 'outline' })).toContain('text-foreground')
  })
})
