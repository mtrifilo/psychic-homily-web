import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { TierAdvancementCard } from './TierAdvancementCard'
import type { UserTier } from '@/features/auth'
import type { AdvancementProgress } from '@/features/auth'

const mockUseAdvancementProgress = vi.fn()

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

vi.mock('@/features/auth', async () => {
  const actual = await vi.importActual<typeof import('@/features/auth')>(
    '@/features/auth'
  )
  return {
    ...actual,
    useAdvancementProgress: (...args: unknown[]) =>
      mockUseAdvancementProgress(...args),
  }
})

function advancementFor(
  tier: UserTier,
  overrides: Partial<AdvancementProgress> = {}
): AdvancementProgress {
  const byTier: Record<Exclude<UserTier, 'local_ambassador'>, AdvancementProgress> = {
    new_user: {
      current_tier: 'new_user',
      next_tier: 'contributor',
      requirements: [
        { requirement: 'approved_edits', current: 2, threshold: 5, met: false },
        { requirement: 'account_age_days', current: 10, threshold: 14, met: false },
        { requirement: 'email_verified', met: true },
      ],
    },
    contributor: {
      current_tier: 'contributor',
      next_tier: 'trusted_contributor',
      requirements: [
        { requirement: 'approved_edits', current: 20, threshold: 25, met: false },
        { requirement: 'approval_rate', current: 97, threshold: 95, met: true },
        { requirement: 'account_age_days', current: 45, threshold: 60, met: false },
      ],
    },
    trusted_contributor: {
      current_tier: 'trusted_contributor',
      next_tier: 'local_ambassador',
      requirements: [
        { requirement: 'approved_edits', current: 32, threshold: 50, met: false },
        { requirement: 'city_edits', current: 8, threshold: 10, met: false },
        { requirement: 'account_age_days', current: 200, threshold: 180, met: true },
      ],
    },
  }
  if (tier === 'local_ambassador') {
    return { current_tier: 'local_ambassador', requirements: [], ...overrides }
  }
  return { ...byTier[tier], ...overrides }
}

describe('TierAdvancementCard', () => {
  beforeEach(() => {
    mockUseAdvancementProgress.mockReset()
    mockUseAdvancementProgress.mockImplementation((enabled: boolean) => ({
      data: enabled ? advancementFor('new_user') : undefined,
      isLoading: false,
    }))
  })

  it('renders the board-H header: sentence-case title + tier badge', () => {
    render(<TierAdvancementCard tier="new_user" />)
    expect(screen.getByText('Contributor tier')).toBeInTheDocument()
    expect(screen.getByText('New User')).toBeInTheDocument()
    expect(screen.queryByText('Your tier:')).not.toBeInTheDocument()
  })

  it('renders the current tier badge for new_user with next-tier requirements', () => {
    render(<TierAdvancementCard tier="new_user" />)

    expect(screen.getByText('New User')).toBeInTheDocument()
    expect(screen.getByText('Contributor')).toBeInTheDocument()
    expect(screen.getByText('Requirements')).toBeInTheDocument()
    expect(screen.getByText(/5 approved edits/i)).toBeInTheDocument()
    expect(screen.getByText(/14 days/i)).toBeInTheDocument()
    expect(screen.getByText(/Verified email/i)).toBeInTheDocument()
  })

  it('renders board-H progress bar + mono counter from advancement data', () => {
    mockUseAdvancementProgress.mockReturnValue({
      data: advancementFor('trusted_contributor'),
      isLoading: false,
    })
    render(<TierAdvancementCard tier="trusted_contributor" />)

    expect(screen.getByText(/32 \/ 50 qualifying edits/i)).toBeInTheDocument()
    const bar = screen.getByRole('progressbar', {
      name: /Approved edits toward next tier/i,
    })
    expect(bar).toHaveAttribute('aria-valuenow', '32')
    expect(bar).toHaveAttribute('aria-valuemax', '50')
  })

  it('marks met requirements with a check and leaves unmet as bullets', () => {
    mockUseAdvancementProgress.mockReturnValue({
      data: advancementFor('new_user'),
      isLoading: false,
    })
    render(<TierAdvancementCard tier="new_user" />)

    expect(screen.getByLabelText('Met')).toBeInTheDocument()
  })

  it('does not fetch advancement for local_ambassador', () => {
    render(<TierAdvancementCard tier="local_ambassador" />)
    expect(mockUseAdvancementProgress).toHaveBeenCalledWith(false)
  })

  it('renders requirements for contributor advancing to trusted_contributor', () => {
    mockUseAdvancementProgress.mockReturnValue({
      data: advancementFor('contributor'),
      isLoading: false,
    })
    render(<TierAdvancementCard tier="contributor" />)

    expect(screen.getByText('Contributor')).toBeInTheDocument()
    expect(screen.getByText('Trusted Contributor')).toBeInTheDocument()
    expect(screen.getByText(/25 approved edits/i)).toBeInTheDocument()
    expect(screen.getByText(/95% approval rate/i)).toBeInTheDocument()
    expect(screen.getByText(/60 days/i)).toBeInTheDocument()
  })

  it('renders requirements for trusted_contributor advancing to local_ambassador', () => {
    mockUseAdvancementProgress.mockReturnValue({
      data: advancementFor('trusted_contributor'),
      isLoading: false,
    })
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
    expect(screen.queryByRole('progressbar')).not.toBeInTheDocument()
  })

  it('links "Learn more about tiers" to /help/tiers', () => {
    render(<TierAdvancementCard tier="new_user" />)

    const learnMore = screen.getByRole('link', {
      name: /Learn more about tiers/i,
    })
    expect(learnMore).toHaveAttribute('href', '/help/tiers')
  })

  it.each([
    { tier: 'new_user', currentLabel: 'New User', nextLabel: 'Contributor', requirementCount: 3 },
    { tier: 'contributor', currentLabel: 'Contributor', nextLabel: 'Trusted Contributor', requirementCount: 3 },
    { tier: 'trusted_contributor', currentLabel: 'Trusted Contributor', nextLabel: 'Local Ambassador', requirementCount: 3 },
  ] satisfies Array<{ tier: UserTier; currentLabel: string; nextLabel: string; requirementCount: number }>)(
    'renders current + next tier badges and $requirementCount requirements for $tier',
    ({ tier, currentLabel, nextLabel, requirementCount }) => {
      mockUseAdvancementProgress.mockReturnValue({
        data: advancementFor(tier),
        isLoading: false,
      })
      const { container } = render(<TierAdvancementCard tier={tier} />)

      expect(screen.getByText(currentLabel)).toBeInTheDocument()
      expect(screen.getByText(nextLabel)).toBeInTheDocument()
      expect(screen.getByText('Requirements')).toBeInTheDocument()
      const reqs = container.querySelectorAll('ul li')
      expect(reqs.length).toBe(requirementCount)
      reqs.forEach((li) => {
        expect(li.textContent?.trim().length).toBeGreaterThan(0)
      })
    }
  )

  it('does NOT render a Next badge or requirements list for local_ambassador', () => {
    const { container } = render(<TierAdvancementCard tier="local_ambassador" />)

    expect(screen.queryByText('Next:')).not.toBeInTheDocument()
    expect(container.querySelector('ul')).toBeNull()
  })

  it('renders current and next tier badges with distinct tier styles for new_user', () => {
    render(<TierAdvancementCard tier="new_user" />)
    const newUserBadge = screen.getByText('New User')
    const contributorBadge = screen.getByText('Contributor')
    expect(newUserBadge.className).not.toBe(contributorBadge.className)
  })

  it('hides the progress bar until advancement data arrives (new-user unaffected shape)', () => {
    mockUseAdvancementProgress.mockReturnValue({
      data: undefined,
      isLoading: true,
    })
    render(<TierAdvancementCard tier="new_user" />)

    expect(screen.queryByRole('progressbar')).not.toBeInTheDocument()
    expect(screen.queryByText(/qualifying edits/i)).not.toBeInTheDocument()
    // Static requirements list still renders from tiers.ts
    expect(screen.getByText(/5 approved edits/i)).toBeInTheDocument()
  })
})
