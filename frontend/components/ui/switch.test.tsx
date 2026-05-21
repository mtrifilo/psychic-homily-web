import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { Switch } from './switch'

describe('Switch', () => {
  it('renders without throwing', () => {
    render(<Switch aria-label="notifications" />)
    expect(
      screen.getByRole('switch', { name: 'notifications' })
    ).toBeInTheDocument()
  })

  it('merges a custom className', () => {
    render(<Switch aria-label="notifications" className="custom-class" />)
    expect(screen.getByRole('switch')).toHaveClass('custom-class')
  })

  it('applies the default size data attribute', () => {
    render(<Switch aria-label="notifications" />)
    expect(screen.getByRole('switch')).toHaveAttribute('data-size', 'default')
  })

  it('applies the sm size data attribute', () => {
    render(<Switch aria-label="notifications" size="sm" />)
    expect(screen.getByRole('switch')).toHaveAttribute('data-size', 'sm')
  })

  it('reflects the checked state', () => {
    render(<Switch aria-label="notifications" defaultChecked />)
    expect(screen.getByRole('switch')).toBeChecked()
  })

  it('honors the disabled prop', () => {
    render(<Switch aria-label="notifications" disabled />)
    expect(screen.getByRole('switch')).toBeDisabled()
  })
})
