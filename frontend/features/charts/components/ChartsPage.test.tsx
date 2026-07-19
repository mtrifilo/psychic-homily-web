import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ChartsPage } from './ChartsPage'

const mockSetWindow = vi.fn()
const mockSetScene = vi.fn()
const mockRefetchScenes = vi.fn()
const mockScopedHook = vi.fn()
type EmptyQuery = {
  data: Record<string, never>
  isLoading: boolean
  isError: boolean
  isSuccess: boolean
  isFetching: boolean
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
let chartDefaults: {
  window: 'month' | 'quarter' | 'all_time'
  scene: string | null
} | null = null
let personalStatsLoading = false
let personalStatsError = false
let personalStats = {
  saved_shows: 12,
  artists_followed: 34,
  top_venue: {
    venue_id: 9,
    name: 'The Rebel Lounge',
    slug: 'the-rebel-lounge',
    saved_show_count: 5,
  } as {
    venue_id: number
    name: string
    slug: string
    saved_show_count: number
  } | null,
  first_activity_at: '2026-03-12T00:00:00Z' as string | null,
}
let queryWindow: 'month' | 'quarter' | 'all_time' | null = null
let queryScene: string | null = null
let sceneListLoading = false
let sceneListError = false
let sceneListFetching = false
let sceneListRefetchError = false
let anticipatedMode: 'ranked' | 'soonest_upcoming' = 'ranked'
let emptyAllModules = false
let chartScenes = [
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
]
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
  return {
    data,
    isLoading: false,
    isError: false,
    isSuccess: true,
    isFetching: false,
  }
}

vi.mock('nuqs', () => ({
  parseAsString: { withOptions: () => ({}) },
  parseAsStringLiteral: () => ({
    withOptions: () => ({}),
    withDefault: () => ({ withOptions: () => ({}) }),
  }),
  useQueryState: (key: string) =>
    key === 'window'
      ? [queryWindow, mockSetWindow]
      : [queryScene, mockSetScene],
}))
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({
    isAuthenticated,
    isLoading: false,
    user: isAuthenticated ? { id: '42' } : null,
  }),
}))
vi.mock('@/features/contributions', () => ({
  useContributeOpportunities: () => ({
    data: undefined,
    isLoading: false,
    isError: false,
  }),
  FOLLOWED_LOOSE_ENDS_KEY: 'followed_artists_missing_links',
}))
vi.mock('@/features/auth', () => ({
  useProfile: () => ({
    data: isAuthenticated
      ? {
          user: {
            preferences: {
              chart_defaults: chartDefaults,
            },
          },
        }
      : undefined,
  }),
  useSetChartDefaults: () => ({
    mutate: vi.fn(),
    isPending: false,
  }),
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
  usePersonalChartsStats: (_userId: string | undefined, enabled: boolean) => {
    if (!enabled) {
      return {
        data: undefined,
        isLoading: false,
        isError: false,
        isSuccess: false,
        isFetching: false,
      }
    }
    if (personalStatsLoading) {
      return {
        data: undefined,
        isLoading: true,
        isError: false,
        isSuccess: false,
        isFetching: true,
      }
    }
    if (personalStatsError) {
      return {
        data: undefined,
        isLoading: false,
        isError: true,
        isSuccess: false,
        isFetching: false,
      }
    }
    return query(personalStats)
  },
  useChartScenes: (window: string) => {
    mockScopedHook('scenes', window)
    if (sceneListLoading) {
      return {
        data: undefined,
        isLoading: true,
        isError: false,
        isSuccess: false,
        isFetching: true,
        refetch: mockRefetchScenes,
      }
    }
    if (sceneListError) {
      return {
        data: undefined,
        isLoading: false,
        isError: true,
        isSuccess: false,
        isFetching: false,
        refetch: mockRefetchScenes,
      }
    }
    const response = {
      ...query({ window, scenes: chartScenes }),
      isFetching: sceneListFetching,
    }
    if (sceneListRefetchError) {
      return { ...response, isError: true, isSuccess: false }
    }
    return response
  },
  useMostActiveArtists: (...args: unknown[]) => {
    mockScopedHook('active', ...args)
    return query({ artists: emptyAllModules ? [] : activeArtists })
  },
  useOnTheRadio: (...args: unknown[]) => {
    mockScopedHook('radio', ...args)
    return query({
      artists: emptyAllModules
        ? []
        : [
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
    })
  },
  useMostAnticipated: (...args: unknown[]) => {
    mockScopedHook('anticipated', ...args)
    return query({
      mode: anticipatedMode,
      shows: emptyAllModules
        ? []
        : [
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
    })
  },
  useBusiestVenues: (...args: unknown[]) => {
    mockScopedHook('venues', ...args)
    return query({
      venues: emptyAllModules
        ? []
        : [
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
    })
  },
  useNewReleases: (...args: unknown[]) => {
    mockScopedHook('releases', ...args)
    return query({
      releases: emptyAllModules
        ? []
        : [
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
    })
  },
  useOpenersToWatch: (...args: unknown[]) => {
    mockScopedHook('openers', ...args)
    return query({
      artists: emptyAllModules
        ? []
        : [
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
    })
  },
  useTopTags: (...args: unknown[]) => {
    mockScopedHook('tags', ...args)
    return query({
      tags: emptyAllModules
        ? []
        : [
            {
              tag_id: 8,
              name: 'Shoegaze',
              slug: 'shoegaze',
              category: 'genre',
              weighted_saves: 14,
              show_count: 4,
              rank: 1,
            },
          ],
    })
  },
  useChartsSummary: (...args: unknown[]) => {
    mockScopedHook('summary', ...args)
    return query({
      shows_added: 22,
      new_artists: 8,
      new_releases: 11,
      radio_plays: 47,
      active_scenes: 3,
    })
  },
  useFreshlyAdded: (...args: unknown[]) => {
    mockScopedHook('freshly', ...args)
    return query({
      items: [
        {
          entity_type: 'release',
          entity_id: 5,
          name: 'Night Ledger',
          slug: 'night-ledger',
          added_at: '2026-07-02T00:00:00Z',
        },
      ],
    })
  },
}))

describe('ChartsPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    isAuthenticated = true
    chartDefaults = null
    personalStatsLoading = false
    personalStatsError = false
    personalStats = {
      saved_shows: 12,
      artists_followed: 34,
      top_venue: {
        venue_id: 9,
        name: 'The Rebel Lounge',
        slug: 'the-rebel-lounge',
        saved_show_count: 5,
      },
      first_activity_at: '2026-03-12T00:00:00Z',
    }
    queryWindow = null
    queryScene = null
    sceneListLoading = false
    sceneListError = false
    sceneListFetching = false
    sceneListRefetchError = false
    anticipatedMode = 'ranked'
    emptyAllModules = false
    chartScenes = [
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
    ]
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

  it('renders the seven-module Broadsheet with inline actions and linked release metadata', () => {
    render(<ChartsPage />)

    expect(screen.getAllByRole('heading', { level: 2 })).toHaveLength(8)
    expect(screen.getByText('Hardest-Working Artists')).toBeInTheDocument()
    expect(screen.getByText('Openers to Watch')).toBeInTheDocument()
    expect(screen.getByText('Top Tags')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Shoegaze' })).toHaveAttribute(
      'href',
      '/shows?tags=shoegaze&cities=all'
    )
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

  it('renders the logged-in personal stats strip without the deferred year link', () => {
    render(<ChartsPage />)

    const strip = screen.getByRole('region', { name: 'Your chart stats' })
    expect(strip).toHaveTextContent('YOU')
    expect(strip).toHaveTextContent('12 shows marked')
    expect(strip).toHaveTextContent('34 artists followed')
    expect(strip).toHaveTextContent('top venue: The Rebel Lounge (5)')
    expect(strip).toHaveTextContent('first logged: Mar 2026')
    expect(screen.queryByText(/Your year in shows/i)).not.toBeInTheDocument()
  })

  it('renders no personal-stats trace for an anonymous visitor', () => {
    isAuthenticated = false
    render(<ChartsPage />)

    expect(
      screen.queryByRole('region', { name: 'Your chart stats' })
    ).not.toBeInTheDocument()
    expect(screen.queryByText('YOU')).not.toBeInTheDocument()
  })

  it('renders a labeled personal-stats skeleton while the request loads', () => {
    personalStatsLoading = true
    render(<ChartsPage />)

    expect(
      screen.getByRole('region', { name: 'Your chart stats' })
    ).toBeInTheDocument()
    expect(
      screen.getByLabelText('Loading your chart stats')
    ).toBeInTheDocument()
  })

  it('renders no personal-stats trace when the optional request fails', () => {
    personalStatsError = true
    render(<ChartsPage />)

    expect(
      screen.queryByRole('region', { name: 'Your chart stats' })
    ).not.toBeInTheDocument()
    expect(screen.queryByText('YOU')).not.toBeInTheDocument()
  })

  it('replaces zero-history stats with the actionable nudge', () => {
    personalStats = {
      saved_shows: 0,
      artists_followed: 0,
      top_venue: null,
      first_activity_at: null,
    }
    render(<ChartsPage />)

    const strip = screen.getByRole('region', { name: 'Your chart stats' })
    expect(strip).toHaveTextContent(
      "Mark shows you're going to and this fills in"
    )
    expect(strip).not.toHaveTextContent('0 shows marked')
  })

  it('shows first activity when history exists outside the counted facts', () => {
    personalStats = {
      saved_shows: 0,
      artists_followed: 0,
      top_venue: null,
      first_activity_at: '2026-03-12T00:00:00Z',
    }
    render(<ChartsPage />)

    const strip = screen.getByRole('region', { name: 'Your chart stats' })
    expect(strip).toHaveTextContent('0 shows marked')
    expect(strip).toHaveTextContent('0 artists followed')
    expect(strip).toHaveTextContent('first logged: Mar 2026')
    expect(strip).not.toHaveTextContent("Mark shows you're going to")
  })

  it('keeps the default quarter out of the URL and writes explicit alternate windows', async () => {
    const user = userEvent.setup()
    render(<ChartsPage />)

    await user.click(screen.getByRole('button', { name: 'Quarter' }))
    expect(mockSetWindow).toHaveBeenLastCalledWith(null)
    await user.click(screen.getByRole('button', { name: 'All Time' }))
    expect(mockSetWindow).toHaveBeenLastCalledWith('all_time')
  })

  it('applies saved chart defaults when URL params are absent', () => {
    chartDefaults = { window: 'month', scene: '38060' }
    render(<ChartsPage />)

    expect(
      screen.getByRole('button', { name: 'This Month' })
    ).toHaveAttribute('aria-pressed', 'true')
    expect(
      screen.getByRole('button', { name: 'Chart scene: Phoenix' })
    ).toBeInTheDocument()
    expect(mockSetWindow).not.toHaveBeenCalled()
    expect(mockSetScene).not.toHaveBeenCalled()

    const moduleCalls = mockScopedHook.mock.calls.filter(
      ([name]) => name !== 'scenes'
    )
    for (const call of moduleCalls) {
      expect(call.at(-1)).toMatchObject({ scene: '38060', enabled: true })
    }
  })

  it('lets explicit URL params override saved chart defaults', () => {
    chartDefaults = { window: 'month', scene: '38060' }
    queryWindow = 'all_time'
    queryScene = 'all'
    render(<ChartsPage />)

    expect(
      screen.getByRole('button', { name: 'All Time' })
    ).toHaveAttribute('aria-pressed', 'true')
    expect(
      screen.getByRole('button', { name: 'Chart scene: All scenes' })
    ).toBeInTheDocument()
    const moduleCalls = mockScopedHook.mock.calls.filter(
      ([name]) => name !== 'scenes'
    )
    for (const call of moduleCalls) {
      expect(call.at(-1)).toMatchObject({ scene: '', enabled: true })
    }
  })

  it('writes an explicit all-scenes sentinel when clearing over a saved scene', async () => {
    const user = userEvent.setup()
    chartDefaults = { window: 'month', scene: '38060' }
    queryScene = '38060'
    render(<ChartsPage />)

    await user.click(
      screen.getByRole('button', { name: 'Chart scene: Phoenix' })
    )
    await user.click(screen.getByRole('menuitemradio', { name: 'All scenes' }))
    expect(mockSetScene).toHaveBeenCalledWith('all')
  })

  it('shows Save as default when the selection differs from saved prefs', () => {
    chartDefaults = { window: 'month', scene: '38060' }
    queryWindow = 'all_time'
    queryScene = 'all'
    render(<ChartsPage />)

    expect(
      screen.getByRole('button', { name: 'Save as default' })
    ).toBeInTheDocument()
  })

  it('hides the save affordance for anonymous visitors', () => {
    isAuthenticated = false
    queryWindow = 'month'
    render(<ChartsPage />)

    expect(
      screen.queryByRole('button', { name: 'Save as default' })
    ).not.toBeInTheDocument()
  })

  it('lists floored scenes and writes the selected metro to the URL', async () => {
    const user = userEvent.setup()
    render(<ChartsPage />)

    await user.click(
      screen.getByRole('button', { name: 'Chart scene: All scenes' })
    )
    expect(
      screen.getByRole('menuitemradio', { name: /Phoenix, AZ/ })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('menuitemradio', { name: /Tucson, AZ/ })
    ).toBeInTheDocument()

    await user.click(screen.getByRole('menuitemradio', { name: /Phoenix, AZ/ }))
    expect(mockSetScene).toHaveBeenCalledWith('38060')
  })

  it('clears a selected scene through the All scenes option', async () => {
    const user = userEvent.setup()
    queryScene = '38060'
    render(<ChartsPage />)

    await user.click(
      screen.getByRole('button', { name: 'Chart scene: Phoenix' })
    )
    await user.click(screen.getByRole('menuitemradio', { name: 'All scenes' }))
    expect(mockSetScene).toHaveBeenCalledWith(null)
  })

  it('round-trips a known URL scene through the masthead and every module', () => {
    queryScene = '38060'
    render(<ChartsPage />)

    expect(
      screen.getByRole('button', { name: 'Chart scene: Phoenix' })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('heading', { level: 1, name: /Phoenix/ })
    ).toBeInTheDocument()
    expect(
      screen.getByText(
        /Scene charts · Phoenix-Mesa-Chandler metro · 41 artists based here · 12 venues tracked/
      )
    ).toBeInTheDocument()

    const moduleCalls = mockScopedHook.mock.calls.filter(
      ([name]) => name !== 'scenes'
    )
    expect(moduleCalls).toHaveLength(9)
    for (const call of moduleCalls) {
      expect(call.at(-1)).toMatchObject({ scene: '38060', enabled: true })
    }
    const fullListLinks = screen.getAllByRole('link', { name: 'full list →' })
    expect(fullListLinks).toHaveLength(6)
    for (const link of fullListLinks) {
      expect(link.getAttribute('href')).toContain('scene=38060')
    }
    expect(screen.getByRole('link', { name: 'Shoegaze' })).toHaveAttribute(
      'href',
      '/shows?tags=shoegaze&cities=Phoenix%2CAZ'
    )
  })

  it('carries a non-default window into every full-list link', () => {
    queryWindow = 'month'
    render(<ChartsPage />)

    const fullListLinks = screen.getAllByRole('link', { name: 'full list →' })
    expect(fullListLinks).toHaveLength(6)
    for (const link of fullListLinks) {
      expect(link.getAttribute('href')).toContain('window=month')
    }
  })

  it('clears an unknown URL scene and renders global charts', () => {
    queryScene = '99999'
    render(<ChartsPage />)

    expect(mockSetScene).toHaveBeenCalledWith(null)
    expect(
      screen.getByRole('button', { name: 'Chart scene: All scenes' })
    ).toBeInTheDocument()
    const moduleCalls = mockScopedHook.mock.calls.filter(
      ([name]) => name !== 'scenes'
    )
    for (const call of moduleCalls) {
      expect(call.at(-1)).toMatchObject({ scene: '', enabled: true })
    }
  })

  it('clears a malformed URL scene before querying chart modules', () => {
    queryScene = 'not-a-cbsa'
    render(<ChartsPage />)

    expect(mockSetScene).toHaveBeenCalledWith(null)
    const moduleCalls = mockScopedHook.mock.calls.filter(
      ([name]) => name !== 'scenes'
    )
    for (const call of moduleCalls) {
      expect(call.at(-1)).toMatchObject({ scene: '', enabled: true })
    }
  })

  it('announces and disables the scene switcher while options load', () => {
    sceneListLoading = true
    render(<ChartsPage />)

    expect(
      screen.getByRole('button', { name: 'Chart scene: Loading scenes' })
    ).toBeDisabled()
  })

  it('revalidates the selected scene when the chart window changes', () => {
    queryScene = '38060'
    const { rerender } = render(<ChartsPage />)

    vi.clearAllMocks()
    queryWindow = 'month'
    chartScenes = [
      {
        metro: '46060',
        name: 'Tucson, AZ',
        city: 'Tucson',
        state: 'AZ',
        show_count: 6,
        artist_count: 19,
        venue_count: 8,
      },
    ]
    rerender(<ChartsPage />)

    expect(mockScopedHook).toHaveBeenCalledWith('scenes', 'month')
    expect(mockSetScene).toHaveBeenCalledWith(null)
    const moduleCalls = mockScopedHook.mock.calls.filter(
      ([name]) => name !== 'scenes'
    )
    for (const call of moduleCalls) {
      expect(call.at(-1)).toMatchObject({ scene: '', enabled: true })
    }
  })

  it('blocks an unverified scene and offers retry when the scene list is unavailable', async () => {
    const user = userEvent.setup()
    queryScene = '99999'
    sceneListError = true
    render(<ChartsPage />)

    expect(
      screen.getByRole('button', { name: 'Retry chart scenes' })
    ).toBeEnabled()
    expect(mockSetScene).not.toHaveBeenCalled()
    expect(
      screen.getByText(
        'Unable to verify this scene. Your chart selection is preserved.'
      )
    ).toBeInTheDocument()
    expect(screen.queryByText('Hardest-Working Artists')).not.toBeVisible()
    const moduleCalls = mockScopedHook.mock.calls.filter(
      ([name]) => name !== 'scenes'
    )
    for (const call of moduleCalls) {
      expect(call.at(-1)).toMatchObject({ scene: '', enabled: false })
    }

    await user.click(screen.getByRole('button', { name: 'Try again' }))
    expect(mockRefetchScenes).toHaveBeenCalledOnce()
  })

  it('keeps global charts visible and offers retry when scene discovery fails', async () => {
    const user = userEvent.setup()
    sceneListError = true
    render(<ChartsPage />)

    expect(screen.getByText('Hardest-Working Artists')).toBeVisible()
    await user.click(screen.getByRole('button', { name: 'Retry chart scenes' }))
    expect(mockRefetchScenes).toHaveBeenCalledOnce()
  })

  it('waits for cached scene data to revalidate before clearing the URL', () => {
    queryScene = '99999'
    sceneListFetching = true
    const { rerender } = render(<ChartsPage />)

    expect(mockSetScene).not.toHaveBeenCalled()
    const pendingCalls = mockScopedHook.mock.calls.filter(
      ([name]) => name !== 'scenes'
    )
    for (const call of pendingCalls) {
      expect(call.at(-1)).toMatchObject({ scene: '', enabled: false })
    }
    expect(screen.queryByText('Freshly Added')).not.toBeInTheDocument()

    vi.clearAllMocks()
    sceneListFetching = false
    rerender(<ChartsPage />)
    expect(mockSetScene).toHaveBeenCalledWith(null)
  })

  it('keeps a validated cached scene visible after a refetch error', () => {
    queryScene = '38060'
    sceneListRefetchError = true
    render(<ChartsPage />)

    expect(
      screen.getByRole('button', { name: 'Chart scene: Phoenix' })
    ).toBeInTheDocument()
    expect(
      screen.queryByText(
        'Unable to verify this scene. Your chart selection is preserved.'
      )
    ).not.toBeInTheDocument()
    const moduleCalls = mockScopedHook.mock.calls.filter(
      ([name]) => name !== 'scenes'
    )
    for (const call of moduleCalls) {
      expect(call.at(-1)).toMatchObject({ scene: '38060', enabled: true })
    }
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

  it('suggests alternative scenes when a scene filter yields no chart rows', async () => {
    const user = userEvent.setup()
    queryScene = '38060'
    emptyAllModules = true
    render(<ChartsPage />)

    const banner = screen.getByTestId('chart-zero-result-suggestions')
    expect(banner).toHaveTextContent(
      'Nothing charting in Phoenix this window — try Tucson.'
    )
    await user.click(screen.getByTestId('chart-suggest-scene-46060'))
    expect(mockSetScene).toHaveBeenCalledWith('46060')
  })

  it('does not suggest scenes when the all-scenes view is empty', () => {
    emptyAllModules = true
    render(<ChartsPage />)
    expect(
      screen.queryByTestId('chart-zero-result-suggestions')
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

  it('pins a calendar archive window and hides the live ticker', () => {
    render(<ChartsPage pinnedWindow="2026-q1" />)

    expect(
      screen.getByRole('heading', { name: 'Charts — Q1 2026' })
    ).toBeInTheDocument()
    expect(screen.getByTestId('chart-archive-masthead')).toBeInTheDocument()
    expect(screen.queryByText('Freshly Added')).not.toBeInTheDocument()
    expect(
      screen.queryByRole('group', { name: 'Chart window' })
    ).not.toBeInTheDocument()

    expect(mockScopedHook).toHaveBeenCalledWith(
      'active',
      '2026-q1',
      7,
      expect.anything()
    )
    const fullListLinks = screen.getAllByRole('link', { name: 'full list →' })
    for (const link of fullListLinks) {
      expect(link.getAttribute('href')).toContain('window=2026-q1')
    }
  })

  it('exposes front-page archive entry links on the live Broadsheet', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-07-18T12:00:00Z'))
    render(<ChartsPage />)

    const entries = screen.getByTestId('chart-archive-entry-links')
    expect(entries).toHaveTextContent('Archives:')
    expect(screen.getByRole('link', { name: '2026' })).toHaveAttribute(
      'href',
      '/charts/2026'
    )
    expect(screen.getByRole('link', { name: 'Q2 2026' })).toHaveAttribute(
      'href',
      '/charts/2026/q2'
    )
    vi.useRealTimers()
  })
})
