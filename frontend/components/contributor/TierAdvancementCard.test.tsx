import React from 'react'
import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { TierAdvancementCard } from './TierAdvancementCard'

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
})
