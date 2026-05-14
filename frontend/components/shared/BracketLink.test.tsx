import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { BracketLink } from './BracketLink'

describe('BracketLink', () => {
  it('renders the label wrapped in literal brackets', () => {
    render(<BracketLink label="Follow" />)
    expect(screen.getByText('Follow')).toBeInTheDocument()
    expect(screen.getByText('[')).toBeInTheDocument()
    expect(screen.getByText(']')).toBeInTheDocument()
  })

  it('renders as a button when no href is provided', () => {
    render(<BracketLink label="Follow" />)
    const button = screen.getByRole('button', { name: 'Follow' })
    expect(button).toBeInTheDocument()
    expect(button).toHaveAttribute('type', 'button')
  })

  it('renders as a link when href is provided', () => {
    render(<BracketLink label="View history" href="/artists/x/history" />)
    const link = screen.getByRole('link', { name: 'View history' })
    expect(link).toBeInTheDocument()
    expect(link).toHaveAttribute('href', '/artists/x/history')
  })

  it('calls onClick when button is clicked', async () => {
    const user = userEvent.setup()
    const onClick = vi.fn()
    render(<BracketLink label="Follow" onClick={onClick} />)
    await user.click(screen.getByRole('button'))
    expect(onClick).toHaveBeenCalledOnce()
  })

  it('marks itself aria-pressed when active', () => {
    render(<BracketLink label="Following" active />)
    expect(screen.getByRole('button')).toHaveAttribute('aria-pressed', 'true')
  })

  it('does not set aria-pressed when inactive', () => {
    render(<BracketLink label="Follow" />)
    expect(screen.getByRole('button')).not.toHaveAttribute('aria-pressed')
  })

  it('applies danger styling for danger variant', () => {
    render(<BracketLink label="X" variant="danger" onClick={vi.fn()} />)
    const button = screen.getByRole('button')
    expect(button.className).toContain('text-destructive')
  })

  it('is disabled when disabled prop is set', () => {
    const onClick = vi.fn()
    render(<BracketLink label="Follow" onClick={onClick} disabled />)
    expect(screen.getByRole('button')).toBeDisabled()
  })

  it('does not fire onClick when disabled', async () => {
    const user = userEvent.setup()
    const onClick = vi.fn()
    render(<BracketLink label="Follow" onClick={onClick} disabled />)
    await user.click(screen.getByRole('button'))
    expect(onClick).not.toHaveBeenCalled()
  })

  it('falls back to button when href is provided AND disabled', () => {
    render(<BracketLink label="Follow" href="/somewhere" disabled />)
    expect(screen.queryByRole('link')).not.toBeInTheDocument()
    expect(screen.getByRole('button')).toBeInTheDocument()
  })

  it('uses ariaLabel override when provided', () => {
    render(<BracketLink label="X" ariaLabel="Remove tag" onClick={vi.fn()} />)
    expect(screen.getByRole('button', { name: 'Remove tag' })).toBeInTheDocument()
  })

  it('passes title to underlying element', () => {
    render(<BracketLink label="X" title="Remove" onClick={vi.fn()} />)
    expect(screen.getByRole('button')).toHaveAttribute('title', 'Remove')
  })

  it('applies custom className', () => {
    render(<BracketLink label="Follow" className="ml-4" />)
    expect(screen.getByRole('button').className).toContain('ml-4')
  })

  it('marks brackets as aria-hidden so screen readers read only the label', () => {
    render(<BracketLink label="Follow" />)
    const openBracket = screen.getByText('[')
    const closeBracket = screen.getByText(']')
    expect(openBracket).toHaveAttribute('aria-hidden', 'true')
    expect(closeBracket).toHaveAttribute('aria-hidden', 'true')
  })
})
