import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { TagOfficialIndicator } from './TagOfficialIndicator'

describe('TagOfficialIndicator', () => {
  it('renders the BadgeCheck icon at size="sm" without the "Official" text', () => {
    render(<TagOfficialIndicator size="sm" />)

    const marker = screen.getByRole('img', { name: 'Official tag' })
    expect(marker).toBeInTheDocument()

    const icon = marker.querySelector('.lucide-badge-check')
    expect(icon).not.toBeNull()

    expect(screen.queryByText('Official')).not.toBeInTheDocument()
  })

  it('renders the BadgeCheck icon and "Official" text at size="md"', () => {
    render(<TagOfficialIndicator size="md" />)

    const marker = screen.getByRole('img', { name: 'Official tag' })
    expect(marker).toBeInTheDocument()

    const icon = marker.querySelector('.lucide-badge-check')
    expect(icon).not.toBeNull()

    expect(screen.getByText('Official')).toBeInTheDocument()
  })

  it('defaults to size="sm" when size is not provided', () => {
    render(<TagOfficialIndicator />)

    expect(screen.getByRole('img', { name: 'Official tag' })).toBeInTheDocument()
    expect(screen.queryByText('Official')).not.toBeInTheDocument()
  })

  it('uses a tag-specific tooltip when tagName is provided', () => {
    render(<TagOfficialIndicator tagName="shoegaze" />)

    const marker = screen.getByRole('img', { name: 'Official tag' })
    expect(marker).toHaveAttribute('title', 'shoegaze (Official)')
  })

  it('falls back to a generic tooltip without tagName', () => {
    render(<TagOfficialIndicator />)

    const marker = screen.getByRole('img', { name: 'Official tag' })
    expect(marker).toHaveAttribute('title', 'Official tag')
  })

  it('preserves aria-label="Official tag" at both sizes', () => {
    const { rerender } = render(<TagOfficialIndicator size="sm" />)
    expect(screen.getByRole('img', { name: 'Official tag' })).toHaveAttribute(
      'aria-label',
      'Official tag'
    )

    rerender(<TagOfficialIndicator size="md" />)
    expect(screen.getByRole('img', { name: 'Official tag' })).toHaveAttribute(
      'aria-label',
      'Official tag'
    )
  })

  it('applies additional className while preserving base classes', () => {
    render(<TagOfficialIndicator className="ml-2" />)

    const marker = screen.getByRole('img', { name: 'Official tag' })
    expect(marker.className).toContain('ml-2')
    expect(marker.className).toContain('text-primary')
  })
})
