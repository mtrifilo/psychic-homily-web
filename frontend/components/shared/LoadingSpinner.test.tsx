import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { LoadingSpinner } from './LoadingSpinner'

describe('LoadingSpinner', () => {
  it('renders a div element', () => {
    const { container } = render(<LoadingSpinner />)
    expect(container.firstChild).toBeInTheDocument()
  })

  it('defaults to medium size', () => {
    const { container } = render(<LoadingSpinner />)
    const spinner = container.firstChild as HTMLElement
    expect(spinner.className).toContain('h-8')
    expect(spinner.className).toContain('w-8')
  })

  it('renders small size', () => {
    const { container } = render(<LoadingSpinner size="sm" />)
    const spinner = container.firstChild as HTMLElement
    expect(spinner.className).toContain('h-4')
    expect(spinner.className).toContain('w-4')
  })

  it('renders large size', () => {
    const { container } = render(<LoadingSpinner size="lg" />)
    const spinner = container.firstChild as HTMLElement
    expect(spinner.className).toContain('h-12')
    expect(spinner.className).toContain('w-12')
  })

  it('has animate-spin class for animation', () => {
    const { container } = render(<LoadingSpinner />)
    const spinner = container.firstChild as HTMLElement
    expect(spinner.className).toContain('animate-spin')
  })

  it('applies custom className', () => {
    const { container } = render(<LoadingSpinner className="mt-4" />)
    const spinner = container.firstChild as HTMLElement
    expect(spinner.className).toContain('mt-4')
  })

  it('merges custom className with default classes', () => {
    const { container } = render(<LoadingSpinner size="sm" className="text-red-500" />)
    const spinner = container.firstChild as HTMLElement
    expect(spinner.className).toContain('animate-spin')
    expect(spinner.className).toContain('text-red-500')
    expect(spinner.className).toContain('h-4')
  })
})
