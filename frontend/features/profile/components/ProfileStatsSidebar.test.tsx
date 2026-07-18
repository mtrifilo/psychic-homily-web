import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { ProfileStatsSidebar } from './ProfileStatsSidebar'
import type { ContributionStats } from '@/features/auth'

vi.mock('./ActivityHeatmap', () => ({
  ActivityHeatmap: ({ username }: { username: string }) => (
    <div data-testid="activity-heatmap">Heatmap for {username}</div>
  ),
}))

vi.mock('./PercentileRankings', () => ({
  PercentileRankings: () => (
    <div data-testid="percentile-rankings">Rankings</div>
  ),
}))

function makeStats(
  overrides: Partial<ContributionStats> = {}
): ContributionStats {
  return {
    shows_submitted: 0,
    venues_submitted: 0,
    venue_edits_submitted: 0,
    releases_created: 0,
    labels_created: 0,
    festivals_created: 0,
    artists_edited: 0,
    revisions_made: 0,
    pending_edits_submitted: 0,
    tag_votes_cast: 0,
    relationship_votes_cast: 0,
    request_votes_cast: 0,
    collection_items_added: 0,
    collection_subscriptions: 0,
    reports_filed: 0,
    reports_resolved: 0,
    followers_count: 0,
    following_count: 0,
    moderation_actions: 0,
    total_contributions: 0,
    ...overrides,
  } as ContributionStats
}

describe('ProfileStatsSidebar', () => {
  it('renders nothing when stats are hidden and no count is available', () => {
    const { container } = render(
      <ProfileStatsSidebar username="alice" stats={undefined} statsCount={undefined} />
    )
    expect(container.innerHTML).toBe('')
  })

  it('renders headline numerals with board-A lowercase labels', () => {
    render(
      <ProfileStatsSidebar
        username="alice"
        stats={makeStats({ total_contributions: 340 })}
        collectionsTotal={8}
      />
    )
    expect(screen.getByText('8')).toBeInTheDocument()
    expect(screen.getByText('collections')).toBeInTheDocument()
    expect(screen.getByText('340')).toBeInTheDocument()
    expect(screen.getByText('contributions')).toBeInTheDocument()
  })

  it('formats approval_rate as a percentage from the 0-1 backend fraction', () => {
    render(
      <ProfileStatsSidebar
        username="alice"
        stats={makeStats({ total_contributions: 340, approval_rate: 0.92 })}
      />
    )
    expect(screen.getByText('92%')).toBeInTheDocument()
    expect(screen.getByText('approval rate')).toBeInTheDocument()
  })

  it('omits the approval rate row when approval_rate is absent', () => {
    render(
      <ProfileStatsSidebar
        username="alice"
        stats={makeStats({ total_contributions: 5 })}
      />
    )
    expect(screen.queryByText('approval rate')).not.toBeInTheDocument()
  })

  it('shows the expander affordance with the contents explainer when collapsed', () => {
    render(
      <ProfileStatsSidebar
        username="alice"
        stats={makeStats({ total_contributions: 5 })}
      />
    )
    expect(
      screen.getByRole('button', { name: /show all statistics/i })
    ).toBeInTheDocument()
    expect(
      screen.getByText(
        'full counters · 365-day activity heatmap · percentile rankings'
      )
    ).toBeInTheDocument()
  })

  it('expands to the breakdown list, heatmap and rankings, then collapses', () => {
    render(
      <ProfileStatsSidebar
        username="alice"
        stats={makeStats({
          shows_submitted: 86,
          tag_votes_cast: 120,
          total_contributions: 206,
        })}
      />
    )
    fireEvent.click(screen.getByRole('button', { name: /show all statistics/i }))

    expect(screen.getByText('All contributions')).toBeInTheDocument()
    expect(screen.getByText('Shows submitted')).toBeInTheDocument()
    expect(screen.getByText('86')).toBeInTheDocument()
    expect(screen.getByText('Tag votes')).toBeInTheDocument()
    // Zero rows are filtered from the dense list.
    expect(screen.queryByText('Venues submitted')).not.toBeInTheDocument()
    expect(screen.getByTestId('activity-heatmap')).toBeInTheDocument()
    expect(screen.getByTestId('percentile-rankings')).toBeInTheDocument()

    fireEvent.click(
      screen.getByRole('button', { name: /hide the full statistics/i })
    )
    expect(screen.queryByText('All contributions')).not.toBeInTheDocument()
  })

  it('suppresses the expander when expandable is false', () => {
    render(
      <ProfileStatsSidebar
        username=""
        stats={makeStats({ total_contributions: 5 })}
        expandable={false}
      />
    )
    expect(screen.getByText('5')).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: /show all statistics/i })
    ).not.toBeInTheDocument()
  })

  describe('zero state (board B)', () => {
    it('shows zeroed numerals, the owner onboarding hint, and no expander', () => {
      render(
        <ProfileStatsSidebar
          username="alice"
          stats={makeStats()}
          collectionsTotal={0}
          isOwner
        />
      )
      expect(screen.getAllByText('0')).toHaveLength(2)
      expect(
        screen.getByText(
          'Save a show or follow an artist and your profile starts filling in.'
        )
      ).toBeInTheDocument()
      expect(
        screen.queryByRole('button', { name: /show all statistics/i })
      ).not.toBeInTheDocument()
    })

    it('keeps the expander (and drops the hint) when only breakdown rows are non-zero', () => {
      // Follows / subscriptions / followers are NOT counted in
      // total_contributions; the expander is their only surface.
      render(
        <ProfileStatsSidebar
          username="alice"
          stats={makeStats({ following_count: 5 })}
          collectionsTotal={0}
          isOwner
        />
      )
      expect(
        screen.getByRole('button', { name: /show all statistics/i })
      ).toBeInTheDocument()
      expect(
        screen.queryByText(/Save a show or follow an artist/)
      ).not.toBeInTheDocument()
    })

    it('omits the owner-directed hint for visitors', () => {
      render(
        <ProfileStatsSidebar
          username="alice"
          stats={makeStats()}
          collectionsTotal={0}
        />
      )
      expect(
        screen.queryByText(/Save a show or follow an artist/)
      ).not.toBeInTheDocument()
      expect(
        screen.queryByRole('button', { name: /show all statistics/i })
      ).not.toBeInTheDocument()
    })
  })

  describe('count-only fallback', () => {
    it('renders a single contributions numeral without an expander', () => {
      render(<ProfileStatsSidebar username="alice" statsCount={42} />)
      expect(screen.getByText('42')).toBeInTheDocument()
      expect(screen.getByText('contributions')).toBeInTheDocument()
      expect(
        screen.queryByRole('button', { name: /show all statistics/i })
      ).not.toBeInTheDocument()
    })

    it('renders nothing when the count is zero', () => {
      const { container } = render(
        <ProfileStatsSidebar username="alice" statsCount={0} />
      )
      expect(container.innerHTML).toBe('')
    })
  })
})
