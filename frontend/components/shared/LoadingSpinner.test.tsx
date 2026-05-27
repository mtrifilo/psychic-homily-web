import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { LoadingSpinner } from './LoadingSpinner'

describe('LoadingSpinner', () => {
  it('exposes role="status" with a default accessible name', () => {
    render(<LoadingSpinner />)
    // role="status" + aria-label makes the spinner announceable to assistive
    // tech. Querying by role is the authoritative shape; sizing/class assertions
    // below cover the visual contract separately.
    const spinner = screen.getByRole('status', { name: 'Loading' })
    expect(spinner).toBeInTheDocument()
    expect(spinner.tagName).toBe('DIV')
    expect(spinner.className).toContain('animate-spin')
    expect(spinner.className).toContain('rounded-full')
    expect(spinner.className).toContain('border-b-2')
  })

  it('accepts a custom accessible label', () => {
    render(<LoadingSpinner label="Loading collections" />)
    expect(
      screen.getByRole('status', { name: 'Loading collections' })
    ).toBeInTheDocument()
  })

  it('defaults to medium size', () => {
    render(<LoadingSpinner />)
    const spinner = screen.getByRole('status')
    expect(spinner.className).toContain('h-8')
    expect(spinner.className).toContain('w-8')
  })

  it('renders small size', () => {
    render(<LoadingSpinner size="sm" />)
    const spinner = screen.getByRole('status')
    expect(spinner.className).toContain('h-4')
    expect(spinner.className).toContain('w-4')
  })

  it('renders large size', () => {
    render(<LoadingSpinner size="lg" />)
    const spinner = screen.getByRole('status')
    expect(spinner.className).toContain('h-12')
    expect(spinner.className).toContain('w-12')
  })

  it('applies custom className', () => {
    render(<LoadingSpinner className="mt-4" />)
    const spinner = screen.getByRole('status')
    expect(spinner.className).toContain('mt-4')
  })

  it('merges custom className with default classes', () => {
    render(<LoadingSpinner size="sm" className="text-red-500" />)
    const spinner = screen.getByRole('status')
    expect(spinner.className).toContain('animate-spin')
    expect(spinner.className).toContain('text-red-500')
    expect(spinner.className).toContain('h-4')
  })
})
