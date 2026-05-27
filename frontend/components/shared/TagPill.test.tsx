import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { TagPill } from './TagPill'

// The pill renders either as a <Link> (when `href` is provided) or a
// <button type="button">. The two branches must be functionally exclusive:
// providing `href` should NEVER emit a button, providing onClick + no href
// should NEVER emit an anchor. Asserting both directions catches a refactor
// that picks the wrong branch for the wrong props.

describe('TagPill', () => {
  describe('label rendering', () => {
    it('renders label text', () => {
      render(<TagPill label="post-punk" />)
      expect(screen.getByText('post-punk')).toBeInTheDocument()
    })

    it('renders label as the accessible name on the button branch', () => {
      render(<TagPill label="shoegaze" />)
      // The single observable contract that "the pill works as a clickable
      // affordance" is that its role + accessible name resolve.
      expect(
        screen.getByRole('button', { name: 'shoegaze' })
      ).toBeInTheDocument()
    })

    it('renders label as the accessible name on the link branch', () => {
      render(<TagPill label="shoegaze" href="/shows?tag=shoegaze" />)
      expect(
        screen.getByRole('link', { name: 'shoegaze' })
      ).toBeInTheDocument()
    })
  })

  describe('href branch (link)', () => {
    it('renders as a link when href is provided', () => {
      render(<TagPill label="shoegaze" href="/shows?tag=shoegaze" />)
      const link = screen.getByRole('link', { name: 'shoegaze' })
      expect(link).toHaveAttribute('href', '/shows?tag=shoegaze')
    })

    it('does NOT render a button when href is provided', () => {
      // The two branches are exclusive. If a future refactor renders both
      // a wrapping <a> AND a <button>, Playwright strict-mode lookups
      // (CLAUDE.md "One link per entity card") would break.
      render(<TagPill label="post-punk" href="/tag/post-punk" />)
      expect(screen.queryByRole('button')).not.toBeInTheDocument()
    })

    it('forwards a relative href as-is', () => {
      render(<TagPill label="ambient" href="/tag/ambient" />)
      expect(
        screen.getByRole('link', { name: 'ambient' })
      ).toHaveAttribute('href', '/tag/ambient')
    })

    it('forwards a query-string-shaped href as-is', () => {
      render(<TagPill label="noise" href="/shows?tag=noise&year=2024" />)
      expect(
        screen.getByRole('link', { name: 'noise' })
      ).toHaveAttribute('href', '/shows?tag=noise&year=2024')
    })
  })

  describe('button branch (onClick)', () => {
    it('renders as a button when no href is provided', () => {
      render(<TagPill label="shoegaze" />)
      expect(screen.getByRole('button')).toBeInTheDocument()
    })

    it('does NOT render a link when no href is provided', () => {
      render(<TagPill label="shoegaze" onClick={vi.fn()} />)
      expect(screen.queryByRole('link')).not.toBeInTheDocument()
    })

    it('button has type="button" attribute (form-submit guard)', () => {
      render(<TagPill label="ambient" />)
      // Default <button> in a form is type="submit"; this would
      // accidentally submit any wrapping form. Explicit type="button" is
      // load-bearing.
      expect(screen.getByRole('button')).toHaveAttribute('type', 'button')
    })

    it('calls onClick handler exactly once per click', async () => {
      const user = userEvent.setup()
      const onClick = vi.fn()
      render(<TagPill label="shoegaze" onClick={onClick} />)

      await user.click(screen.getByRole('button', { name: 'shoegaze' }))
      expect(onClick).toHaveBeenCalledOnce()
    })

    it('does not fire onClick when href is present (link branch ignores onClick)', async () => {
      const user = userEvent.setup()
      const onClick = vi.fn()
      render(
        <TagPill label="shoegaze" href="/shows?tag=shoegaze" onClick={onClick} />
      )

      await user.click(screen.getByRole('link', { name: 'shoegaze' }))
      // The link branch does not bind onClick at all (see source) — this
      // locks that behaviour in so a refactor that adds onClick to the
      // link path doesn't silently double-fire.
      expect(onClick).not.toHaveBeenCalled()
    })
  })

  describe('vote count', () => {
    it('displays positive vote count with + prefix', () => {
      render(<TagPill label="post-punk" voteCount={12} />)
      expect(screen.getByText('+12')).toBeInTheDocument()
    })

    it('displays zero vote count with + prefix', () => {
      render(<TagPill label="post-punk" voteCount={0} />)
      // 0 reads as "+0" rather than "0" or "-0" — the source treats
      // zero as a non-negative for prefix purposes.
      expect(screen.getByText('+0')).toBeInTheDocument()
    })

    it('displays negative vote count without + prefix (uses native -)', () => {
      render(<TagPill label="post-punk" voteCount={-3} />)
      expect(screen.getByText('-3')).toBeInTheDocument()
    })

    it('does not display vote count when undefined', () => {
      render(<TagPill label="post-punk" />)
      // Asserts the explicit unset state — undefined should suppress the
      // counter entirely (no "+undefined" or "+null").
      expect(screen.queryByText(/^[+-]/)).not.toBeInTheDocument()
    })

    it('renders both label and vote count together in a link', () => {
      render(
        <TagPill label="noise-rock" voteCount={7} href="/tag/noise-rock" />
      )
      const link = screen.getByRole('link')
      expect(link).toHaveTextContent('noise-rock')
      expect(link).toHaveTextContent('+7')
    })

    it('renders both label and vote count together in a button', () => {
      render(<TagPill label="noise-rock" voteCount={7} onClick={vi.fn()} />)
      const button = screen.getByRole('button')
      expect(button).toHaveTextContent('noise-rock')
      expect(button).toHaveTextContent('+7')
    })
  })

  describe('styling', () => {
    it('applies custom className to the button branch', () => {
      render(<TagPill label="jazz" className="mt-2" />)
      const button = screen.getByRole('button')
      expect(button.className).toContain('mt-2')
    })

    it('applies custom className to the link branch', () => {
      render(<TagPill label="jazz" href="/tag/jazz" className="mt-2" />)
      const link = screen.getByRole('link')
      expect(link.className).toContain('mt-2')
    })

    it('shares pill chrome classes between both branches', () => {
      // Both branches share the same `pillClasses` constant — the visual
      // shape must not diverge between Link and button. Picks a few
      // load-bearing classes from `cn(...)` to lock the shared chrome.
      const { rerender } = render(<TagPill label="x" />)
      const buttonClasses = screen.getByRole('button').className

      rerender(<TagPill label="x" href="/x" />)
      const linkClasses = screen.getByRole('link').className

      for (const cls of [
        'inline-flex',
        'rounded-md',
        'bg-muted',
        'text-muted-foreground',
        'border-border/50',
      ]) {
        expect(buttonClasses).toContain(cls)
        expect(linkClasses).toContain(cls)
      }
    })
  })
})
