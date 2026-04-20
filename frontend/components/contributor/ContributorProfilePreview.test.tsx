import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'
import { ContributorProfilePreview } from './ContributorProfilePreview'
import type { PublicProfileResponse, ContributionsResponse } from '@/features/auth'

// Mock next/link
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

// Mock hooks
const mockUseOwnContributorProfile = vi.fn()
const mockUseOwnContributions = vi.fn()

vi.mock('@/features/auth', () => ({
  useOwnContributorProfile: () => mockUseOwnContributorProfile(),
  useOwnContributions: () => mockUseOwnContributions(),
}))

// Mock child components to isolate unit tests
vi.mock('./UserTierBadge', () => ({
  UserTierBadge: ({ tier }: { tier: string }) => (
    <span data-testid="tier-badge">{tier}</span>
  ),
}))

vi.mock('./ContributionStatsGrid', () => ({
  ContributionStatsGrid: () => (
    <div data-testid="stats-grid">Stats Grid</div>
  ),
}))

vi.mock('./ContributionTimeline', () => ({
  ContributionTimeline: () => (
    <div data-testid="contribution-timeline">Timeline</div>
  ),
}))

function makeProfile(
  overrides: Partial<PublicProfileResponse> = {}
): PublicProfileResponse {
  return {
    username: 'testuser',
    profile_visibility: 'public',
    user_tier: 'contributor',
    joined_at: '2025-01-15T00:00:00Z',
    ...overrides,
  }
}

describe('ContributorProfilePreview', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseOwnContributions.mockReturnValue({ data: null })
  })

  it('renders loading skeleton when loading', () => {
    mockUseOwnContributorProfile.mockReturnValue({
      data: null,
      isLoading: true,
    })

    const { container } = renderWithProviders(<ContributorProfilePreview />)
    // Skeleton has a specific class
    const skeletons = container.querySelectorAll('[class*="animate-pulse"], [data-slot="skeleton"]')
    // Alternative: the skeleton renders placeholder divs
    expect(container.querySelector('.space-y-6')).toBeInTheDocument()
  })

  it('renders error message when profile is null', () => {
    mockUseOwnContributorProfile.mockReturnValue({
      data: null,
      isLoading: false,
    })

    renderWithProviders(<ContributorProfilePreview />)
    expect(
      screen.getByText('Unable to load your contributor profile.')
    ).toBeInTheDocument()
  })

  it('renders profile card with display name from first_name', () => {
    mockUseOwnContributorProfile.mockReturnValue({
      data: makeProfile({ first_name: 'Alice', username: 'alice123' }),
      isLoading: false,
    })

    renderWithProviders(<ContributorProfilePreview />)
    expect(screen.getByText('Alice')).toBeInTheDocument()
    expect(screen.getByText('@alice123')).toBeInTheDocument()
  })

  it('falls back to username when first_name is missing', () => {
    mockUseOwnContributorProfile.mockReturnValue({
      data: makeProfile({ username: 'alice123' }),
      isLoading: false,
    })

    renderWithProviders(<ContributorProfilePreview />)
    expect(screen.getByText('alice123')).toBeInTheDocument()
  })

  it('renders avatar image when avatar_url is provided', () => {
    mockUseOwnContributorProfile.mockReturnValue({
      data: makeProfile({
        avatar_url: 'https://example.com/avatar.jpg',
        first_name: 'Alice',
      }),
      isLoading: false,
    })

    renderWithProviders(<ContributorProfilePreview />)
    const img = screen.getByAltText("Alice's avatar")
    expect(img).toHaveAttribute('src', 'https://example.com/avatar.jpg')
  })

  it('renders initial letter when no avatar_url', () => {
    mockUseOwnContributorProfile.mockReturnValue({
      data: makeProfile({ first_name: 'Bob' }),
      isLoading: false,
    })

    renderWithProviders(<ContributorProfilePreview />)
    expect(screen.getByText('B')).toBeInTheDocument()
  })

  it('renders "?" initial when no name and no username', () => {
    mockUseOwnContributorProfile.mockReturnValue({
      data: makeProfile({ username: '', first_name: undefined }),
      isLoading: false,
    })

    renderWithProviders(<ContributorProfilePreview />)
    expect(screen.getByText('?')).toBeInTheDocument()
  })

  it('shows "View Public Profile" button when profile is public', () => {
    mockUseOwnContributorProfile.mockReturnValue({
      data: makeProfile({
        profile_visibility: 'public',
        username: 'testuser',
      }),
      isLoading: false,
    })

    renderWithProviders(<ContributorProfilePreview />)
    const link = screen.getByText('View Public Profile').closest('a')
    expect(link).toHaveAttribute('href', '/users/testuser')
  })

  it('hides "View Public Profile" button when profile is private', () => {
    mockUseOwnContributorProfile.mockReturnValue({
      data: makeProfile({ profile_visibility: 'private' }),
      isLoading: false,
    })

    renderWithProviders(<ContributorProfilePreview />)
    expect(screen.queryByText('View Public Profile')).not.toBeInTheDocument()
  })

  it('shows "Profile is private" notice when profile is private', () => {
    mockUseOwnContributorProfile.mockReturnValue({
      data: makeProfile({ profile_visibility: 'private' }),
      isLoading: false,
    })

    renderWithProviders(<ContributorProfilePreview />)
    expect(screen.getByText('Profile is private')).toBeInTheDocument()
  })

  it('shows joined date', () => {
    mockUseOwnContributorProfile.mockReturnValue({
      data: makeProfile({ joined_at: '2025-01-15T00:00:00Z' }),
      isLoading: false,
    })

    renderWithProviders(<ContributorProfilePreview />)
    expect(screen.getByText(/Joined January 2025/)).toBeInTheDocument()
  })

  it('shows "Your Impact" section when stats have contributions', () => {
    mockUseOwnContributorProfile.mockReturnValue({
      data: makeProfile({
        stats: {
          shows_submitted: 5,
          venues_submitted: 2,
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
          total_contributions: 7,
        },
      }),
      isLoading: false,
    })

    renderWithProviders(<ContributorProfilePreview />)
    expect(screen.getByText('Your Impact')).toBeInTheDocument()
    expect(screen.getByTestId('stats-grid')).toBeInTheDocument()
  })

  it('shows impact summary text', () => {
    mockUseOwnContributorProfile.mockReturnValue({
      data: makeProfile({
        stats: {
          shows_submitted: 5,
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
          total_contributions: 5,
        },
      }),
      isLoading: false,
    })

    renderWithProviders(<ContributorProfilePreview />)
    expect(
      screen.getByText(
        "You've contributed 5 shows to the knowledge graph."
      )
    ).toBeInTheDocument()
  })

  it('builds impact summary with multiple types', () => {
    mockUseOwnContributorProfile.mockReturnValue({
      data: makeProfile({
        stats: {
          shows_submitted: 3,
          venues_submitted: 2,
          venue_edits_submitted: 0,
          releases_created: 1,
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
          total_contributions: 6,
        },
      }),
      isLoading: false,
    })

    renderWithProviders(<ContributorProfilePreview />)
    expect(
      screen.getByText(
        "You've contributed 3 shows, 2 venues and 1 release to the knowledge graph."
      )
    ).toBeInTheDocument()
  })

  it('uses singular form for count of 1', () => {
    mockUseOwnContributorProfile.mockReturnValue({
      data: makeProfile({
        stats: {
          shows_submitted: 1,
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
          total_contributions: 1,
        },
      }),
      isLoading: false,
    })

    renderWithProviders(<ContributorProfilePreview />)
    expect(
      screen.getByText(
        "You've contributed 1 show to the knowledge graph."
      )
    ).toBeInTheDocument()
  })

  it('hides impact section when total_contributions is 0', () => {
    mockUseOwnContributorProfile.mockReturnValue({
      data: makeProfile({
        stats: {
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
        },
      }),
      isLoading: false,
    })

    renderWithProviders(<ContributorProfilePreview />)
    expect(screen.queryByText('Your Impact')).not.toBeInTheDocument()
  })

  it('shows "Start Contributing" empty state when no stats', () => {
    mockUseOwnContributorProfile.mockReturnValue({
      data: makeProfile(),
      isLoading: false,
    })

    renderWithProviders(<ContributorProfilePreview />)
    expect(screen.getByText('Start Contributing')).toBeInTheDocument()
  })

  it('shows "Recent Activity" when there are contributions', () => {
    mockUseOwnContributorProfile.mockReturnValue({
      data: makeProfile(),
      isLoading: false,
    })
    mockUseOwnContributions.mockReturnValue({
      data: {
        contributions: [
          {
            id: 1,
            action: 'created',
            entity_type: 'show',
            entity_id: 1,
            created_at: '2025-01-01T00:00:00Z',
            source: 'web',
          },
        ],
      },
    })

    renderWithProviders(<ContributorProfilePreview />)
    expect(screen.getByText('Recent Activity')).toBeInTheDocument()
    expect(screen.getByTestId('contribution-timeline')).toBeInTheDocument()
  })

  it('hides "Recent Activity" when no contributions', () => {
    mockUseOwnContributorProfile.mockReturnValue({
      data: makeProfile(),
      isLoading: false,
    })
    mockUseOwnContributions.mockReturnValue({
      data: { contributions: [] },
    })

    renderWithProviders(<ContributorProfilePreview />)
    expect(screen.queryByText('Recent Activity')).not.toBeInTheDocument()
  })

  it('renders tier badge', () => {
    mockUseOwnContributorProfile.mockReturnValue({
      data: makeProfile({ user_tier: 'trusted_contributor' }),
      isLoading: false,
    })

    renderWithProviders(<ContributorProfilePreview />)
    expect(screen.getByTestId('tier-badge')).toHaveTextContent(
      'trusted_contributor'
    )
  })

  it('hides "View Public Profile" when username is missing', () => {
    mockUseOwnContributorProfile.mockReturnValue({
      data: makeProfile({
        profile_visibility: 'public',
        username: '',
      }),
      isLoading: false,
    })

    renderWithProviders(<ContributorProfilePreview />)
    expect(screen.queryByText('View Public Profile')).not.toBeInTheDocument()
  })
})
