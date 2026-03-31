import React from 'react'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'
import { PublicProfile } from './PublicProfile'
import type { PublicProfileResponse } from '@/features/auth'

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
const mockUsePublicProfile = vi.fn()
const mockUsePublicContributions = vi.fn()

vi.mock('@/features/auth', () => ({
  usePublicProfile: (username: string) => mockUsePublicProfile(username),
  usePublicContributions: (username: string, opts: unknown) =>
    mockUsePublicContributions(username, opts),
}))

// Mock child components
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

vi.mock('./ProfileSections', () => ({
  ProfileSections: () => (
    <div data-testid="profile-sections">Sections</div>
  ),
}))

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

function makeProfile(
  overrides: Partial<PublicProfileResponse> = {}
): PublicProfileResponse {
  return {
    username: 'alice',
    profile_visibility: 'public',
    user_tier: 'contributor',
    joined_at: '2025-01-15T00:00:00Z',
    ...overrides,
  }
}

describe('PublicProfile', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-03-19T12:00:00Z'))
    mockUsePublicContributions.mockReturnValue({ data: null })
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('renders loading skeleton while fetching', () => {
    mockUsePublicProfile.mockReturnValue({
      data: null,
      isLoading: true,
      error: null,
    })

    const { container } = renderWithProviders(
      <PublicProfile username="alice" />
    )
    // Skeleton renders placeholder elements
    expect(container.querySelector('.space-y-6')).toBeInTheDocument()
  })

  it('renders 404 page for user not found', () => {
    const error = new Error('Not found')
    Object.assign(error, { status: 404 })

    mockUsePublicProfile.mockReturnValue({
      data: null,
      isLoading: false,
      error,
    })

    renderWithProviders(<PublicProfile username="nonexistent" />)
    expect(screen.getByText('User Not Found')).toBeInTheDocument()
    expect(
      screen.getByText(/could not be found/)
    ).toBeInTheDocument()
  })

  it('renders generic error page for non-404 errors', () => {
    const error = new Error('Server error')
    Object.assign(error, { status: 500 })

    mockUsePublicProfile.mockReturnValue({
      data: null,
      isLoading: false,
      error,
    })

    renderWithProviders(<PublicProfile username="alice" />)
    expect(screen.getByText('Error')).toBeInTheDocument()
    expect(
      screen.getByText('Failed to load profile. Please try again later.')
    ).toBeInTheDocument()
  })

  it('renders private profile message', () => {
    mockUsePublicProfile.mockReturnValue({
      data: makeProfile({ profile_visibility: 'private' }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<PublicProfile username="alice" />)
    expect(screen.getByText('Private Profile')).toBeInTheDocument()
    expect(
      screen.getByText("This user's profile is set to private.")
    ).toBeInTheDocument()
  })

  it('renders null when profile data is missing', () => {
    mockUsePublicProfile.mockReturnValue({
      data: null,
      isLoading: false,
      error: null,
    })

    const { container } = renderWithProviders(
      <PublicProfile username="alice" />
    )
    expect(container.firstChild).toBeNull()
  })

  it('renders profile header with display name and username', () => {
    mockUsePublicProfile.mockReturnValue({
      data: makeProfile({
        first_name: 'Alice',
        username: 'alice',
      }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<PublicProfile username="alice" />)
    expect(screen.getByText('Alice')).toBeInTheDocument()
    expect(screen.getByText('@alice')).toBeInTheDocument()
  })

  it('falls back to username when first_name is missing', () => {
    mockUsePublicProfile.mockReturnValue({
      data: makeProfile({ username: 'alice' }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<PublicProfile username="alice" />)
    // Username appears both as display name and @username
    const aliceElements = screen.getAllByText(/alice/)
    expect(aliceElements.length).toBeGreaterThanOrEqual(1)
  })

  it('renders avatar image when avatar_url is provided', () => {
    mockUsePublicProfile.mockReturnValue({
      data: makeProfile({
        first_name: 'Alice',
        avatar_url: 'https://example.com/avatar.jpg',
      }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<PublicProfile username="alice" />)
    const img = screen.getByAltText("Alice's avatar")
    expect(img).toHaveAttribute('src', 'https://example.com/avatar.jpg')
  })

  it('renders initial letter when no avatar_url', () => {
    mockUsePublicProfile.mockReturnValue({
      data: makeProfile({ first_name: 'Bob' }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<PublicProfile username="bob" />)
    expect(screen.getByText('B')).toBeInTheDocument()
  })

  it('renders bio when provided', () => {
    mockUsePublicProfile.mockReturnValue({
      data: makeProfile({ bio: 'Music enthusiast and show-goer.' }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<PublicProfile username="alice" />)
    expect(
      screen.getByText('Music enthusiast and show-goer.')
    ).toBeInTheDocument()
  })

  it('does not render bio when not provided', () => {
    mockUsePublicProfile.mockReturnValue({
      data: makeProfile({ bio: undefined }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<PublicProfile username="alice" />)
    expect(
      screen.queryByText('Music enthusiast and show-goer.')
    ).not.toBeInTheDocument()
  })

  it('shows joined date', () => {
    mockUsePublicProfile.mockReturnValue({
      data: makeProfile({ joined_at: '2025-06-15T12:00:00Z' }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<PublicProfile username="alice" />)
    expect(screen.getByText(/Joined June 2025/)).toBeInTheDocument()
  })

  it('shows last active "Today"', () => {
    mockUsePublicProfile.mockReturnValue({
      data: makeProfile({ last_active: '2026-03-19T10:00:00Z' }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<PublicProfile username="alice" />)
    expect(screen.getByText(/Active Today/)).toBeInTheDocument()
  })

  it('shows last active "Yesterday"', () => {
    mockUsePublicProfile.mockReturnValue({
      data: makeProfile({ last_active: '2026-03-18T10:00:00Z' }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<PublicProfile username="alice" />)
    expect(screen.getByText(/Active Yesterday/)).toBeInTheDocument()
  })

  it('shows last active in days', () => {
    mockUsePublicProfile.mockReturnValue({
      data: makeProfile({ last_active: '2026-03-15T10:00:00Z' }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<PublicProfile username="alice" />)
    expect(screen.getByText(/Active 4 days ago/)).toBeInTheDocument()
  })

  it('shows last active in weeks', () => {
    mockUsePublicProfile.mockReturnValue({
      data: makeProfile({ last_active: '2026-03-05T10:00:00Z' }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<PublicProfile username="alice" />)
    expect(screen.getByText(/Active 2 weeks ago/)).toBeInTheDocument()
  })

  it('does not show last active when not provided', () => {
    mockUsePublicProfile.mockReturnValue({
      data: makeProfile({ last_active: undefined }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<PublicProfile username="alice" />)
    expect(screen.queryByText(/Active/)).not.toBeInTheDocument()
  })

  it('shows stats count only section', () => {
    mockUsePublicProfile.mockReturnValue({
      data: makeProfile({
        stats: undefined,
        stats_count: 42,
      }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<PublicProfile username="alice" />)
    expect(screen.getByText('42')).toBeInTheDocument()
    expect(screen.getByText(/total contributions/)).toBeInTheDocument()
  })

  it('does not show stats count when it is 0', () => {
    mockUsePublicProfile.mockReturnValue({
      data: makeProfile({
        stats: undefined,
        stats_count: 0,
      }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<PublicProfile username="alice" />)
    expect(screen.queryByText(/total contributions/)).not.toBeInTheDocument()
  })

  it('shows full stats grid when stats are provided', () => {
    mockUsePublicProfile.mockReturnValue({
      data: makeProfile({
        stats: {
          shows_submitted: 10,
          venues_submitted: 5,
          venue_edits_submitted: 3,
          releases_created: 0,
          labels_created: 0,
          festivals_created: 0,
          artists_edited: 0,
          moderation_actions: 0,
          total_contributions: 18,
        },
      }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<PublicProfile username="alice" />)
    expect(screen.getByText('Contributions')).toBeInTheDocument()
    expect(screen.getByTestId('stats-grid')).toBeInTheDocument()
  })

  it('shows recent activity when contributions exist', () => {
    mockUsePublicProfile.mockReturnValue({
      data: makeProfile(),
      isLoading: false,
      error: null,
    })
    mockUsePublicContributions.mockReturnValue({
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

    renderWithProviders(<PublicProfile username="alice" />)
    expect(screen.getByText('Recent Activity')).toBeInTheDocument()
    expect(screen.getByTestId('contribution-timeline')).toBeInTheDocument()
  })

  it('hides recent activity when no contributions', () => {
    mockUsePublicProfile.mockReturnValue({
      data: makeProfile(),
      isLoading: false,
      error: null,
    })
    mockUsePublicContributions.mockReturnValue({
      data: { contributions: [] },
    })

    renderWithProviders(<PublicProfile username="alice" />)
    expect(screen.queryByText('Recent Activity')).not.toBeInTheDocument()
  })

  it('shows custom sections when available', () => {
    mockUsePublicProfile.mockReturnValue({
      data: makeProfile({
        sections: [
          {
            id: 1,
            title: 'About',
            content: 'Hi',
            position: 0,
            is_visible: true,
            created_at: '2025-01-01T00:00:00Z',
            updated_at: '2025-01-01T00:00:00Z',
          },
        ],
      }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<PublicProfile username="alice" />)
    expect(screen.getByTestId('profile-sections')).toBeInTheDocument()
  })

  it('shows empty state when profile has no content', () => {
    mockUsePublicProfile.mockReturnValue({
      data: makeProfile({
        stats: undefined,
        stats_count: undefined,
        sections: undefined,
      }),
      isLoading: false,
      error: null,
    })
    mockUsePublicContributions.mockReturnValue({
      data: { contributions: [] },
    })

    renderWithProviders(<PublicProfile username="alice" />)
    expect(
      screen.getByText(
        "This user hasn't added any content to their profile yet."
      )
    ).toBeInTheDocument()
  })

  it('renders tier badge', () => {
    mockUsePublicProfile.mockReturnValue({
      data: makeProfile({ user_tier: 'local_ambassador' }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<PublicProfile username="alice" />)
    expect(screen.getByTestId('tier-badge')).toHaveTextContent(
      'local_ambassador'
    )
  })

  it('prefers stats_grid over stats_count when stats provided', () => {
    mockUsePublicProfile.mockReturnValue({
      data: makeProfile({
        stats: {
          shows_submitted: 5,
          venues_submitted: 0,
          venue_edits_submitted: 0,
          releases_created: 0,
          labels_created: 0,
          festivals_created: 0,
          artists_edited: 0,
          moderation_actions: 0,
          total_contributions: 5,
        },
        stats_count: 5,
      }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<PublicProfile username="alice" />)
    // Should show full stats, not just count
    expect(screen.getByTestId('stats-grid')).toBeInTheDocument()
    expect(screen.queryByText(/total contributions/)).not.toBeInTheDocument()
  })
})
