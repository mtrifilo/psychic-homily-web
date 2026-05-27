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

  // Per-branch a11y coverage (PSY-862). The button + link branches build
  // independent accessible-name attributes, so each branch needs its own
  // explicit assertion.

  describe('link branch a11y', () => {
    it('uses label as accessible name on the link branch by default', () => {
      render(<BracketLink label="View history" href="/x/history" />)
      // `getByRole('link', { name })` resolves via accessible name —
      // tests that the `aria-label={ariaLabel ?? label}` default fires
      // for the link path, not just the button path.
      expect(
        screen.getByRole('link', { name: 'View history' })
      ).toBeInTheDocument()
    })

    it('uses ariaLabel override on the link branch', () => {
      render(
        <BracketLink
          label="History"
          ariaLabel="Open revision history"
          href="/x/history"
        />
      )
      expect(
        screen.getByRole('link', { name: 'Open revision history' })
      ).toBeInTheDocument()
    })

    it('forwards title to the link branch', () => {
      render(
        <BracketLink label="History" href="/x/history" title="See full history" />
      )
      expect(screen.getByRole('link')).toHaveAttribute('title', 'See full history')
    })

    it('passes href through verbatim on the link branch', () => {
      render(
        <BracketLink
          label="Filter by tag"
          href="/shows?tag=post-punk&year=2024"
        />
      )
      expect(
        screen.getByRole('link', { name: 'Filter by tag' })
      ).toHaveAttribute('href', '/shows?tag=post-punk&year=2024')
    })
  })

  describe('href + disabled fallback (anchors have no native disabled state)', () => {
    it('renders a disabled button (not a link) when href AND disabled are both set', () => {
      // Anchors cannot be natively disabled. The component falls back to
      // a `<button disabled>` to prevent click-through and keep AT users
      // from bypassing the disabled state.
      render(<BracketLink label="Follow" href="/somewhere" disabled />)
      expect(screen.queryByRole('link')).not.toBeInTheDocument()
      const button = screen.getByRole('button', { name: 'Follow' })
      expect(button).toBeDisabled()
    })
  })
})
