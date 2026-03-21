import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { ContributionStatsGrid } from './ContributionStatsGrid'
import type { ContributionStats } from '@/features/auth'

function makeStats(overrides: Partial<ContributionStats> = {}): ContributionStats {
  return {
    shows_submitted: 0,
    venues_submitted: 0,
    venue_edits_submitted: 0,
    releases_created: 0,
    labels_created: 0,
    festivals_created: 0,
    artists_edited: 0,
    moderation_actions: 0,
    total_contributions: 0,
    ...overrides,
  }
}

describe('ContributionStatsGrid', () => {
  it('shows empty state when all stats are zero', () => {
    render(<ContributionStatsGrid stats={makeStats()} />)
    expect(screen.getByText('No contributions yet.')).toBeInTheDocument()
  })

  it('renders stat card for shows_submitted', () => {
    render(<ContributionStatsGrid stats={makeStats({ shows_submitted: 10 })} />)
    expect(screen.getByText('10')).toBeInTheDocument()
    expect(screen.getByText('Shows Submitted')).toBeInTheDocument()
  })

  it('renders stat card for venues_submitted', () => {
    render(<ContributionStatsGrid stats={makeStats({ venues_submitted: 3 })} />)
    expect(screen.getByText('3')).toBeInTheDocument()
    expect(screen.getByText('Venues Submitted')).toBeInTheDocument()
  })

  it('renders stat card for venue_edits_submitted', () => {
    render(<ContributionStatsGrid stats={makeStats({ venue_edits_submitted: 7 })} />)
    expect(screen.getByText('7')).toBeInTheDocument()
    expect(screen.getByText('Venue Edits')).toBeInTheDocument()
  })

  it('renders stat card for releases_created', () => {
    render(<ContributionStatsGrid stats={makeStats({ releases_created: 5 })} />)
    expect(screen.getByText('5')).toBeInTheDocument()
    expect(screen.getByText('Releases Created')).toBeInTheDocument()
  })

  it('renders stat card for labels_created', () => {
    render(<ContributionStatsGrid stats={makeStats({ labels_created: 2 })} />)
    expect(screen.getByText('2')).toBeInTheDocument()
    expect(screen.getByText('Labels Created')).toBeInTheDocument()
  })

  it('renders stat card for festivals_created', () => {
    render(<ContributionStatsGrid stats={makeStats({ festivals_created: 1 })} />)
    expect(screen.getByText('1')).toBeInTheDocument()
    expect(screen.getByText('Festivals Created')).toBeInTheDocument()
  })

  it('renders stat card for artists_edited', () => {
    render(<ContributionStatsGrid stats={makeStats({ artists_edited: 15 })} />)
    expect(screen.getByText('15')).toBeInTheDocument()
    expect(screen.getByText('Artists Edited')).toBeInTheDocument()
  })

  it('renders stat card for moderation_actions', () => {
    render(<ContributionStatsGrid stats={makeStats({ moderation_actions: 42 })} />)
    expect(screen.getByText('42')).toBeInTheDocument()
    expect(screen.getByText('Moderation Actions')).toBeInTheDocument()
  })

  it('only renders cards for non-zero stats', () => {
    render(
      <ContributionStatsGrid
        stats={makeStats({ shows_submitted: 5, artists_edited: 3 })}
      />
    )
    expect(screen.getByText('Shows Submitted')).toBeInTheDocument()
    expect(screen.getByText('Artists Edited')).toBeInTheDocument()
    // Zero-value stats should not appear
    expect(screen.queryByText('Venues Submitted')).not.toBeInTheDocument()
    expect(screen.queryByText('Releases Created')).not.toBeInTheDocument()
    expect(screen.queryByText('Labels Created')).not.toBeInTheDocument()
    expect(screen.queryByText('Festivals Created')).not.toBeInTheDocument()
    expect(screen.queryByText('Moderation Actions')).not.toBeInTheDocument()
    expect(screen.queryByText('Venue Edits')).not.toBeInTheDocument()
  })

  it('renders all stat cards when all stats are non-zero', () => {
    render(
      <ContributionStatsGrid
        stats={makeStats({
          shows_submitted: 10,
          venues_submitted: 5,
          venue_edits_submitted: 3,
          releases_created: 8,
          labels_created: 2,
          festivals_created: 1,
          artists_edited: 12,
          moderation_actions: 6,
        })}
      />
    )
    expect(screen.getByText('Shows Submitted')).toBeInTheDocument()
    expect(screen.getByText('Venues Submitted')).toBeInTheDocument()
    expect(screen.getByText('Venue Edits')).toBeInTheDocument()
    expect(screen.getByText('Releases Created')).toBeInTheDocument()
    expect(screen.getByText('Labels Created')).toBeInTheDocument()
    expect(screen.getByText('Festivals Created')).toBeInTheDocument()
    expect(screen.getByText('Artists Edited')).toBeInTheDocument()
    expect(screen.getByText('Moderation Actions')).toBeInTheDocument()
  })

  it('does not show empty state text when there are non-zero stats', () => {
    render(<ContributionStatsGrid stats={makeStats({ shows_submitted: 1 })} />)
    expect(screen.queryByText('No contributions yet.')).not.toBeInTheDocument()
  })

  it('handles large numbers', () => {
    render(
      <ContributionStatsGrid stats={makeStats({ shows_submitted: 99999 })} />
    )
    expect(screen.getByText('99999')).toBeInTheDocument()
  })
})
