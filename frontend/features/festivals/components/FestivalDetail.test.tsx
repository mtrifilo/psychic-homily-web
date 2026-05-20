import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'
import type { FestivalDetail as FestivalDetailType } from '../types'

// next/link
vi.mock('next/link', () => ({
  default: ({
    href,
    children,
    ...rest
  }: {
    href: string
    children: React.ReactNode
  }) => (
    <a href={href} {...rest}>
      {children}
    </a>
  ),
}))

// Data hooks (festival detail + its lineup/venue/series companions)
const mockUseFestival = vi.fn()
const mockUseFestivalArtists = vi.fn()
const mockUseFestivalVenues = vi.fn()
const mockUseFestivals = vi.fn()
vi.mock('../hooks/useFestivals', () => ({
  useFestival: (opts: unknown) => mockUseFestival(opts),
  useFestivalArtists: (opts: unknown) => mockUseFestivalArtists(opts),
  useFestivalVenues: (opts: unknown) => mockUseFestivalVenues(opts),
  useFestivals: (opts: unknown) => mockUseFestivals(opts),
}))

const mockUseIsAuthenticated = vi.fn()
vi.mock('@/features/auth', () => ({
  useIsAuthenticated: () => mockUseIsAuthenticated(),
}))

// Heavy intelligence children get their own coverage; stub here to stay focused.
vi.mock('./FestivalLineup', () => ({
  FestivalLineup: ({ artists }: { artists: unknown[] }) => (
    <div data-testid="festival-lineup">Lineup ({artists.length})</div>
  ),
}))
vi.mock('./SimilarFestivals', () => ({
  SimilarFestivals: () => <div data-testid="similar-festivals" />,
}))
vi.mock('./RisingArtists', () => ({
  RisingArtists: () => <div data-testid="rising-artists" />,
}))
vi.mock('./SeriesHistory', () => ({
  SeriesHistory: () => <div data-testid="series-history" />,
}))

vi.mock('@/features/collections', () => ({
  EntityCollections: () => <div data-testid="entity-collections" />,
}))

vi.mock('@/features/contributions', () => ({
  EntityEditDrawer: ({ open }: { open: boolean }) =>
    open ? <div data-testid="edit-drawer">Edit Drawer</div> : null,
  EntitySaveSuccessBanner: ({ visible }: { visible: boolean }) =>
    visible ? <div data-testid="save-banner">Saved</div> : null,
  useEntitySaveSuccessBanner: () => ({
    isVisible: false,
    handleSaveSuccess: vi.fn(),
  }),
  AttributionLine: () => null,
  ReportEntityDialog: ({ open }: { open: boolean }) =>
    open ? <div data-testid="report-dialog" /> : null,
  ContributionPrompt: () => null,
}))

vi.mock('@/features/comments', () => ({
  CommentThread: () => <div data-testid="comment-thread" />,
}))

vi.mock('@/features/tags', () => ({
  EntityTagList: () => <div data-testid="entity-tag-list" />,
  AddTagDialog: () => null,
}))

vi.mock('@/components/shared', () => ({
  EntityDetailLayout: ({
    children,
    sidebar,
    header,
    fallback,
  }: {
    children: React.ReactNode
    sidebar: React.ReactNode
    header: React.ReactNode
    fallback: { href: string; label: string }
  }) => (
    <div data-testid="entity-layout">
      <a href={fallback.href}>{fallback.label}</a>
      <div data-testid="header-slot">{header}</div>
      <div data-testid="sidebar-slot">{sidebar}</div>
      <div data-testid="content-slot">{children}</div>
    </div>
  ),
  EntityHeader: ({
    title,
    subtitle,
    actions,
  }: {
    title: string
    subtitle?: React.ReactNode
    actions?: React.ReactNode
  }) => (
    <div data-testid="entity-header">
      <h1>{title}</h1>
      {subtitle && <div data-testid="subtitle">{subtitle}</div>}
      {actions && <div data-testid="header-actions">{actions}</div>}
    </div>
  ),
  SocialLinks: () => <div data-testid="social-links" />,
  FollowButton: () => <button data-testid="follow-button">Follow</button>,
  AddToCollectionButton: () => (
    <button data-testid="add-to-collection">Add</button>
  ),
  RevisionHistory: () => <div data-testid="revision-history" />,
  BracketLink: ({ label, onClick }: { label: string; onClick?: () => void }) => (
    <button onClick={onClick} data-testid={`bracket-${label}`}>
      [{label}]
    </button>
  ),
  SectionHeader: ({ title }: { title: string }) => <h3>{title}</h3>,
  StatsList: ({ items }: { items: { label: string; value: React.ReactNode }[] }) => (
    <dl data-testid="stats-list">
      {items.map((i) => (
        <div key={i.label}>
          <dt>{i.label}</dt>
          <dd>{i.value}</dd>
        </div>
      ))}
    </dl>
  ),
}))

import { FestivalDetail } from './FestivalDetail'

function makeFestival(
  overrides: Partial<FestivalDetailType> = {}
): FestivalDetailType {
  return {
    id: 1,
    name: 'FORM Arcosanti',
    slug: 'form-arcosanti-2025',
    series_slug: 'form-arcosanti',
    edition_year: 2025,
    description: null,
    location_name: 'Arcosanti',
    city: 'Mayer',
    state: 'AZ',
    country: 'US',
    start_date: '2025-05-09',
    end_date: '2025-05-11',
    website: null,
    ticket_url: null,
    flyer_url: null,
    status: 'confirmed',
    social: null,
    artist_count: 12,
    venue_count: 1,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('FestivalDetail', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseIsAuthenticated.mockReturnValue({
      user: null,
      isAuthenticated: false,
      isLoading: false,
    })
    mockUseFestivalArtists.mockReturnValue({ data: { artists: [] }, isLoading: false })
    mockUseFestivalVenues.mockReturnValue({ data: { venues: [] }, isLoading: false })
    mockUseFestivals.mockReturnValue({ data: { festivals: [] } })
  })

  it('shows a loading spinner while the festival is fetching', () => {
    mockUseFestival.mockReturnValue({ data: undefined, isLoading: true, error: null })
    renderWithProviders(<FestivalDetail idOrSlug="form-arcosanti" />)
    expect(document.querySelector('.animate-spin')).toBeInTheDocument()
  })

  it('renders a 404 message for a not-found error', () => {
    mockUseFestival.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error('festival not found'),
    })
    renderWithProviders(<FestivalDetail idOrSlug="missing" />)
    expect(screen.getByText('Festival Not Found')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Back to Festivals' })).toHaveAttribute(
      'href',
      '/festivals'
    )
  })

  it('renders a generic error message for non-404 errors', () => {
    mockUseFestival.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error('server exploded'),
    })
    renderWithProviders(<FestivalDetail idOrSlug="boom" />)
    expect(screen.getByText('Error Loading Festival')).toBeInTheDocument()
    expect(screen.getByText('server exploded')).toBeInTheDocument()
  })

  it('renders the header, stats, and lineup when the festival loads', () => {
    mockUseFestival.mockReturnValue({
      data: makeFestival(),
      isLoading: false,
      error: null,
    })
    mockUseFestivalArtists.mockReturnValue({
      data: { artists: [{ id: 1, day_date: null }] },
      isLoading: false,
    })
    renderWithProviders(<FestivalDetail idOrSlug="form-arcosanti" />)

    expect(
      screen.getByRole('heading', { name: 'FORM Arcosanti' })
    ).toBeInTheDocument()
    expect(screen.getByText('Confirmed')).toBeInTheDocument()
    expect(screen.getByText('May 9–11, 2025')).toBeInTheDocument()
    expect(screen.getByTestId('festival-lineup')).toHaveTextContent('Lineup (1)')
  })

  it('renders the website and ticket links when present', () => {
    mockUseFestival.mockReturnValue({
      data: makeFestival({
        website: 'https://form.com',
        ticket_url: 'https://tickets.com',
      }),
      isLoading: false,
      error: null,
    })
    renderWithProviders(<FestivalDetail idOrSlug="form-arcosanti" />)

    expect(
      screen.getByRole('link', { name: 'Official Website' })
    ).toHaveAttribute('href', 'https://form.com')
    expect(screen.getByRole('link', { name: 'Buy Tickets' })).toHaveAttribute(
      'href',
      'https://tickets.com'
    )
  })

  it('renders the venues section with venue links', () => {
    mockUseFestival.mockReturnValue({
      data: makeFestival(),
      isLoading: false,
      error: null,
    })
    mockUseFestivalVenues.mockReturnValue({
      data: {
        venues: [
          {
            id: 1,
            venue_id: 5,
            venue_name: 'The Venue',
            venue_slug: 'the-venue',
            city: 'Phoenix',
            state: 'AZ',
            is_primary: true,
          },
        ],
      },
      isLoading: false,
    })
    renderWithProviders(<FestivalDetail idOrSlug="form-arcosanti" />)

    expect(screen.getByRole('heading', { name: 'Venues' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'The Venue' })).toHaveAttribute(
      'href',
      '/venues/the-venue'
    )
    expect(screen.getByText('Primary')).toBeInTheDocument()
  })

  it('hides contribution affordances for anonymous visitors', () => {
    mockUseFestival.mockReturnValue({
      data: makeFestival(),
      isLoading: false,
      error: null,
    })
    renderWithProviders(<FestivalDetail idOrSlug="form-arcosanti" />)
    expect(screen.queryByTestId('bracket-Report')).not.toBeInTheDocument()
    expect(screen.queryByTestId('bracket-Edit')).not.toBeInTheDocument()
  })

  it('shows an Edit affordance for trusted contributors', () => {
    mockUseIsAuthenticated.mockReturnValue({
      user: { is_admin: false, user_tier: 'trusted_contributor' },
      isAuthenticated: true,
      isLoading: false,
    })
    mockUseFestival.mockReturnValue({
      data: makeFestival(),
      isLoading: false,
      error: null,
    })
    renderWithProviders(<FestivalDetail idOrSlug="form-arcosanti" />)
    expect(screen.getByTestId('bracket-Edit')).toBeInTheDocument()
  })
})
