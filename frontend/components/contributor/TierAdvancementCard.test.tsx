import React from 'react'
import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { TierAdvancementCard } from './TierAdvancementCard'
import type { UserTier } from '@/features/auth'

vi.mock('next/link', () => ({
  default: ({
    href,
    children,
    ...props
  }: {
    href: string
    children: React.ReactNode
  }) => (
    <a href={href} {...props}>
      {children}
    </a>
  ),
}))

describe('TierAdvancementCard', () => {
  it('renders the board-H header: sentence-case title + tier badge', () => {
    render(<TierAdvancementCard tier="new_user" />)
    expect(screen.getByText('Contributor tier')).toBeInTheDocument()
    // The current-tier badge sits in the header row (the old "Your tier:"
    // label was dropped per board H).
    expect(screen.getByText('New User')).toBeInTheDocument()
    expect(screen.queryByText('Your tier:')).not.toBeInTheDocument()
  })

  it('renders the current tier badge for new_user with next-tier requirements', () => {
    render(<TierAdvancementCard tier="new_user" />)

    expect(screen.getByText('New User')).toBeInTheDocument()
    expect(screen.getByText('Contributor')).toBeInTheDocument()
    expect(screen.getByText('Requirements')).toBeInTheDocument()
    // Criteria sourced from backend auto_promotion.go
    expect(screen.getByText(/5 approved edits/i)).toBeInTheDocument()
    expect(screen.getByText(/14 days/i)).toBeInTheDocument()
    expect(screen.getByText(/Verified email/i)).toBeInTheDocument()
  })

  it('renders requirements for contributor advancing to trusted_contributor', () => {
    render(<TierAdvancementCard tier="contributor" />)

    expect(screen.getByText('Contributor')).toBeInTheDocument()
    expect(screen.getByText('Trusted Contributor')).toBeInTheDocument()
    expect(screen.getByText(/25 approved edits/i)).toBeInTheDocument()
    expect(screen.getByText(/95% approval rate/i)).toBeInTheDocument()
    expect(screen.getByText(/60 days/i)).toBeInTheDocument()
  })

  it('renders requirements for trusted_contributor advancing to local_ambassador', () => {
    render(<TierAdvancementCard tier="trusted_contributor" />)

    expect(screen.getByText('Trusted Contributor')).toBeInTheDocument()
    expect(screen.getByText('Local Ambassador')).toBeInTheDocument()
    expect(screen.getByText(/50 approved edits/i)).toBeInTheDocument()
    expect(screen.getByText(/10 approved edits on venues or artists/i)).toBeInTheDocument()
    expect(screen.getByText(/180 days/i)).toBeInTheDocument()
  })

  it('renders a "highest tier" message for local_ambassador and no requirements list', () => {
    render(<TierAdvancementCard tier="local_ambassador" />)

    expect(screen.getByText('Local Ambassador')).toBeInTheDocument()
    expect(screen.getByText(/highest contributor tier/i)).toBeInTheDocument()
    expect(screen.queryByText('Requirements')).not.toBeInTheDocument()
  })

  it('links "Learn more about tiers" to /help/tiers', () => {
    render(<TierAdvancementCard tier="new_user" />)

    const learnMore = screen.getByRole('link', {
      name: /Learn more about tiers/i,
    })
    expect(learnMore).toHaveAttribute('href', '/help/tiers')
  })

  // Table-driven check: for every advancing tier, the card MUST render
  //   - the current-tier badge label
  //   - the next-tier badge label
  //   - the "Requirements" header
  //   - a non-empty requirements list (one <li> per requirement)
  // and for the terminal tier, the card MUST render the "highest tier" copy
  // INSTEAD OF a Requirements section.
  it.each([
    { tier: 'new_user', currentLabel: 'New User', nextLabel: 'Contributor', requirementCount: 3 },
    { tier: 'contributor', currentLabel: 'Contributor', nextLabel: 'Trusted Contributor', requirementCount: 3 },
    { tier: 'trusted_contributor', currentLabel: 'Trusted Contributor', nextLabel: 'Local Ambassador', requirementCount: 3 },
  ] satisfies Array<{ tier: UserTier; currentLabel: string; nextLabel: string; requirementCount: number }>)(
    'renders current + next tier badges and $requirementCount requirements for $tier',
    ({ tier, currentLabel, nextLabel, requirementCount }) => {
      const { container } = render(<TierAdvancementCard tier={tier} />)

      // Current tier badge renders in the header row (board H)
      expect(screen.getByText(currentLabel)).toBeInTheDocument()
      // Next tier label appears in the "Next:" block
      expect(screen.getByText(nextLabel)).toBeInTheDocument()
      // Requirements header present
      expect(screen.getByText('Requirements')).toBeInTheDocument()
      // Requirements list rendered as <li> items inside a <ul>
      const reqs = container.querySelectorAll('ul li')
      expect(reqs.length).toBe(requirementCount)
      // Each requirement has non-empty text
      reqs.forEach((li) => {
        expect(li.textContent?.trim().length).toBeGreaterThan(0)
      })
    }
  )

  // For local_ambassador, no "Next" block should render and no <ul> should exist
  it('does NOT render a Next badge or requirements list for local_ambassador', () => {
    const { container } = render(<TierAdvancementCard tier="local_ambassador" />)

    expect(screen.queryByText('Next:')).not.toBeInTheDocument()
    expect(container.querySelector('ul')).toBeNull()
  })

  // The current tier badge and the Next-tier badge MUST be visually distinct
  // (different className strings on the UserTierBadge).
  it('renders current and next tier badges with distinct tier styles for new_user', () => {
    render(<TierAdvancementCard tier="new_user" />)
    const newUserBadge = screen.getByText('New User')
    const contributorBadge = screen.getByText('Contributor')
    expect(newUserBadge.className).not.toBe(contributorBadge.className)
  })
})
