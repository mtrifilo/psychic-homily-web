import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ChartDrilldownPage, drilldownHref } from './ChartDrilldownPage'
import type { ChartModuleSlug } from '../moduleConfig'

const mockSetWindow = vi.fn()
const mockSetScene = vi.fn()
const mockSetPage = vi.fn()
const mockChartHook = vi.fn()
const mockRefetchScenes = vi.fn()
let queryWindow: 'month' | 'quarter' | 'all_time' = 'quarter'
let queryScene: string | null = '38060'
let queryPage = 1
let sceneQueryError = false
let sceneQueryDataAvailable = true
let moduleQueryError = false
/** The module query is serving the PREVIOUS page while the next one loads. */
let modulePlaceholderData = false

function query<T>(data: T, enabled = true) {
  return {
    data: enabled ? data : undefined,
    isLoading: false,
    isError: false,
    isSuccess: enabled,
    isFetching: false,
    isPlaceholderData: false,
    refetch: vi.fn(),
  }
}

// The pager seeds its hrefs from the live query string so unknown params
// survive a page change; this stands in for the URL the reader arrived on.
let liveQuery = 'window=quarter&scene=38060'
vi.mock('next/navigation', () => ({
  useSearchParams: () => new URLSearchParams(liveQuery),
}))

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

// The inline action buttons are stubbed to keep their own suites' concerns out
// of this one; the pager is spliced back in from its module because this suite
// asserts the URLs it emits.
vi.mock('@/components/shared', async () => {
  const { Pagination } = await vi.importActual<
    typeof import('@/components/shared/Pagination')
  >('@/components/shared/Pagination')
  return {
    Pagination,
    FollowButton: ({ entityId }: { entityId: number }) => (
      <button>follow-{entityId}</button>
    ),
    SaveButton: ({ showId }: { showId: number }) => (
      <button>save-show-{showId}</button>
    ),
    ReleaseSaveButton: ({ releaseId }: { releaseId: number }) => (
      <button>save-release-{releaseId}</button>
    ),
  }
})

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
  return {
    ...query(payloads[slug], options.enabled),
    isError: moduleQueryError,
    isPlaceholderData: modulePlaceholderData,
    isFetching: modulePlaceholderData,
  }
}

vi.mock('../hooks', () => ({
  useChartScenes: () => {
    const data = {
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
        {
          metro: '46060',
          name: 'Tucson, AZ',
          city: 'Tucson',
          state: 'AZ',
          show_count: 17,
          artist_count: 19,
          venue_count: 8,
        },
      ],
    }
    return {
      ...query(data),
      data: sceneQueryDataAvailable ? data : undefined,
      isError: sceneQueryError,
      isSuccess: !sceneQueryError,
      refetch: mockRefetchScenes,
    }
  },
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
  useMostAnticipated: (
    _window: string,
    _limit: number,
    options: { enabled?: boolean }
  ) =>
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

describe('drilldownHref', () => {
  it('carries the window and scene filters through a page change', () => {
    expect(
      drilldownHref('busiest-venues', 'window=all_time&scene=38060', 'all_time', '38060', 4)
    ).toBe('/charts/busiest-venues?window=all_time&scene=38060&page=4')
  })

  it('drops the page param on page one so the head of the list has one URL', () => {
    expect(
      drilldownHref('busiest-venues', 'window=all_time&scene=38060&page=4', 'all_time', '38060', 1)
    ).toBe('/charts/busiest-venues?window=all_time&scene=38060')
  })

  it('omits the query string entirely when nothing is set', () => {
    expect(drilldownHref('new-releases', '', 'quarter', '', 1)).toBe(
      '/charts/new-releases'
    )
    expect(drilldownHref('new-releases', '', 'quarter', '', 2)).toBe(
      '/charts/new-releases?page=2'
    )
  })

  it('omits the default window so the head of the list has a single URL', () => {
    // `chartRankHref` links in with an explicit `?window=quarter`; echoing that
    // back would give page one two URLs for identical content.
    expect(
      drilldownHref('new-releases', 'window=quarter', 'quarter', '', 2)
    ).toBe('/charts/new-releases?page=2')
  })

  it('collapses an unrecognized window onto the chart that actually rendered', () => {
    // The caller passes the RESOLVED window, so `?window=banana` (which renders
    // the quarter chart) cannot multiply itself across every page link.
    expect(
      drilldownHref('new-releases', 'window=banana', 'quarter', '', 2)
    ).toBe('/charts/new-releases?page=2')
  })

  it('keeps an archive window, which is a real chart and not the default', () => {
    expect(
      drilldownHref('new-releases', 'window=2025-q1', '2025-q1', '', 2)
    ).toBe('/charts/new-releases?window=2025-q1&page=2')
  })

  it('preserves params it knows nothing about, as the nuqs writer did', () => {
    expect(
      drilldownHref(
        'new-releases',
        'utm_source=newsletter&window=month',
        'month',
        '',
        3
      )
    ).toBe('/charts/new-releases?utm_source=newsletter&window=month&page=3')
  })

  it('clears a scene that is no longer selected', () => {
    expect(
      drilldownHref('new-releases', 'scene=38060&page=2', 'quarter', '', 2)
    ).toBe('/charts/new-releases?page=2')
  })

  it('percent-encodes filter values instead of splicing them raw', () => {
    expect(drilldownHref('new-releases', '', 'all_time', 'a&b=c', 2)).toBe(
      '/charts/new-releases?window=all_time&scene=a%26b%3Dc&page=2'
    )
  })
})

describe('ChartDrilldownPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    queryWindow = 'quarter'
    queryScene = '38060'
    queryPage = 1
    liveQuery = 'window=quarter&scene=38060'
    sceneQueryError = false
    sceneQueryDataAvailable = true
    moduleQueryError = false
    modulePlaceholderData = false
    payloads['most-active-artists'].total = 120
    payloads['most-active-artists'].artists = [
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
    ]
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

    // The pager navigates by href, not by a nuqs write: `?page=` is set by the
    // link, and the scene filter rides along so a page change never silently
    // drops it. `window=quarter` is the default and is normalized away, so the
    // strip agrees with what the window tabs write.
    const pager = within(screen.getByTestId('pagination-desktop'))
    expect(pager.getByRole('link', { name: /Next/ })).toHaveAttribute(
      'href',
      '/charts/most-active-artists?scene=38060&page=3'
    )
    expect(pager.getByRole('link', { name: 'Page 1' })).toHaveAttribute(
      'href',
      '/charts/most-active-artists?scene=38060'
    )
    expect(pager.getByRole('link', { name: 'Page 2' })).toHaveAttribute(
      'aria-current',
      'page'
    )

    await user.click(screen.getByRole('button', { name: 'All Time' }))
    expect(mockSetPage).toHaveBeenCalledWith(null)
    expect(mockSetWindow).toHaveBeenCalledWith('all_time')
  })

  it('renders keyed row values beneath their configured headers', () => {
    render(<ChartDrilldownPage module="most-active-artists" />)

    const table = screen.getByRole('table')
    const headers = within(table)
      .getAllByRole('columnheader')
      .map(header => header.textContent)
    const row = screen.getByRole('link', { name: 'Glass Harbor' }).closest('tr')
    expect(row).not.toBeNull()
    const cells = within(row!).getAllByRole('cell')

    expect(cells[headers.indexOf('Shows')]).toHaveTextContent('9')
    expect(cells[headers.indexOf('Headline %')]).toHaveTextContent('80%')
    expect(cells[headers.indexOf('Last show')]).toHaveTextContent('Valley Bar')
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

  it('caps navigation at the backend offset boundary for larger totals', () => {
    queryPage = 201
    payloads['most-active-artists'].total = 20_000
    render(<ChartDrilldownPage module="most-active-artists" />)

    const pager = within(screen.getByTestId('pagination-desktop'))
    expect(pager.getByRole('link', { name: 'Page 201' })).toHaveAttribute(
      'aria-current',
      'page'
    )
    expect(
      pager.queryByRole('link', { name: 'Page 400' })
    ).not.toBeInTheDocument()
    // The last reachable page offers no onward link at all, so there is no href
    // pointing past the backend's offset cap.
    expect(
      within(screen.getByTestId('pagination-desktop')).getByTestId(
        'pagination-next-disabled'
      )
    ).toBeInTheDocument()
    expect(pager.queryByRole('link', { name: /Next/ })).not.toBeInTheDocument()
    expect(
      screen.getByText(/first 10,050 accessible/)
    ).toBeInTheDocument()
  })

  it('announces a page change to screen readers (PSY-1768)', () => {
    // The whole point of holding the previous page on screen: the results
    // region — pager included — stays MOUNTED, so the pager's live region is
    // already in the accessibility tree when its text changes. It used to be
    // torn down and rebuilt on every click, and a live region that mounts
    // already populated announces nothing.
    queryPage = 2
    payloads['most-active-artists'].total = 300
    const { rerender } = render(<ChartDrilldownPage module="most-active-artists" />)
    const announcement = screen.getByRole('status')
    expect(announcement).toBeEmptyDOMElement()

    queryPage = 3
    modulePlaceholderData = true
    rerender(<ChartDrilldownPage module="most-active-artists" />)

    expect(screen.getByRole('status')).toHaveTextContent('Page 3 of 6')
  })

  it('keeps the previous rows on screen, dimmed, while the next page loads', () => {
    queryPage = 2
    modulePlaceholderData = true
    render(<ChartDrilldownPage module="most-active-artists" />)

    // Rows, not skeletons.
    expect(
      screen.getByRole('link', { name: 'Glass Harbor' })
    ).toBeInTheDocument()
    expect(screen.getByRole('table').parentElement?.className).toContain(
      'opacity-60'
    )
    // The pager survives the change, which is what carries the announcement.
    expect(screen.getByTestId('pagination')).toBeInTheDocument()
  })

  it('elides the row range rather than stating a wrong one over stale rows', () => {
    // The range is computed from the NEW offset over the OLD rows, so it would
    // read "Showing 51–51" above page one's row. Dimming says stale; it does
    // not make a wrong number right.
    queryPage = 2
    modulePlaceholderData = true
    render(<ChartDrilldownPage module="most-active-artists" />)

    expect(screen.getByText('Showing … of 120')).toBeInTheDocument()
    expect(screen.queryByText('Showing 51–51 of 120')).not.toBeInTheDocument()
  })

  it('holds the table in loading while clamping a beyond-end page', () => {
    queryPage = 4
    payloads['most-active-artists'].total = 120
    render(<ChartDrilldownPage module="most-active-artists" />)

    expect(screen.queryByText('Showing 151–0 of 120')).not.toBeInTheDocument()
    expect(
      screen.queryByText('No qualifying rows in this window and scene.')
    ).not.toBeInTheDocument()
    expect(mockSetPage).toHaveBeenCalledWith(3)
  })

  it('suggests alternative scenes on a zero-result scene drilldown', async () => {
    const user = userEvent.setup()
    payloads['most-active-artists'].total = 0
    payloads['most-active-artists'].artists = []
    render(<ChartDrilldownPage module="most-active-artists" />)

    expect(
      screen.getByText('No qualifying rows in this window and scene.')
    ).toBeInTheDocument()
    expect(screen.getByTestId('chart-zero-result-suggestions')).toHaveTextContent(
      'Nothing charting in Phoenix this window — try Tucson.'
    )
    await user.click(screen.getByTestId('chart-suggest-scene-46060'))
    expect(mockSetScene).toHaveBeenCalledWith('46060')
  })

  it('paginates the unranked most-anticipated fallback without repeating page one', () => {
    queryPage = 2
    payloads['most-anticipated'].mode = 'soonest_upcoming'
    payloads['most-anticipated'].total = 75
    render(<ChartDrilldownPage module="most-anticipated" />)

    expect(mockChartHook).toHaveBeenCalledWith(
      'most-anticipated',
      expect.objectContaining({ offset: 50, enabled: true, scene: '38060' })
    )
    expect(screen.getByText('Showing 51–51 of 75')).toBeInTheDocument()
  })

  it('preserves an unverified scene and offers retry when discovery fails', async () => {
    const user = userEvent.setup()
    sceneQueryError = true
    sceneQueryDataAvailable = false
    render(<ChartDrilldownPage module="most-active-artists" />)

    expect(
      screen.getByText(
        'Unable to verify this scene. Your selection is preserved.'
      )
    ).toBeInTheDocument()
    expect(mockSetScene).not.toHaveBeenCalled()
    expect(mockChartHook).toHaveBeenCalledWith(
      'most-active-artists',
      expect.objectContaining({ enabled: false })
    )

    await user.click(screen.getByRole('button', { name: 'Try again' }))
    expect(mockRefetchScenes).toHaveBeenCalledOnce()
  })

  it('keeps cached rows visible when a background module refetch fails', () => {
    moduleQueryError = true
    render(<ChartDrilldownPage module="most-active-artists" />)

    expect(
      screen.getByRole('link', { name: 'Glass Harbor' })
    ).toBeInTheDocument()
    expect(
      screen.queryByText('Unable to load this chart.')
    ).not.toBeInTheDocument()
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
