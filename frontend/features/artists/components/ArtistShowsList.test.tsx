import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import type { ArtistShow, ArtistTimeFilter } from '../types'

// Routes useArtistShows fetches by `timeFilter` so the test fixtures for
// upcoming vs past can be set independently.
const upcomingResult = {
  data: undefined as { shows: ArtistShow[]; total: number } | undefined,
  isLoading: false,
  error: null as Error | null,
}
const pastResult = {
  data: undefined as { shows: ArtistShow[]; total: number } | undefined,
  isLoading: false,
  error: null as Error | null,
}

vi.mock('../hooks/useArtists', () => ({
  useArtistShows: ({ timeFilter }: { timeFilter: ArtistTimeFilter }) =>
    timeFilter === 'past' ? pastResult : upcomingResult,
}))

// Identity passthrough — dedup behavior is exercised in features/shows/utils.test.ts.
vi.mock('@/features/shows', () => ({
  dedupArtistShows: <T,>(shows: T[]) => shows,
}))

// Lightweight shared primitive mocks — render plain elements with the props
// the tests care about. Real implementations are unit-tested in their own
// files; we just need inspectable wrappers here.
vi.mock('@/components/shared', () => ({
  BracketLink: ({
    label,
    onClick,
  }: {
    label: string
    onClick?: () => void
  }) => (
    <button data-testid={`bracket-${label}`} onClick={onClick}>
      [{label}]
    </button>
  ),
  SectionHeader: ({
    title,
    action,
  }: {
    title: string
    action?: React.ReactNode
  }) => (
    <div data-testid={`section-header-${title}`}>
      <h2>{title}</h2>
      {action}
    </div>
  ),
  DenseTable: ({
    children,
    'aria-label': ariaLabel,
  }: {
    children: React.ReactNode
    'aria-label'?: string
  }) => (
    <table data-testid={`densetable-${ariaLabel ?? 'unlabeled'}`} aria-label={ariaLabel}>
      {children}
    </table>
  ),
}))

vi.mock('@/features/notifications', () => ({
  NotifyMeButton: ({ entityName }: { entityName: string }) => (
    <button data-testid="notify-me-button">Notify me about {entityName}</button>
  ),
}))

import { ArtistShowsList } from './ArtistShowsList'

function makeShow(overrides: Partial<ArtistShow> = {}): ArtistShow {
  return {
    id: 1,
    slug: 'test-show',
    title: 'Test Show',
    event_date: '2025-06-15T20:00:00Z',
    price: 15,
    age_requirement: null,
    venue: {
      id: 1,
      slug: 'test-venue',
      name: 'Test Venue',
      city: 'Phoenix',
      state: 'AZ',
    },
    artists: [
      { id: 42, slug: 'main-artist', name: 'Main Artist' },
      { id: 99, slug: 'opener', name: 'The Opener' },
    ],
    ...overrides,
  }
}

function setUpcoming(data: { shows: ArtistShow[]; total?: number } | null, opts?: { isLoading?: boolean; error?: Error | null }) {
  upcomingResult.data = data ? { shows: data.shows, total: data.total ?? data.shows.length } : undefined
  upcomingResult.isLoading = opts?.isLoading ?? false
  upcomingResult.error = opts?.error ?? null
}

function setPast(data: { shows: ArtistShow[]; total?: number } | null, opts?: { isLoading?: boolean; error?: Error | null }) {
  pastResult.data = data ? { shows: data.shows, total: data.total ?? data.shows.length } : undefined
  pastResult.isLoading = opts?.isLoading ?? false
  pastResult.error = opts?.error ?? null
}

describe('ArtistShowsList — upcoming section', () => {
  beforeEach(() => {
    setUpcoming(null)
    setPast(null)
  })

  it('renders the Upcoming shows section header always', () => {
    renderWithProviders(<ArtistShowsList artistId={42} artistName="Test Artist" />)
    expect(screen.getByTestId('section-header-Upcoming shows')).toBeInTheDocument()
  })

  it('shows a loader while upcoming shows are loading', () => {
    setUpcoming(null, { isLoading: true })
    renderWithProviders(<ArtistShowsList artistId={42} artistName="Test Artist" />)
    expect(document.querySelector('.animate-spin')).toBeInTheDocument()
  })

  it('shows an error message when the upcoming fetch fails', () => {
    setUpcoming(null, { error: new Error('boom') })
    renderWithProviders(<ArtistShowsList artistId={42} artistName="Test Artist" />)
    expect(screen.getByText(/Failed to load shows/i)).toBeInTheDocument()
  })

  it('renders an inline [Notify me] affordance when there are no upcoming shows', () => {
    setUpcoming({ shows: [] })
    renderWithProviders(<ArtistShowsList artistId={42} artistName="Just Mustard" />)
    expect(screen.getByText(/No upcoming shows yet/i)).toBeInTheDocument()
    expect(screen.getByTestId('notify-me-button')).toHaveTextContent('Just Mustard')
  })

  it('renders upcoming shows as a DenseTable when data is present', () => {
    setUpcoming({ shows: [makeShow()] })
    renderWithProviders(<ArtistShowsList artistId={42} artistName="Test Artist" />)
    expect(screen.getByTestId('densetable-Upcoming shows')).toBeInTheDocument()
    expect(screen.getByText('Test Venue')).toBeInTheDocument()
  })

  it('shows the "Showing N of M" hint when results are truncated', () => {
    setUpcoming({ shows: [makeShow()], total: 30 })
    renderWithProviders(<ArtistShowsList artistId={42} artistName="Test Artist" />)
    expect(screen.getByText(/Showing 1 of 30 shows/)).toBeInTheDocument()
  })

  it('filters the current artist out of the Bill column', () => {
    setUpcoming({ shows: [makeShow()] })
    renderWithProviders(<ArtistShowsList artistId={42} artistName="Main Artist" />)
    // Bill cell should mention The Opener but NOT Main Artist (artistId=42).
    expect(screen.getByText('The Opener')).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: 'Main Artist' })).not.toBeInTheDocument()
  })
})

describe('ArtistShowsList — past section', () => {
  beforeEach(() => {
    setUpcoming(null)
    setPast(null)
  })

  it('omits the Past shows section entirely when there are 0 past shows', () => {
    setPast({ shows: [] })
    renderWithProviders(<ArtistShowsList artistId={42} artistName="Test Artist" />)
    expect(screen.queryByTestId('section-header-Past shows')).not.toBeInTheDocument()
  })

  it('renders the Past shows section header with a [Show] toggle when past shows exist', () => {
    setPast({ shows: [makeShow({ id: 5, title: 'Past Show' })] })
    renderWithProviders(<ArtistShowsList artistId={42} artistName="Test Artist" />)
    expect(screen.getByTestId('section-header-Past shows')).toBeInTheDocument()
    expect(screen.getByTestId('bracket-Show')).toBeInTheDocument()
  })

  it('past shows body is collapsed by default', () => {
    setPast({ shows: [makeShow({ id: 5, title: 'Past Show' })] })
    renderWithProviders(<ArtistShowsList artistId={42} artistName="Test Artist" />)
    expect(screen.queryByTestId('densetable-Past shows')).not.toBeInTheDocument()
  })

  it('expands the past shows body when [Show] is clicked', async () => {
    const user = userEvent.setup()
    setPast({ shows: [makeShow({ id: 5, title: 'Past Show' })] })
    renderWithProviders(<ArtistShowsList artistId={42} artistName="Test Artist" />)
    await user.click(screen.getByTestId('bracket-Show'))
    expect(screen.getByTestId('densetable-Past shows')).toBeInTheDocument()
    // Toggle label flips to [Hide].
    expect(screen.getByTestId('bracket-Hide')).toBeInTheDocument()
  })

  it('collapses again when [Hide] is clicked', async () => {
    const user = userEvent.setup()
    setPast({ shows: [makeShow({ id: 5, title: 'Past Show' })] })
    renderWithProviders(<ArtistShowsList artistId={42} artistName="Test Artist" />)
    await user.click(screen.getByTestId('bracket-Show'))
    await user.click(screen.getByTestId('bracket-Hide'))
    expect(screen.queryByTestId('densetable-Past shows')).not.toBeInTheDocument()
  })
})
