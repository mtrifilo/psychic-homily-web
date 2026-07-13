import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ChartsPage } from './ChartsPage'

const mockSetWindow = vi.fn()
type EmptyQuery = {
  data: Record<string, never>
  isLoading: boolean
  isError: boolean
}
const mockBatchFollowStatus = vi.fn<(...args: unknown[]) => EmptyQuery>(() =>
  query({})
)
const mockShowSaveCountBatch = vi.fn<(...args: unknown[]) => EmptyQuery>(() =>
  query({})
)
const mockReleaseSaveCountBatch = vi.fn<(...args: unknown[]) => EmptyQuery>(
  () => query({})
)
let isAuthenticated = true
let anticipatedMode: 'ranked' | 'soonest_upcoming' = 'ranked'
let activeArtists = [
  {
    artist_id: 1,
    name: 'Glass Harbor',
    slug: 'glass-harbor',
    city: 'Phoenix',
    state: 'AZ',
    show_count: 9,
    headline_pct: 80,
    last_show_date: null,
    last_show_slug: '',
    last_show_venue: '',
    rank: 1,
  },
]

function query<T>(data: T) {
  return { data, isLoading: false, isError: false }
}

vi.mock('nuqs', () => ({
  parseAsStringLiteral: () => ({
    withDefault: () => ({ withOptions: () => ({}) }),
  }),
  useQueryState: () => ['quarter', mockSetWindow],
}))
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({ isAuthenticated }),
}))
vi.mock('@/lib/hooks/common/useFollow', () => ({
  useBatchFollowStatus: (entityType: string, ids: number[]) =>
    mockBatchFollowStatus(entityType, ids),
}))
vi.mock('@/features/shows', () => ({
  useShowSaveCountBatch: (ids: number[], authenticated: boolean) =>
    mockShowSaveCountBatch(ids, authenticated),
}))
vi.mock('@/features/releases', () => ({
  getReleaseTypeLabel: (type: string) => type.toUpperCase(),
  useReleaseSaveCountBatch: (ids: number[], authenticated: boolean) =>
    mockReleaseSaveCountBatch(ids, authenticated),
}))
vi.mock('@/components/shared', () => ({
  FollowButton: ({ entityId }: { entityId: number }) => (
    <button>follow-{entityId}</button>
  ),
  SaveButton: ({ showId }: { showId: number }) => (
    <button>save-show-{showId}</button>
  ),
  ReleaseSaveButton: ({ releaseId }: { releaseId: number }) => (
    <button>save-release-{releaseId}</button>
  ),
}))
vi.mock('../hooks', () => ({
  useMostActiveArtists: () => query({ artists: activeArtists }),
  useOnTheRadio: () =>
    query({
      artists: [
        {
          artist_id: 2,
          name: 'Static Bloom',
          slug: 'static-bloom',
          city: 'Tucson',
          state: 'AZ',
          play_count: 12,
          station_count: 2,
          is_new: true,
          rank: 1,
        },
      ],
    }),
  useMostAnticipated: () =>
    query({
      mode: anticipatedMode,
      shows: [
        {
          show_id: 3,
          title: 'Glass Harbor at Valley Bar',
          slug: 'glass-harbor-valley-bar',
          date: '2026-07-18T03:00:00Z',
          venue_name: 'Valley Bar',
          venue_slug: 'valley-bar',
          city: 'Phoenix',
          artist_names: ['Glass Harbor'],
          save_count: anticipatedMode === 'ranked' ? 7 : undefined,
          rank: anticipatedMode === 'ranked' ? 1 : undefined,
        },
      ],
    }),
  useBusiestVenues: () =>
    query({
      venues: [
        {
          venue_id: 4,
          name: 'Valley Bar',
          slug: 'valley-bar',
          city: 'Phoenix',
          state: 'AZ',
          show_count: 14,
          rank: 1,
        },
      ],
    }),
  useNewReleases: () =>
    query({
      releases: [
        {
          release_id: 5,
          title: 'Night Ledger',
          slug: 'night-ledger',
          release_type: 'lp',
          release_date: '2026-07-01',
          added_at: '2026-07-02T00:00:00Z',
          rank: 1,
          artists: [{ id: 1, name: 'Glass Harbor', slug: 'glass-harbor' }],
          labels: [{ id: 6, name: 'Desert Static', slug: 'desert-static' }],
        },
      ],
    }),
  useOpenersToWatch: () =>
    query({
      artists: [
        {
          artist_id: 7,
          name: 'Soft Exit',
          slug: 'soft-exit',
          city: 'Mesa',
          state: 'AZ',
          support_slot_count: 5,
          rank: 1,
        },
      ],
    }),
  useChartsSummary: () =>
    query({
      shows_added: 22,
      new_artists: 8,
      new_releases: 11,
      radio_plays: 47,
      active_scenes: 3,
    }),
  useFreshlyAdded: () =>
    query({
      items: [
        {
          entity_type: 'release',
          entity_id: 5,
          name: 'Night Ledger',
          slug: 'night-ledger',
          added_at: '2026-07-02T00:00:00Z',
        },
      ],
    }),
}))

describe('ChartsPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    isAuthenticated = true
    anticipatedMode = 'ranked'
    activeArtists = [
      {
        artist_id: 1,
        name: 'Glass Harbor',
        slug: 'glass-harbor',
        city: 'Phoenix',
        state: 'AZ',
        show_count: 9,
        headline_pct: 80,
        last_show_date: null,
        last_show_slug: '',
        last_show_venue: '',
        rank: 1,
      },
    ]
  })

  it('renders the six-module Broadsheet with inline actions and linked release metadata', () => {
    render(<ChartsPage />)

    expect(screen.getAllByRole('heading', { level: 2 })).toHaveLength(7)
    expect(screen.getByText('Hardest-Working Artists')).toBeInTheDocument()
    expect(screen.getByText('Openers to Watch')).toBeInTheDocument()
    expect(
      screen
        .getAllByRole('link', { name: 'Night Ledger' })
        .some(link => link.getAttribute('href') === '/releases/night-ledger')
    ).toBe(true)
    expect(
      screen
        .getAllByRole('link', { name: 'Glass Harbor' })
        .some(link => link.getAttribute('href') === '/artists/glass-harbor')
    ).toBe(true)
    expect(screen.getByRole('link', { name: 'Desert Static' })).toHaveAttribute(
      'href',
      '/labels/desert-static'
    )
    expect(
      screen.getByRole('button', { name: 'save-release-5' })
    ).toBeInTheDocument()
  })

  it('keeps the default quarter out of the URL and writes explicit alternate windows', async () => {
    const user = userEvent.setup()
    render(<ChartsPage />)

    await user.click(screen.getByRole('button', { name: 'Quarter' }))
    expect(mockSetWindow).toHaveBeenLastCalledWith(null)
    await user.click(screen.getByRole('button', { name: 'All Time' }))
    expect(mockSetWindow).toHaveBeenLastCalledWith('all_time')
  })

  it('renders the soonest-upcoming fallback without inventing a rank or save total', () => {
    anticipatedMode = 'soonest_upcoming'
    render(<ChartsPage />)

    const anticipatedModule = screen.getByTestId('chart-most-anticipated')
    expect(anticipatedModule).toHaveTextContent('—')
    expect(
      screen.queryByText('7', {
        selector: '[data-testid="chart-most-anticipated"] span',
      })
    ).not.toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'save-show-3' })
    ).toBeInTheDocument()
  })

  it('collapses a successful empty module', () => {
    activeArtists = []
    render(<ChartsPage />)
    expect(
      screen.queryByTestId('chart-most-active-artists')
    ).not.toBeInTheDocument()
  })

  it('falls back to entity ids for slugless chart rows', () => {
    activeArtists = [{ ...activeArtists[0], slug: '' }]
    render(<ChartsPage />)
    expect(
      screen
        .getAllByRole('link', { name: 'Glass Harbor' })
        .some(link => link.getAttribute('href') === '/artists/1')
    ).toBe(true)
  })

  it('does not request private action state for anonymous chart visitors', () => {
    isAuthenticated = false
    render(<ChartsPage />)

    expect(mockBatchFollowStatus).toHaveBeenCalledWith('artists', [])
    expect(mockBatchFollowStatus).toHaveBeenCalledWith('venues', [])
    expect(mockShowSaveCountBatch).toHaveBeenCalledWith([], false)
    expect(mockReleaseSaveCountBatch).toHaveBeenCalledWith([], false)
  })
})
