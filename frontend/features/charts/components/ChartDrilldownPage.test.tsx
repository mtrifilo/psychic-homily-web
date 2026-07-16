import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ChartDrilldownPage } from './ChartDrilldownPage'
import type { ChartModuleSlug } from '../moduleConfig'

const mockSetWindow = vi.fn()
const mockSetScene = vi.fn()
const mockSetPage = vi.fn()
const mockChartHook = vi.fn()
let queryWindow: 'month' | 'quarter' | 'all_time' = 'quarter'
let queryScene: string | null = '38060'
let queryPage = 1

function query<T>(data: T, enabled = true) {
  return {
    data: enabled ? data : undefined,
    isLoading: false,
    isError: false,
    isSuccess: enabled,
    isFetching: false,
  }
}

vi.mock('nuqs', () => ({
  parseAsInteger: { withDefault: () => ({ withOptions: () => ({}) }) },
  parseAsString: { withOptions: () => ({}) },
  parseAsStringLiteral: () => ({
    withDefault: () => ({ withOptions: () => ({}) }),
  }),
  useQueryState: (key: string) => {
    if (key === 'window') return [queryWindow, mockSetWindow]
    if (key === 'scene') return [queryScene, mockSetScene]
    return [queryPage, mockSetPage]
  },
}))

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({
    isAuthenticated: true,
    user: { id: '42' },
  }),
}))

vi.mock('@/lib/hooks/common/useFollow', () => ({
  useBatchFollowStatus: () => query({}),
}))

vi.mock('@/features/shows', () => ({
  useShowSaveCountBatch: () => query({}),
}))

vi.mock('@/features/releases', () => ({
  getReleaseTypeLabel: (value: string) => value.toUpperCase(),
  useReleaseSaveCountBatch: () => query({}),
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

const payloads = {
  'most-active-artists': {
    window: 'quarter',
    scene: '38060',
    total: 120,
    artists: [
      {
        artist_id: 1,
        name: 'Glass Harbor',
        slug: 'glass-harbor',
        city: 'Phoenix',
        state: 'AZ',
        show_count: 9,
        headline_pct: 80,
        last_show_date: '2026-07-02T00:00:00Z',
        last_show_slug: 'glass-harbor-valley-bar',
        last_show_venue: 'Valley Bar',
        rank: 51,
      },
    ],
  },
  'on-the-radio': {
    window: 'quarter',
    scene: '38060',
    total: 1,
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
  },
  'most-anticipated': {
    mode: 'ranked',
    scene: '38060',
    total: 1,
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
        save_count: 7,
        rank: 1,
      },
    ],
  },
  'busiest-venues': {
    window: 'quarter',
    scene: '38060',
    total: 1,
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
  },
  'new-releases': {
    window: 'quarter',
    scene: '38060',
    total: 1,
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
  },
  'openers-to-watch': {
    window: 'quarter',
    scene: '38060',
    total: 1,
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
  },
}

function moduleQuery(slug: ChartModuleSlug, options: { enabled?: boolean }) {
  mockChartHook(slug, options)
  return query(payloads[slug], options.enabled)
}

vi.mock('../hooks', () => ({
  useChartScenes: () =>
    query({
      window: queryWindow,
      scenes: [
        {
          metro: '38060',
          name: 'Phoenix-Mesa-Chandler, AZ',
          city: 'Phoenix',
          state: 'AZ',
          show_count: 42,
          artist_count: 41,
          venue_count: 12,
        },
      ],
    }),
  useMostActiveArtists: (
    _window: string,
    _limit: number,
    options: { enabled?: boolean }
  ) => moduleQuery('most-active-artists', options),
  useOnTheRadio: (
    _window: string,
    _limit: number,
    options: { enabled?: boolean }
  ) => moduleQuery('on-the-radio', options),
  useMostAnticipated: (_limit: number, options: { enabled?: boolean }) =>
    moduleQuery('most-anticipated', options),
  useBusiestVenues: (
    _window: string,
    _limit: number,
    options: { enabled?: boolean }
  ) => moduleQuery('busiest-venues', options),
  useNewReleases: (
    _window: string,
    _limit: number,
    options: { enabled?: boolean }
  ) => moduleQuery('new-releases', options),
  useOpenersToWatch: (
    _window: string,
    _limit: number,
    options: { enabled?: boolean }
  ) => moduleQuery('openers-to-watch', options),
}))

describe('ChartDrilldownPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    queryWindow = 'quarter'
    queryScene = '38060'
    queryPage = 1
  })

  it('derives page offset, renders stable server ranks, and updates URL pagination', async () => {
    const user = userEvent.setup()
    queryPage = 2
    render(<ChartDrilldownPage module="most-active-artists" />)

    expect(
      screen.getByText('120 qualifying artists · Phoenix scene')
    ).toBeInTheDocument()
    expect(screen.getByText('51')).toBeInTheDocument()
    expect(screen.getByText('Showing 51–51 of 120')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Glass Harbor' })).toHaveAttribute(
      'href',
      '/artists/glass-harbor'
    )
    expect(screen.getByRole('link', { name: /Valley Bar/ })).toHaveAttribute(
      'href',
      '/shows/glass-harbor-valley-bar'
    )
    expect(mockChartHook).toHaveBeenCalledWith(
      'most-active-artists',
      expect.objectContaining({ offset: 50, enabled: true, scene: '38060' })
    )

    await user.click(screen.getByRole('button', { name: 'Next' }))
    expect(mockSetPage).toHaveBeenCalledWith(3)
    await user.click(screen.getByRole('button', { name: 'All Time' }))
    expect(mockSetPage).toHaveBeenCalledWith(null)
    expect(mockSetWindow).toHaveBeenCalledWith('all_time')
  })

  it('clamps URL pages to the backend offset boundary before querying', () => {
    queryPage = 999
    render(<ChartDrilldownPage module="most-active-artists" />)

    expect(mockSetPage).toHaveBeenCalledWith(201)
    expect(mockChartHook).toHaveBeenCalledWith(
      'most-active-artists',
      expect.objectContaining({ offset: 10_000, enabled: true })
    )
  })

  it.each([
    ['most-active-artists', 'follow-1'],
    ['on-the-radio', 'follow-2'],
    ['most-anticipated', 'save-show-3'],
    ['busiest-venues', 'follow-4'],
    ['new-releases', 'save-release-5'],
    ['openers-to-watch', 'follow-7'],
  ] as const)('renders the current inline action for %s', (module, action) => {
    render(<ChartDrilldownPage module={module} />)

    expect(screen.getByRole('button', { name: action })).toBeInTheDocument()
  })

  it('links release, artist, and label references from the payload', () => {
    render(<ChartDrilldownPage module="new-releases" />)

    expect(screen.getByRole('link', { name: 'Night Ledger' })).toHaveAttribute(
      'href',
      '/releases/night-ledger'
    )
    expect(screen.getByRole('link', { name: 'Glass Harbor' })).toHaveAttribute(
      'href',
      '/artists/glass-harbor'
    )
    expect(screen.getByRole('link', { name: 'Desert Static' })).toHaveAttribute(
      'href',
      '/labels/desert-static'
    )
  })

  it('links the anticipated show and venue references', () => {
    render(<ChartDrilldownPage module="most-anticipated" />)

    expect(
      screen.getByRole('link', { name: /Glass Harbor at Valley Bar/ })
    ).toHaveAttribute('href', '/shows/glass-harbor-valley-bar')
    const venueLink = screen
      .getAllByRole('link', { name: /Valley Bar/ })
      .find(link => link.getAttribute('href') === '/venues/valley-bar')
    expect(venueLink).toHaveAttribute('href', '/venues/valley-bar')
  })
})
