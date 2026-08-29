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

  describe('pointer cursor', () => {
    // Tailwind preflight resets <button> to `cursor: default`, so the button
    // branch needs an explicit cursor-pointer to read as interactive.
    it('applies cursor-pointer on the button branch', () => {
      render(<BracketLink label="Follow" onClick={vi.fn()} />)
      expect(screen.getByRole('button').className).toContain('cursor-pointer')
    })

    it('applies cursor-pointer on the link branch', () => {
      render(<BracketLink label="View history" href="/artists/x/history" />)
      expect(screen.getByRole('link').className).toContain('cursor-pointer')
    })

    it('lets cursor-not-allowed win over cursor-pointer when disabled', () => {
      render(<BracketLink label="Follow" onClick={vi.fn()} disabled />)
      const className = screen.getByRole('button').className
      expect(className).toContain('cursor-not-allowed')
      expect(className).not.toContain('cursor-pointer')
    })
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

  describe('external links', () => {
    it('opens an external href in a new tab with rel hygiene', () => {
      render(
        <BracketLink
          label="Directions ↗"
          href="https://www.google.com/maps/search/?api=1&query=x"
          external
        />
      )
      const link = screen.getByRole('link', {
        name: 'Directions ↗ (opens in a new tab)',
      })
      expect(link).toHaveAttribute('target', '_blank')
      expect(link).toHaveAttribute('rel', 'noopener noreferrer')
      expect(link).toHaveAttribute(
        'href',
        'https://www.google.com/maps/search/?api=1&query=x'
      )
    })

    // The announcement belongs to the component so that no call site can write
    // it, forget it, or let it drift from the target it describes.
    it('appends the new-tab announcement to a caller ariaLabel rather than replacing it', () => {
      render(
        <BracketLink
          label="site ↗"
          href="https://crescentphx.test"
          external
          ariaLabel="Crescent Ballroom website"
        />
      )
      expect(
        screen.getByRole('link', {
          name: 'Crescent Ballroom website (opens in a new tab)',
        })
      ).toBeInTheDocument()
    })

    // Tolerance, NOT endorsement: `external`'s contract says callers must not
    // write this phrase. These pin that doing it anyway degrades to a correct
    // name rather than a stutter, across the phrasings people actually reach
    // for. The middle case is the exact string this codebase carried before
    // the announcement moved into the component; a literal check for the
    // canonical wording would miss it.
    //
    // Passing no ariaLabel here would make the count assertion arithmetically
    // true and pin nothing, which is what the earlier version of this test did.
    it.each([
      ['Buy tickets (opens in a new tab)'],
      ['Directions to Salt Shed (opens Google Maps in a new tab)'],
      ['Listen live (opens in a new window)'],
    ])('does not double a hand-written announcement: %s', ariaLabel => {
      render(
        <BracketLink label="Go ↗" href="https://x.test" external ariaLabel={ariaLabel} />
      )
      const name = screen.getByRole('link').getAttribute('aria-label') as string
      expect(name).toBe(ariaLabel)
      expect(name.match(/new (tab|window)/g)).toHaveLength(1)
    })

    // Anchored at the END on purpose: the name can carry operator-entered text
    // (a venue or station name), and stored content must not be able to
    // suppress the announcement for the whole control.
    it('still announces when the phrase appears mid-name rather than as a trailing note', () => {
      render(
        <BracketLink
          label="Go ↗"
          href="https://x.test"
          external
          ariaLabel="The (opens in a new tab) Lounge"
        />
      )
      expect(
        screen.getByRole('link', {
          name: 'The (opens in a new tab) Lounge (opens in a new tab)',
        })
      ).toBeInTheDocument()
    })

    // Asserted on all three render branches, not just this one: the button
    // branch carries the destructive [X] / [Remove] controls, where an unnamed
    // control is worst.
    it('falls back to the visible label when ariaLabel is blank', () => {
      render(
        <BracketLink
          label="Buy Tickets ↗"
          href="https://tix.test"
          external
          ariaLabel="   "
        />
      )
      expect(
        screen.getByRole('link', { name: 'Buy Tickets ↗ (opens in a new tab)' })
      ).toBeInTheDocument()
    })

    it('falls back to the visible label when ariaLabel is blank on an internal link', () => {
      render(<BracketLink label="History" href="/history" ariaLabel="  " />)
      expect(screen.getByRole('link', { name: 'History' })).toBeInTheDocument()
    })

    it('falls back to the visible label when ariaLabel is blank on the button branch', () => {
      render(<BracketLink label="X" variant="danger" ariaLabel="" onClick={() => {}} />)
      expect(screen.getByRole('button', { name: 'X' })).toBeInTheDocument()
    })

    it('leaves internal links in the same tab, with no new-tab announcement', () => {
      render(<BracketLink label="History" href="/history" />)
      const link = screen.getByRole('link', { name: 'History' })
      expect(link).not.toHaveAttribute('target')
      expect(link.getAttribute('aria-label')).not.toMatch(/new tab/)
    })

    // The disabled fallback opens nothing, so announcing a new tab there would
    // be the same mismatch between claim and behavior this design removes.
    it('still falls back to a disabled button when disabled, without announcing a new tab', () => {
      render(
        <BracketLink label="Directions ↗" href="https://maps.example" external disabled />
      )
      expect(screen.queryByRole('link')).not.toBeInTheDocument()
      const button = screen.getByRole('button', { name: 'Directions ↗' })
      expect(button).toBeDisabled()
      expect(button.getAttribute('aria-label')).not.toMatch(/new tab/)
    })

    // The scheme floor lives in the primitive: `external` is exactly where a
    // user-controlled URL eventually arrives, and a non-http value must
    // never render as an anchor — disabled fallback, same as href+disabled.
    it.each([
      ['javascript:alert(1)'],
      ['JaVaScRiPt:alert(1)'],
      ['data:text/html,x'],
      ['ftp://tix.example'],
      // Protocol-relative: inherits the page scheme, so it IS off-site.
      ['//evil.example/x'],
      // Leading whitespace/control chars, which browsers strip from an href
      // before navigating. The check runs on the raw value, so these stay
      // rejected; that is the safe direction.
      [' javascript:alert(1)'],
      ['\njavascript:alert(1)'],
      ['\tjavascript:alert(1)'],
      // Scheme-less: not a live relative link either.
      ['tix.example/buy'],
    ])('renders a disabled button, never an anchor, for %s', href => {
      render(<BracketLink label="Buy ↗" href={href} external />)
      expect(screen.queryByRole('link')).not.toBeInTheDocument()
      expect(screen.getByRole('button', { name: 'Buy ↗' })).toBeDisabled()
    })

    // The write path persists raw operator/contributor paste while validating
    // a trimmed copy, so padded URLs reach this component. A browser strips
    // that whitespace when parsing an href, so rejecting it here would kill a
    // link the platform considers valid.
    it.each([
      ['  https://tix.example'],
      ['https://tix.example  '],
      ['\n https://tix.example \t'],
    ])('renders a live, trimmed anchor for a padded url (%j)', href => {
      render(<BracketLink label="Buy ↗" href={href} external />)
      const link = screen.getByRole('link', { name: /^Buy ↗/ })
      expect(link).toHaveAttribute('href', 'https://tix.example')
      expect(link).toHaveAttribute('target', '_blank')
      expect(link).toHaveAttribute('rel', 'noopener noreferrer')
    })
  })

  describe('pre-hydration click replay (PSY-1615)', () => {
    // BracketLink owns replay for ~71 bracket controls, so none of them declare
    // it themselves. Without these assertions, deleting the `{...replayOnHydrate}`
    // spread would type-check, render identically, pass every other test, and
    // silently drop clicks on every bracket control in the app.
    it('marks the button branch as a replay root', () => {
      render(<BracketLink label="Save" onClick={() => {}} />)
      expect(screen.getByRole('button', { name: 'Save' })).toHaveAttribute(
        'data-replay-on-hydrate'
      )
    })

    it('does NOT mark the link branch — a real anchor already survives the window', () => {
      render(<BracketLink label="History" href="/history" />)
      expect(screen.getByRole('link', { name: 'History' })).not.toHaveAttribute(
        'data-replay-on-hydrate'
      )
    })

    it('still forwards the caller ref that Radix asChild triggers depend on', () => {
      // The replay ref is composed with the forwarded one; neither may be lost.
      const ref = { current: null as HTMLButtonElement | null }
      render(<BracketLink ref={ref} label="Open" onClick={() => {}} />)
      expect(ref.current).toBeInstanceOf(HTMLButtonElement)
      expect(ref.current).toHaveAttribute('data-replay-on-hydrate')
    })
  })
})
