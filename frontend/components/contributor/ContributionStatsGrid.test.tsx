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
    revisions_made: 0,
    pending_edits_submitted: 0,
    tag_votes_cast: 0,
    relationship_votes_cast: 0,
    request_votes_cast: 0,
    collection_items_added: 0,
    collection_subscriptions: 0,
    shows_attended: 0,
    reports_filed: 0,
    reports_resolved: 0,
    followers_count: 0,
    following_count: 0,
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

  // New stat type tests
  it('renders stat card for revisions_made', () => {
    render(<ContributionStatsGrid stats={makeStats({ revisions_made: 8 })} />)
    expect(screen.getByText('8')).toBeInTheDocument()
    expect(screen.getByText('Revisions Made')).toBeInTheDocument()
  })

  it('renders stat card for pending_edits_submitted', () => {
    render(<ContributionStatsGrid stats={makeStats({ pending_edits_submitted: 4 })} />)
    expect(screen.getByText('4')).toBeInTheDocument()
    expect(screen.getByText('Pending Edits')).toBeInTheDocument()
  })

  it('renders stat card for tag_votes_cast', () => {
    render(<ContributionStatsGrid stats={makeStats({ tag_votes_cast: 20 })} />)
    expect(screen.getByText('20')).toBeInTheDocument()
    expect(screen.getByText('Tag Votes')).toBeInTheDocument()
  })

  it('renders stat card for relationship_votes_cast', () => {
    render(<ContributionStatsGrid stats={makeStats({ relationship_votes_cast: 6 })} />)
    expect(screen.getByText('6')).toBeInTheDocument()
    expect(screen.getByText('Relationship Votes')).toBeInTheDocument()
  })

  it('renders stat card for request_votes_cast', () => {
    render(<ContributionStatsGrid stats={makeStats({ request_votes_cast: 12 })} />)
    expect(screen.getByText('12')).toBeInTheDocument()
    expect(screen.getByText('Request Votes')).toBeInTheDocument()
  })

  it('renders stat card for collection_items_added', () => {
    render(<ContributionStatsGrid stats={makeStats({ collection_items_added: 9 })} />)
    expect(screen.getByText('9')).toBeInTheDocument()
    expect(screen.getByText('Collection Items')).toBeInTheDocument()
  })

  it('renders stat card for collection_subscriptions', () => {
    render(<ContributionStatsGrid stats={makeStats({ collection_subscriptions: 3 })} />)
    expect(screen.getByText('3')).toBeInTheDocument()
    expect(screen.getByText('Subscriptions')).toBeInTheDocument()
  })

  it('renders stat card for shows_attended', () => {
    render(<ContributionStatsGrid stats={makeStats({ shows_attended: 25 })} />)
    expect(screen.getByText('25')).toBeInTheDocument()
    expect(screen.getByText('Shows Attended')).toBeInTheDocument()
  })

  it('renders stat card for reports_filed', () => {
    render(<ContributionStatsGrid stats={makeStats({ reports_filed: 5 })} />)
    expect(screen.getByText('5')).toBeInTheDocument()
    expect(screen.getByText('Reports Filed')).toBeInTheDocument()
  })

  it('renders stat card for reports_resolved', () => {
    render(<ContributionStatsGrid stats={makeStats({ reports_resolved: 11 })} />)
    expect(screen.getByText('11')).toBeInTheDocument()
    expect(screen.getByText('Reports Resolved')).toBeInTheDocument()
  })

  it('renders stat card for following_count', () => {
    render(<ContributionStatsGrid stats={makeStats({ following_count: 7 })} />)
    expect(screen.getByText('7')).toBeInTheDocument()
    expect(screen.getByText('Following')).toBeInTheDocument()
  })

  it('renders approval rate when present', () => {
    render(<ContributionStatsGrid stats={makeStats({ approval_rate: 0.85 })} />)
    expect(screen.getByText('85%')).toBeInTheDocument()
    expect(screen.getByText('Approval Rate')).toBeInTheDocument()
  })

  it('does not render approval rate when absent', () => {
    render(<ContributionStatsGrid stats={makeStats({ shows_submitted: 1 })} />)
    expect(screen.queryByText('Approval Rate')).not.toBeInTheDocument()
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
    expect(screen.queryByText('Tag Votes')).not.toBeInTheDocument()
    expect(screen.queryByText('Revisions Made')).not.toBeInTheDocument()
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
          revisions_made: 4,
          pending_edits_submitted: 6,
          tag_votes_cast: 20,
          relationship_votes_cast: 7,
          request_votes_cast: 9,
          collection_items_added: 11,
          collection_subscriptions: 3,
          shows_attended: 15,
          reports_filed: 2,
          reports_resolved: 5,
          followers_count: 8,
          following_count: 14,
          moderation_actions: 6,
          approval_rate: 0.92,
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
    expect(screen.getByText('Revisions Made')).toBeInTheDocument()
    expect(screen.getByText('Pending Edits')).toBeInTheDocument()
    expect(screen.getByText('Tag Votes')).toBeInTheDocument()
    expect(screen.getByText('Relationship Votes')).toBeInTheDocument()
    expect(screen.getByText('Request Votes')).toBeInTheDocument()
    expect(screen.getByText('Collection Items')).toBeInTheDocument()
    expect(screen.getByText('Subscriptions')).toBeInTheDocument()
    expect(screen.getByText('Shows Attended')).toBeInTheDocument()
    expect(screen.getByText('Reports Filed')).toBeInTheDocument()
    expect(screen.getByText('Reports Resolved')).toBeInTheDocument()
    expect(screen.getByText('Followers')).toBeInTheDocument()
    expect(screen.getByText('Following')).toBeInTheDocument()
    expect(screen.getByText('Approval Rate')).toBeInTheDocument()
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
