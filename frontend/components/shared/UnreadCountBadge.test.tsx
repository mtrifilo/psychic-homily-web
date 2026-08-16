import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { UnreadCountBadge, withUnreadLabel } from './UnreadCountBadge'

describe('UnreadCountBadge', () => {
  it('renders the count', () => {
    render(<UnreadCountBadge count={3} />)
    expect(screen.getByTestId('unread-count-badge')).toHaveTextContent('3')
  })

  // Every consumer renders this unconditionally and lets the count decide, so
  // the zero guard lives here rather than at four call sites.
  it.each([0, -1])('renders nothing at count %i', count => {
    render(<UnreadCountBadge count={count} />)
    expect(screen.queryByTestId('unread-count-badge')).not.toBeInTheDocument()
  })

  // The count is announced through the host control's accessible name
  // (withUnreadLabel), so the badge itself must not double-announce it.
  it('is decorative — hidden from assistive tech', () => {
    render(<UnreadCountBadge count={3} />)
    expect(screen.getByTestId('unread-count-badge')).toHaveAttribute(
      'aria-hidden'
    )
  })

  it('takes host placement without dropping its own styling', () => {
    render(<UnreadCountBadge count={1} className="absolute right-0" />)
    const badge = screen.getByTestId('unread-count-badge')
    expect(badge).toHaveClass('absolute', 'right-0', 'bg-primary')
  })
})

describe('withUnreadLabel', () => {
  it('appends the count when there is something unread', () => {
    expect(withUnreadLabel('Account', 3)).toBe('Account (3 unread)')
  })

  // Load-bearing: name-based queries that predate the badge (tests, e2e
  // locators, "Account" exact-text) must keep matching at zero.
  it.each([0, -1])('returns the bare label at count %i', count => {
    expect(withUnreadLabel('Account', count)).toBe('Account')
  })
})
