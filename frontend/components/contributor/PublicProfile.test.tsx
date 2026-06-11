import React from 'react'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { screen, fireEvent } from '@testing-library/react'
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

vi.mock('@/features/auth', () => ({
  usePublicProfile: (username: string) => mockUsePublicProfile(username),
}))

// The new profile-list sections (PSY-1045) own their hooks and have their own
// suites — mock them here so this suite stays focused on PublicProfile's
// composition and gating.
vi.mock('./ProfileFollowing', () => ({
  ProfileFollowing: ({ username }: { username: string }) => (
    <div data-testid="profile-following">Following for {username}</div>
  ),
}))

vi.mock('./ProfileAttendedShows', () => ({
  ProfileAttendedShows: ({ username }: { username: string }) => (
    <div data-testid="profile-attended-shows">Attended for {username}</div>
  ),
}))

vi.mock('./ProfileCollections', () => ({
  ProfileCollections: ({ username }: { username: string }) => (
    <div data-testid="profile-collections">Collections for {username}</div>
  ),
}))

vi.mock('./ProfileFieldNotes', () => ({
  ProfileFieldNotes: ({ username }: { username: string }) => (
    <div data-testid="profile-field-notes">Field notes for {username}</div>
  ),
}))

// Owner-detection compares the logged-in user (from AuthContext) against the
// viewed profile's username. Default to an anonymous viewer; owner tests set
// mockUser explicitly. PSY-1025.
let mockUser: { username?: string } | null = null

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({ user: mockUser }),
}))

// Mock child components
vi.mock('./UserTierBadge', () => ({
  UserTierBadge: ({ tier }: { tier: string }) => (
    <span data-testid="tier-badge">{tier}</span>
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

// PSY-945: PublicProfile mounts <UserCollections username=...>, which fires
// a real GET /users/:username/collections. This suite never awaits it; left
// un-stubbed it leaked to the real network under the old
// onUnhandledRequest:'bypass' policy and could still be pending at worker
// teardown ("Closing rpc while fetch was pending"). Stub it to stay hermetic.
const mockUseUserPublicCollections = vi.fn()

vi.mock('@/features/collections', () => ({
  UserCollections: ({ username }: { username: string }) => (
    <div data-testid="user-collections">Collections for {username}</div>
  ),
  useUserPublicCollections: (username: string) =>
    mockUseUserPublicCollections(username),
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
    mockUseUserPublicCollections.mockReturnValue({ data: undefined })
    mockUser = null
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

  it('does NOT show "Edit profile" on a visitor view of a private profile', () => {
    mockUser = { username: 'bob' }
    mockUsePublicProfile.mockReturnValue({
      data: makeProfile({ username: 'alice', profile_visibility: 'private' }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<PublicProfile username="alice" />)
    expect(screen.getByText('Private Profile')).toBeInTheDocument()
    expect(
      screen.queryByRole('link', { name: /edit profile/i })
    ).not.toBeInTheDocument()
  })

  it('shows owner copy + Edit affordance on the owner view of a private profile', () => {
    // The backend returns the private profile to its owner; the owner must be
    // able to reach settings to change visibility (PSY-1025).
    mockUser = { username: 'alice' }
    mockUsePublicProfile.mockReturnValue({
      data: makeProfile({ username: 'alice', profile_visibility: 'private' }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<PublicProfile username="alice" />)
    expect(screen.getByText('Your Profile Is Private')).toBeInTheDocument()
    const editLink = screen.getByRole('link', { name: /edit profile/i })
    expect(editLink).toHaveAttribute('href', '/profile')
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
    expect(screen.getByText(/joined June 2025/)).toBeInTheDocument()
  })

  it('shows last active "today"', () => {
    mockUsePublicProfile.mockReturnValue({
      data: makeProfile({ last_active: '2026-03-19T10:00:00Z' }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<PublicProfile username="alice" />)
    expect(screen.getByText(/active today/)).toBeInTheDocument()
  })

  it('shows last active "yesterday"', () => {
    mockUsePublicProfile.mockReturnValue({
      data: makeProfile({ last_active: '2026-03-18T10:00:00Z' }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<PublicProfile username="alice" />)
    expect(screen.getByText(/active yesterday/)).toBeInTheDocument()
  })

  it('shows last active in days', () => {
    mockUsePublicProfile.mockReturnValue({
      data: makeProfile({ last_active: '2026-03-15T10:00:00Z' }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<PublicProfile username="alice" />)
    expect(screen.getByText(/active 4 days ago/)).toBeInTheDocument()
  })

  it('shows last active in weeks', () => {
    mockUsePublicProfile.mockReturnValue({
      data: makeProfile({ last_active: '2026-03-05T10:00:00Z' }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<PublicProfile username="alice" />)
    expect(screen.getByText(/active 2 weeks ago/)).toBeInTheDocument()
  })

  it('does not show last active when not provided', () => {
    mockUsePublicProfile.mockReturnValue({
      data: makeProfile({ last_active: undefined }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<PublicProfile username="alice" />)
    expect(screen.queryByText(/active/)).not.toBeInTheDocument()
  })

  it('shows the count-only contributions line in the stats sidebar', () => {
    mockUsePublicProfile.mockReturnValue({
      data: makeProfile({
        stats: undefined,
        stats_count: 42,
      }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<PublicProfile username="alice" />)
    // count_only stats render as a single headline line in the sidebar card
    // (PSY-1045) — no expander, no grid.
    expect(screen.getByText('Statistics')).toBeInTheDocument()
    expect(screen.getByText('42')).toBeInTheDocument()
    expect(screen.getByText('contributions')).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: /show all statistics/i })
    ).not.toBeInTheDocument()
    expect(screen.queryByText('All contributions')).not.toBeInTheDocument()
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
          total_contributions: 18,
        },
      }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<PublicProfile username="alice" />)
    // The full dashboard is DEMOTED behind the sidebar expander (PSY-1045):
    // headline numbers visible, grid/heatmap/rankings only after expanding.
    expect(screen.getByText('Statistics')).toBeInTheDocument()
    expect(screen.getByText('18')).toBeInTheDocument() // total contributions headline
    expect(screen.queryByText('All contributions')).not.toBeInTheDocument()
    expect(screen.queryByTestId('activity-heatmap')).not.toBeInTheDocument()
    expect(screen.queryByTestId('percentile-rankings')).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /show all statistics/i }))

    // The dense breakdown list (board G) renders non-zero rows only.
    expect(screen.getByText('All contributions')).toBeInTheDocument()
    expect(screen.getByText('Shows submitted')).toBeInTheDocument()
    expect(screen.getByText('10')).toBeInTheDocument()
    expect(screen.queryByText('Releases created')).not.toBeInTheDocument()
    expect(screen.getByTestId('activity-heatmap')).toBeInTheDocument()
    expect(screen.getByTestId('percentile-rankings')).toBeInTheDocument()

    fireEvent.click(
      screen.getByRole('button', { name: /hide the full statistics/i })
    )
    expect(screen.queryByText('All contributions')).not.toBeInTheDocument()
  })

  it('does not render a Recent activity section (removed per design boards, PSY-1062)', () => {
    mockUsePublicProfile.mockReturnValue({
      data: makeProfile(),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<PublicProfile username="alice" />)
    expect(screen.queryByText('Recent activity')).not.toBeInTheDocument()
  })

  // --- Header Share affordance (PSY-1062, boards A/C) ---

  it('shows Share to visitors and copies the profile URL with inline confirmation', async () => {
    // Real timers: the copied-state reset uses setTimeout and the clipboard
    // call is async; fake timers would deadlock the await.
    vi.useRealTimers()
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.assign(navigator, { clipboard: { writeText } })

    mockUsePublicProfile.mockReturnValue({
      data: makeProfile(),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<PublicProfile username="alice" />)
    const share = screen.getByRole('button', {
      name: /copy a link to this profile/i,
    })
    expect(share).toHaveTextContent('Share')
    fireEvent.click(share)

    expect(writeText).toHaveBeenCalledWith(
      `${window.location.origin}/users/alice`
    )
    expect(await screen.findByText('Copied ✓')).toBeInTheDocument()
  })

  it('shows both Edit profile and Share to the owner', () => {
    mockUser = { username: 'alice' }
    mockUsePublicProfile.mockReturnValue({
      data: makeProfile({ username: 'alice' }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<PublicProfile username="alice" />)
    expect(
      screen.getByRole('link', { name: /edit profile/i })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: /copy a link to this profile/i })
    ).toBeInTheDocument()
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
    // Empty-state requires KNOWN-zero contributions (visible stats at 0) —
    // with stats hidden we can't claim emptiness (see the visitor tests).
    mockUsePublicProfile.mockReturnValue({
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
        stats_count: undefined,
        sections: undefined,
      }),
      isLoading: false,
      error: null,
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
        stats_count: 5,
      }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<PublicProfile username="alice" />)
    // Full stats win over the count-only fallback: the expander affordance
    // renders (count-only mode has no expander) and opening it shows the
    // breakdown panel.
    expect(
      screen.getByRole('button', { name: /show all statistics/i })
    ).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /show all statistics/i }))
    expect(screen.getByText('All contributions')).toBeInTheDocument()
  })

  // --- Owner-only Edit affordance (PSY-1025) ---

  it('shows "Edit profile" affordance to the profile owner', () => {
    mockUser = { username: 'alice' }
    mockUsePublicProfile.mockReturnValue({
      data: makeProfile({ username: 'alice' }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<PublicProfile username="alice" />)
    const editLink = screen.getByRole('link', { name: /edit profile/i })
    expect(editLink).toBeInTheDocument()
    expect(editLink).toHaveAttribute('href', '/profile')
  })

  it('matches owner case-insensitively (URL casing differs from stored)', () => {
    mockUser = { username: 'Alice' }
    mockUsePublicProfile.mockReturnValue({
      data: makeProfile({ username: 'alice' }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<PublicProfile username="alice" />)
    expect(
      screen.getByRole('link', { name: /edit profile/i })
    ).toBeInTheDocument()
  })

  it('does NOT show "Edit profile" to a different logged-in user', () => {
    mockUser = { username: 'bob' }
    mockUsePublicProfile.mockReturnValue({
      data: makeProfile({ username: 'alice' }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<PublicProfile username="alice" />)
    expect(
      screen.queryByRole('link', { name: /edit profile/i })
    ).not.toBeInTheDocument()
  })

  it('does NOT show "Edit profile" to an anonymous visitor', () => {
    mockUser = null
    mockUsePublicProfile.mockReturnValue({
      data: makeProfile({ username: 'alice' }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<PublicProfile username="alice" />)
    expect(
      screen.queryByRole('link', { name: /edit profile/i })
    ).not.toBeInTheDocument()
  })

  it('does NOT match owner when the logged-in user has no username', () => {
    mockUser = {}
    mockUsePublicProfile.mockReturnValue({
      data: makeProfile({ username: 'alice' }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<PublicProfile username="alice" />)
    expect(
      screen.queryByRole('link', { name: /edit profile/i })
    ).not.toBeInTheDocument()
  })
  // --- PSY-1045 content-first layout ---

  it('composes the content sections in content-first order', () => {
    mockUsePublicProfile.mockReturnValue({
      data: makeProfile({ bio: 'Desert-scene lifer.' }),
      isLoading: false,
      error: null,
    })
    mockUseUserPublicCollections.mockReturnValue({
      data: { collections: [], total: 2 },
    })

    renderWithProviders(<PublicProfile username="alice" />)

    // All content sections render via their (mocked) children.
    expect(screen.getByText('Bio')).toBeInTheDocument()
    expect(screen.getByTestId('profile-following')).toBeInTheDocument()
    expect(screen.getByTestId('profile-collections')).toBeInTheDocument()
    expect(screen.getByTestId('profile-attended-shows')).toBeInTheDocument()
    expect(screen.getByTestId('profile-field-notes')).toBeInTheDocument()

    // Bio (content) precedes the following section in the DOM, and the
    // attended-shows diary precedes field notes — the content-first order.
    const bio = screen.getByText('Desert-scene lifer.')
    const following = screen.getByTestId('profile-following')
    const attended = screen.getByTestId('profile-attended-shows')
    const notes = screen.getByTestId('profile-field-notes')
    expect(
      bio.compareDocumentPosition(following) &
        Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy()
    expect(
      attended.compareDocumentPosition(notes) & Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy()
  })

  it('shows the Get started checklist to the owner of an empty profile', () => {
    mockUser = { username: 'alice' }
    mockUsePublicProfile.mockReturnValue({
      data: makeProfile({
        username: 'alice',
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
      error: null,
    })

    renderWithProviders(<PublicProfile username="alice" />)

    expect(screen.getByText('Get started')).toBeInTheDocument()
    expect(screen.getByText('Log a show you attended')).toBeInTheDocument()
    expect(screen.getByText('Follow artists you love')).toBeInTheDocument()
    expect(
      screen.getByText('Start your first collection')
    ).toBeInTheDocument()
    // The checklist REPLACES the content sections for a brand-new profile.
    expect(screen.queryByTestId('profile-following')).not.toBeInTheDocument()
    // And the visitor-facing empty card never shows to the owner.
    expect(
      screen.queryByText(/hasn't added any content/)
    ).not.toBeInTheDocument()
  })

  it('does NOT show the Get started checklist to visitors of an empty profile', () => {
    mockUsePublicProfile.mockReturnValue({
      // Visible-and-zero stats (the post-PSY-1045 default for anonymous
      // viewers): the empty-state card requires KNOWN-zero contributions.
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
      error: null,
    })

    renderWithProviders(<PublicProfile username="alice" />)
    expect(screen.queryByText('Get started')).not.toBeInTheDocument()
    expect(screen.getByText(/hasn't added any content/)).toBeInTheDocument()
  })

  it('does NOT claim emptiness to visitors when stats are hidden', () => {
    // With stats hidden we can't know whether the self-fetching sections
    // rendered content — so no "hasn't added any content" card.
    mockUsePublicProfile.mockReturnValue({
      data: makeProfile({ stats: undefined, stats_count: undefined }),
      isLoading: false,
      error: null,
    })

    renderWithProviders(<PublicProfile username="alice" />)
    expect(
      screen.queryByText(/hasn't added any content/)
    ).not.toBeInTheDocument()
  })

  it('shows the add-bio prompt to an owner without a bio (non-empty profile)', () => {
    mockUser = { username: 'alice' }
    mockUsePublicProfile.mockReturnValue({
      data: makeProfile({ username: 'alice' }),
      isLoading: false,
      error: null,
    })
    // Collections exist, so this is NOT a brand-new profile (no checklist).
    mockUseUserPublicCollections.mockReturnValue({
      data: { collections: [], total: 3 },
    })

    renderWithProviders(<PublicProfile username="alice" />)
    expect(screen.getByText(/Add a short bio/)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Add bio' })).toBeInTheDocument()
    expect(screen.queryByText('Get started')).not.toBeInTheDocument()
  })

  it('never shows the add-bio prompt to visitors', () => {
    mockUsePublicProfile.mockReturnValue({
      data: makeProfile(),
      isLoading: false,
      error: null,
    })
    mockUseUserPublicCollections.mockReturnValue({
      data: { collections: [], total: 3 },
    })

    renderWithProviders(<PublicProfile username="alice" />)
    expect(screen.queryByText(/Add a short bio/)).not.toBeInTheDocument()
  })
})

