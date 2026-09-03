import { describe, it, expect, vi, beforeEach } from 'vitest'
import { fireEvent, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders, screen, within } from '@/test/utils'

const mockReplace = vi.fn()
const mockRedirect = vi.fn()
const mockUseAuthContext = vi.fn()
const mockUseSavedShows = vi.fn()
const mockUseSavedReleases = vi.fn()
const mockUseLibraryFollowingCounts = vi.fn()
const mockUseLibraryFollowing = vi.fn()
const mockScrollTo = vi.fn()
const mockUnsaveShow = vi.fn()
const mockUnfollowEntity = vi.fn()
const mockToggleRelease = vi.fn()
const mockUseUnfollow = vi.fn()
const mockFetchNextPage = vi.fn(async () => ({ hasNextPage: false }))

let mockSearchParams = new URLSearchParams()

vi.mock('next/navigation', () => ({
  usePathname: () => '/library?tab=releases',
  useRouter: () => ({ replace: mockReplace }),
  useSearchParams: () => mockSearchParams,
  redirect: (path: string) => mockRedirect(path),
}))

// `authStatus` is the setting; `isAuthenticated` derives from it at the
// boundary, so no case describes a viewer whose two auth signals disagree.
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => {
    const value = mockUseAuthContext()
    return {
      ...value,
      isAuthenticated: value.authStatus === 'authenticated',
      isLoading: value.authStatus === 'pending',
    }
  },
}))

// Stub the heavy feature modules so this suite stays focused on the Library
// chrome and the compact saved-show row contract introduced by PSY-1440.
vi.mock('@/features/shows', () => ({
  useInfiniteSavedShows: (
    timeFilter: 'upcoming' | 'past',
    userId: number | undefined,
    enabled: boolean
  ) => mockUseSavedShows(timeFilter, userId, enabled),
  useUnsaveShow: () => ({
    mutate: mockUnsaveShow,
    isPending: false,
    variables: undefined,
  }),
  DeleteShowDialog: () => null,
  UnpublishShowDialog: () => null,
  MakePrivateDialog: () => null,
  PublishShowDialog: () => null,
  ShowForm: () => null,
  SHOW_LIST_FEATURE_POLICY: {
    ownership: {
      showSaveButton: false,
      showOwnerActions: false,
      showDetailsLink: false,
    },
  },
}))

vi.mock('@/features/releases', () => ({
  getReleaseTypeLabel: (type: string) => type,
  useSavedReleases: (...args: unknown[]) => mockUseSavedReleases(...args),
  useReleaseSaveCount: () => ({ data: undefined, isLoading: false }),
  useReleaseSaveToggle: () => ({
    toggle: mockToggleRelease,
    isLoading: false,
    error: null,
  }),
}))

vi.mock('@/lib/hooks/common/useFollow', () => ({
  useLibraryFollowingCounts: () => mockUseLibraryFollowingCounts(),
  useLibraryFollowing: (type: string) => mockUseLibraryFollowing(type),
  useUnfollow: () => mockUseUnfollow(),
}))

// PSY-1905: the alerts context bar and the per-row alerts menu. Both read the
// account preferences, so one mock covers the pair.
const mockUpdateFollowAlerts = vi.fn()
let mockAlertPreferences: {
  home_metro: string | null
  alert_defaults?: {
    shows: { in_app: boolean; email: boolean }
    releases: { in_app: boolean; email: boolean }
  }
} | null = { home_metro: '38060' }

vi.mock('@/features/auth/hooks/useAlertPreferences', () => ({
  useAlertPreferences: () => ({
    data: mockAlertPreferences ?? undefined,
    isLoading: false,
    // Resolved-or-not is load-bearing now: an unresolved read must leave the
    // home area UNKNOWN rather than reading as "no home area".
    isSuccess: mockAlertPreferences !== null,
  }),
  useHomeMetroState: () =>
    mockAlertPreferences === null
      ? undefined
      : Boolean(mockAlertPreferences.home_metro),
  useSetHomeMetro: () => ({ mutate: vi.fn(), isPending: false, isError: false }),
}))

vi.mock('@/lib/hooks/common/useFollowAlerts', () => ({
  useUpdateFollowAlerts: () => ({
    mutate: mockUpdateFollowAlerts,
    isPending: false,
    isError: false,
  }),
}))

vi.mock('@/components/shared/HomeMetroField', () => ({
  HomeMetroSelect: () => <div data-testid="home-metro-select" />,
  useHomeMetroLabel: (metro: string | null | undefined) =>
    metro ? 'Phoenix-Mesa-Chandler, AZ' : null,
}))

vi.mock('@/features/venues', () => ({
  VenueDeniedDialog: () => null,
}))

vi.mock('@/features/collections', () => ({
  CalendarFeedSection: () => <div data-testid="calendar-feed" />,
}))

vi.mock('@/features/charts/hooks', () => ({
  usePersonalChartsStats: () => ({
    data: {
      saved_shows: 0,
      artists_followed: 0,
      venues_followed: 0,
      labels_followed: 0,
      scenes_followed: 0,
      festivals_followed: 0,
      top_venue: null,
      first_activity_at: null,
      top_scenes: [],
      top_tags: [],
      top_artists: [],
    },
    isLoading: false,
    isError: false,
  }),
}))

vi.mock('@/lib/hooks/admin/useAdminShows', () => ({
  useSetShowSoldOut: () => ({ mutate: vi.fn(), isPending: false }),
  useSetShowCancelled: () => ({ mutate: vi.fn(), isPending: false }),
}))

import LibraryPage from './page'

function setAuthenticated() {
  mockUseAuthContext.mockReturnValue({
    authStatus: 'authenticated',
    user: { id: '1', email: 'alice@example.com', is_admin: false },
  })
}

function setLoadedData() {
  mockUseSavedShows.mockReturnValue({
    data: {
      pages: [{ shows: [], total: 0, limit: 4, offset: 0 }],
      pageParams: [{ limit: 4, offset: 0 }],
    },
    isLoading: false,
    error: null,
    hasNextPage: false,
    isFetchingNextPage: false,
    fetchNextPage: mockFetchNextPage,
  })
  mockUseSavedReleases.mockReturnValue({
    data: { releases: [], total: 0, limit: 50, offset: 0 },
    isLoading: false,
    error: null,
  })
  mockUseLibraryFollowingCounts.mockReturnValue({
    data: { artists: 4, venues: 2, scenes: 3, labels: 1, festivals: 0, tags: 0 },
    isLoading: false,
    isFetching: false,
    error: null,
  })
  mockUseLibraryFollowing.mockReturnValue({
    data: { pages: [{ following: [], total: 0, limit: 50, offset: 0 }] },
    isLoading: false,
    isFetching: false,
    hasNextPage: false,
    fetchNextPage: vi.fn(),
    isFetchingNextPage: false,
    isFetchNextPageError: false,
    error: null,
  })
}

function makeSavedShow({
  id,
  title,
  eventDate,
  savedAt,
}: {
  id: number
  title: string
  eventDate: string
  savedAt: string
}) {
  return {
    id,
    title,
    slug: `show-${id}`,
    event_date: eventDate,
    saved_at: savedAt,
    state: 'AZ',
    artists: [{ id, name: title, slug: `artist-${id}` }],
    venues: [
      {
        id,
        name: `Venue ${id}`,
        slug: `venue-${id}`,
        city: 'Phoenix',
        state: 'AZ',
        timezone: 'America/Phoenix',
      },
    ],
  }
}

describe('LibraryPage (PSY-1440, PSY-1435)', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    vi.clearAllMocks()
    window.localStorage.removeItem('ph-library-view')
    Object.defineProperty(HTMLElement.prototype, 'scrollTo', {
      configurable: true,
      value: mockScrollTo,
    })
    mockSearchParams = new URLSearchParams()
    mockAlertPreferences = { home_metro: '38060' }
    setAuthenticated()
    setLoadedData()
    mockUseUnfollow.mockReturnValue({
      mutate: mockUnfollowEntity,
      isPending: false,
      isError: false,
    })
  })

  describe('header', () => {
    it('renders a plain Library title with the one-line subtitle', () => {
      renderWithProviders(<LibraryPage />)

      expect(
        screen.getByRole('heading', { level: 1, name: 'Library' })
      ).toBeTruthy()
      expect(
        screen.getByText("Everything you've saved and everyone you follow.")
      ).toBeTruthy()
    })

    it('renders the taste sidebar chrome', () => {
      renderWithProviders(<LibraryPage />)
      expect(screen.getByTestId('library-taste-sidebar')).toBeTruthy()
      expect(screen.getByText('Your taste')).toBeTruthy()
    })
  })

  describe('tab row', () => {
    it('renders counts for every follow-management tab', () => {
      renderWithProviders(<LibraryPage />)

      const tablist = screen.getByRole('tablist')
      const tabs = within(tablist).getAllByRole('tab')
      expect(tabs.map(t => t.textContent)).toEqual([
        'Shows',
        'Artists · 4',
        'Venues · 2',
        'Scenes · 3',
        'Labels · 1',
        'Festivals · 0',
        'Tags · 0',
        'Releases · 0',
      ])
      expect(mockUseSavedShows).toHaveBeenCalledTimes(2)
      expect(mockUseSavedShows).toHaveBeenCalledWith('upcoming', 1, true)
      expect(mockUseSavedShows).toHaveBeenCalledWith('past', 1, true)
      expect(mockUseLibraryFollowingCounts).toHaveBeenCalledTimes(1)
      expect(
        screen.getByRole('tab', { name: 'Artists, 4 followed' })
      ).toBeTruthy()
      expect(
        screen.getByRole('tab', { name: 'Releases, 0 saved' })
      ).toBeTruthy()
    })

    it('uses the horizontally scrollable underline tab row (no wrap)', () => {
      renderWithProviders(<LibraryPage />)

      const tablist = screen.getByRole('tablist')
      expect(tablist.className).toContain('overflow-x-auto')
      expect(tablist.className).toContain('flex-nowrap')
      expect(tablist.className).toContain('border-b')
    })

    it('scrolls a deep-linked trailing tab into the mobile tab viewport', () => {
      mockSearchParams = new URLSearchParams('tab=releases')
      const defaultBounds = HTMLElement.prototype.getBoundingClientRect
      vi.spyOn(
        HTMLElement.prototype,
        'getBoundingClientRect'
      ).mockImplementation(function (this: HTMLElement) {
        if (this.getAttribute('role') === 'tablist') {
          return { ...defaultBounds.call(this), left: 0, right: 358 }
        }
        if (
          this.getAttribute('role') === 'tab' &&
          this.textContent === 'Releases · 0'
        ) {
          return { ...defaultBounds.call(this), left: 500, right: 570 }
        }
        return defaultBounds.call(this)
      })

      renderWithProviders(<LibraryPage />)

      expect(
        screen
          .getByRole('tab', { name: 'Releases, 0 saved' })
          .getAttribute('data-state')
      ).toBe('active')
      expect(mockScrollTo).toHaveBeenCalledWith({
        behavior: 'instant',
        left: 212,
      })
    })

    it('redirects the retired submissions tab before Library data hooks run', () => {
      mockSearchParams = new URLSearchParams(
        'tab=submissions&submitted=private'
      )

      renderWithProviders(<LibraryPage />)

      expect(mockRedirect).toHaveBeenCalledWith(
        '/contribute/submissions?submitted=private'
      )
      expect(mockUseLibraryFollowingCounts).not.toHaveBeenCalled()
      expect(mockUseSavedReleases).not.toHaveBeenCalled()
      expect(screen.queryByRole('tab', { name: /submissions/i })).toBeNull()
    })

    it('realigns a deep-linked tab after asynchronous counts widen the row', () => {
      mockSearchParams = new URLSearchParams('tab=releases')
      let countsLoaded = false
      mockUseLibraryFollowingCounts.mockImplementation(() => ({
        data: countsLoaded
          ? { artists: 3, venues: 3, scenes: 3, labels: 3, festivals: 0, tags: 0 }
          : undefined,
        isLoading: !countsLoaded,
        isFetching: !countsLoaded,
        error: null,
      }))
      mockUseSavedReleases.mockImplementation(() => ({
        data: countsLoaded
          ? { releases: [], total: 1, limit: 1, offset: 0 }
          : undefined,
        isLoading: !countsLoaded,
        error: null,
      }))

      const defaultBounds = HTMLElement.prototype.getBoundingClientRect
      vi.spyOn(
        HTMLElement.prototype,
        'getBoundingClientRect'
      ).mockImplementation(function (this: HTMLElement) {
        if (this.getAttribute('role') === 'tablist') {
          return { ...defaultBounds.call(this), left: 0, right: 358 }
        }
        if (
          this.getAttribute('role') === 'tab' &&
          this.getAttribute('data-state') === 'active'
        ) {
          return {
            ...defaultBounds.call(this),
            left: countsLoaded ? 500 : 280,
            right: countsLoaded ? 580 : 350,
          }
        }
        return defaultBounds.call(this)
      })

      const { rerender } = renderWithProviders(<LibraryPage />)
      expect(mockScrollTo).not.toHaveBeenCalled()

      countsLoaded = true
      rerender(<LibraryPage />)

      expect(mockScrollTo).toHaveBeenCalledWith({
        behavior: 'instant',
        left: 222,
      })
    })
  })

  describe('empty states', () => {
    it('renders the dense Shows empty state with browse CTA and bracket links', () => {
      renderWithProviders(<LibraryPage />)

      expect(screen.getByText('Nothing saved yet.')).toBeTruthy()
      expect(
        screen.getByText(
          'Save a show and it lands here — upcoming shows first, past ones kept as your record.'
        )
      ).toBeTruthy()

      const browse = screen.getByRole('link', { name: 'Browse shows' })
      expect(browse.getAttribute('href')).toBe('/shows')

      const graph = screen.getByRole('link', { name: 'explore the graph' })
      expect(graph.getAttribute('href')).toBe('/graph')
      const atlas = screen.getByRole('link', { name: 'the atlas' })
      expect(atlas.getAttribute('href')).toBe('/atlas')
      const submissions = screen.getByRole('link', {
        name: 'show submissions',
      })
      expect(submissions.getAttribute('href')).toBe('/contribute/submissions')
    })

    it.each([
      [
        'artists',
        'No artists followed.',
        'Follow artists to keep up with their shows and releases.',
        'Browse artists',
        '/artists',
      ],
      [
        'venues',
        'No venues followed.',
        'Follow venues to keep up with their upcoming shows.',
        'Browse venues',
        '/venues',
      ],
      [
        'scenes',
        'No scenes followed.',
        'Follow scenes to keep up with the places you care about.',
        'Explore scenes',
        '/atlas',
      ],
      [
        'releases',
        'No releases saved yet.',
        'Save releases to see them here.',
        'Browse releases',
        '/releases',
      ],
      [
        'labels',
        'No labels followed.',
        'Follow labels to discover new releases and roster updates.',
        'Browse labels',
        '/labels',
      ],
      [
        'festivals',
        'No festivals followed.',
        'Follow festivals to get lineup and schedule updates.',
        'Browse festivals',
        '/festivals',
      ],
      [
        'tags',
        'No tags followed.',
        'Follow tags to surface them on your profile Following row.',
        'Browse tags',
        '/tags',
      ],
    ])(
      'renders exact %s empty-state copy and CTA',
      (tab, title, description, cta, href) => {
        mockSearchParams = new URLSearchParams(`tab=${tab}`)

        renderWithProviders(<LibraryPage />)

        expect(screen.getByText(title)).toBeTruthy()
        expect(screen.getByText(description)).toBeTruthy()
        expect(
          screen.getByRole('link', { name: cta }).getAttribute('href')
        ).toBe(href)
      }
    )
  })

  describe('follow rows', () => {
    it('renders the server-sorted Scenes page and exposes management actions', () => {
      mockSearchParams = new URLSearchParams('tab=scenes')
      mockUseUnfollow.mockReturnValue({
        mutate: mockUnfollowEntity,
        isPending: false,
        isError: true,
      })
      mockUseLibraryFollowing.mockReturnValue({
        data: {
          pages: [
            {
              following: [
                {
                  entity_type: 'scene',
                  entity_id: 1,
                  name: 'Chicago, IL',
                  slug: 'chicago-il',
                  followed_at: '2026-03-01T12:00:00Z',
                },
                {
                  entity_type: 'scene',
                  entity_id: 2,
                  name: 'Phoenix, AZ',
                  slug: 'phoenix-az',
                  followed_at: '2026-07-01T00:00:00Z',
                },
              ],
              total: 2,
              limit: 50,
              offset: 0,
            },
          ],
        },
        isLoading: false,
        isFetching: false,
        hasNextPage: false,
        fetchNextPage: vi.fn(),
        isFetchingNextPage: false,
        isFetchNextPageError: false,
        error: null,
      })

      renderWithProviders(<LibraryPage />)

      const rows = screen.getAllByRole('article')
      expect(within(rows[0]).getByRole('link').textContent).toBe('Chicago, IL')
      expect(within(rows[1]).getByRole('link').textContent).toBe('Phoenix, AZ')
      expect(within(rows[0]).getByText('followed Mar 2026')).toBeTruthy()
      expect(
        within(rows[0]).getByRole('button', { name: 'Unfollow Chicago, IL' })
      ).toBeTruthy()
      // A scene follow carries no alert subscription (the endpoints 422 for
      // it), so its row gets no bracket rather than a disabled one implying
      // the subscription could be switched on.
      expect(
        within(rows[0]).queryByRole('button', { name: /alerts/i })
      ).toBeNull()

      fireEvent.click(
        within(rows[0]).getByRole('button', { name: 'Unfollow Chicago, IL' })
      )
      expect(mockUnfollowEntity).toHaveBeenCalledWith({
        entityType: 'scenes',
        entityId: 'chicago-il',
      })
      expect(within(rows[0]).getByRole('alert')).toHaveTextContent(
        "Couldn't unfollow Chicago, IL. Try again."
      )
    })

    // ----- PSY-1905: the per-follow alerts bracket and its context bar -----

    const artistRow = (
      alerts?: {
        entity_type: string
        entity_id: number
        shows: { enabled: boolean; in_app: boolean; email: boolean; scope?: string }
      }
    ) => ({
      entity_type: 'artist',
      entity_id: 1,
      name: 'Alpha',
      slug: 'alpha',
      followed_at: '2026-07-01T00:00:00Z',
      ...(alerts ? { alerts } : {}),
    })

    const setArtistPage = (row: ReturnType<typeof artistRow>) => {
      mockSearchParams = new URLSearchParams('tab=artists')
      mockUseLibraryFollowing.mockReturnValue({
        data: { pages: [{ following: [row], limit: 50 }] },
        isLoading: false,
        isFetching: false,
        hasNextPage: false,
        fetchNextPage: vi.fn(),
        isFetchingNextPage: false,
        isFetchNextPageError: false,
        error: null,
      })
    }

    it('summarizes each follow’s alert scope in its row bracket', () => {
      setArtistPage(
        artistRow({
          entity_type: 'artist',
          entity_id: 1,
          shows: { enabled: true, in_app: true, email: false, scope: 'near_me' },
        })
      )
      renderWithProviders(<LibraryPage />)

      expect(
        screen.getByRole('button', { name: 'Show alerts for Alpha: near me' })
      ).toBeTruthy()
    })

    // Near-me is only real with a home area behind it. With none set, the row
    // must report what the server will actually do rather than the stored word.
    it('reports everywhere when the stored scope is near me but no area is set', () => {
      mockAlertPreferences = { home_metro: null }
      setArtistPage(
        artistRow({
          entity_type: 'artist',
          entity_id: 1,
          shows: { enabled: true, in_app: true, email: false, scope: 'near_me' },
        })
      )
      renderWithProviders(<LibraryPage />)

      expect(
        screen.getByRole('button', { name: 'Show alerts for Alpha: everywhere' })
      ).toBeTruthy()
    })

    it('renders no bracket for a follow the server sent no subscription for', () => {
      setArtistPage(artistRow())
      renderWithProviders(<LibraryPage />)

      expect(screen.queryByRole('button', { name: /alerts for/i })).toBeNull()
    })

    // Enabled with both channels off means the notifier skips this recipient,
    // so the row cannot summarize itself as "near me". The bar states the
    // ACCOUNT half of the same fact, which is what a new follow inherits.
    it('reads paused on the row and the bar when no channel is left', () => {
      mockAlertPreferences = {
        home_metro: '38060',
        alert_defaults: {
          shows: { in_app: false, email: false },
          releases: { in_app: true, email: false },
        },
      }
      setArtistPage(
        artistRow({
          entity_type: 'artist',
          entity_id: 1,
          shows: { enabled: true, in_app: false, email: false, scope: 'near_me' },
        })
      )
      renderWithProviders(<LibraryPage />)

      expect(
        screen.getByRole('button', { name: 'Show alerts for Alpha: paused' })
      ).toBeTruthy()
      expect(screen.getByText('paused')).toBeInTheDocument()
      expect(screen.queryByText('Near me')).toBeNull()
    })

    // A follow the user switched off is not paused, and it must not drag the
    // rest of the tab with it. The bar answers from the account matrix, so a
    // mixed tab, and a partial page of a cursor-paginated list, cannot move
    // its answer at all.
    it('keeps the pause with one switched-off follow in the same tab', () => {
      mockAlertPreferences = {
        home_metro: '38060',
        alert_defaults: {
          shows: { in_app: false, email: false },
          releases: { in_app: true, email: false },
        },
      }
      mockSearchParams = new URLSearchParams('tab=artists')
      mockUseLibraryFollowing.mockReturnValue({
        data: {
          pages: [
            {
              following: [
                artistRow({
                  entity_type: 'artist',
                  entity_id: 1,
                  shows: { enabled: true, in_app: false, email: false },
                }),
                {
                  ...artistRow({
                    entity_type: 'artist',
                    entity_id: 2,
                    shows: { enabled: false, in_app: false, email: false },
                  }),
                  entity_id: 2,
                  name: 'Beta',
                  slug: 'beta',
                },
              ],
              limit: 50,
            },
          ],
        },
        isLoading: false,
        isFetching: false,
        hasNextPage: false,
        fetchNextPage: vi.fn(),
        isFetchingNextPage: false,
        isFetchNextPageError: false,
        error: null,
      })
      renderWithProviders(<LibraryPage />)

      expect(screen.getByText('paused')).toBeInTheDocument()
      expect(screen.queryByText('Near me')).toBeNull()
    })

    it('writes the chosen scope through the follow-alerts mutation', async () => {
      const user = userEvent.setup()
      setArtistPage(
        artistRow({
          entity_type: 'artist',
          entity_id: 1,
          shows: { enabled: true, in_app: true, email: false, scope: 'near_me' },
        })
      )
      renderWithProviders(<LibraryPage />)

      // Radix opens this menu on pointer-down, so it needs userEvent rather
      // than a bare click event (same as FilterCard's dropdown suite).
      await user.click(
        screen.getByRole('button', { name: 'Show alerts for Alpha: near me' })
      )
      await user.click(screen.getByRole('menuitem', { name: 'Off' }))

      // The PATCH pins only the axis the choice decides: sending a scope with
      // an "off" would store a preference the user never expressed.
      expect(mockUpdateFollowAlerts).toHaveBeenCalledWith({
        entityType: 'artists',
        entityId: 1,
        update: { shows: { enabled: false } },
      })
    })

    // A venue sits in one place. The bar's scope sentence and area control
    // describe a restriction venue follows do not have, and contradict the
    // venue reveal one page over that says exactly that.
    it('omits the scope and area copy on the venues tab', () => {
      mockSearchParams = new URLSearchParams('tab=venues')
      mockUseLibraryFollowing.mockReturnValue({
        data: {
          pages: [
            {
              following: [
                {
                  entity_type: 'venue',
                  entity_id: 2,
                  name: 'Rebel Lounge',
                  slug: 'rebel-lounge',
                  followed_at: '2026-07-01T00:00:00Z',
                  alerts: {
                    entity_type: 'venue',
                    entity_id: 2,
                    shows: { enabled: true, in_app: true, email: false },
                  },
                },
              ],
              limit: 50,
            },
          ],
        },
        isLoading: false,
        isFetching: false,
        hasNextPage: false,
        fetchNextPage: vi.fn(),
        isFetchingNextPage: false,
        isFetchNextPageError: false,
        error: null,
      })
      renderWithProviders(<LibraryPage />)

      expect(screen.queryByText(/New follows start at/)).toBeNull()
      expect(screen.queryByText(/Your area/)).toBeNull()
      // The row control is still there, on its own on/off axis.
      expect(
        screen.getByRole('button', { name: 'Show alerts for Rebel Lounge: on' })
      ).toBeTruthy()
    })

    // Unknown is not "no area": labelling a near-me follow "everywhere" for a
    // round trip overstates the reach of a subscription the server scopes.
    it('renders no bracket while the account preferences are unresolved', () => {
      mockAlertPreferences = null
      setArtistPage(
        artistRow({
          entity_type: 'artist',
          entity_id: 1,
          shows: { enabled: true, in_app: true, email: false, scope: 'near_me' },
        })
      )
      renderWithProviders(<LibraryPage />)

      expect(screen.queryByRole('button', { name: /alerts for/i })).toBeNull()
    })

    it('shows the alerts context bar on a tab whose follows carry alerts', () => {
      setArtistPage(
        artistRow({
          entity_type: 'artist',
          entity_id: 1,
          shows: { enabled: true, in_app: true, email: false, scope: 'near_me' },
        })
      )
      renderWithProviders(<LibraryPage />)

      expect(screen.getByText(/New follows start at/)).toBeTruthy()
      expect(screen.getByText('Phoenix-Mesa-Chandler, AZ')).toBeTruthy()
      expect(
        screen.getByRole('link', { name: 'custom alerts →' })
      ).toHaveAttribute('href', '/settings/notification-filters')
    })

    it('loads the next bounded following page on demand', () => {
      mockSearchParams = new URLSearchParams('tab=artists')
      const fetchNextPage = vi.fn()
      mockUseLibraryFollowing.mockReturnValue({
        data: {
          pages: [
            {
              following: [
                {
                  entity_type: 'artist',
                  entity_id: 1,
                  name: 'Alpha',
                  slug: 'alpha',
                  followed_at: '2026-07-01T00:00:00Z',
                },
              ],
              total: 51,
              limit: 50,
              offset: 0,
            },
          ],
        },
        isLoading: false,
        isFetching: false,
        hasNextPage: true,
        fetchNextPage,
        isFetchingNextPage: false,
        isFetchNextPageError: false,
        error: null,
      })

      renderWithProviders(<LibraryPage />)
      fireEvent.click(screen.getByRole('button', { name: 'Load more' }))
      expect(fetchNextPage).toHaveBeenCalledTimes(1)
    })

    it('keeps loaded rows visible when the next page fails', () => {
      mockSearchParams = new URLSearchParams('tab=artists')
      mockUseLibraryFollowing.mockReturnValue({
        data: {
          pages: [
            {
              following: [
                {
                  entity_type: 'artist',
                  entity_id: 1,
                  name: 'Alpha',
                  slug: 'alpha',
                  followed_at: '2026-07-01T00:00:00Z',
                },
              ],
              limit: 50,
              next_cursor: 'retry-cursor',
            },
          ],
        },
        isLoading: false,
        isFetching: false,
        hasNextPage: true,
        fetchNextPage: vi.fn(),
        isFetchingNextPage: false,
        isFetchNextPageError: true,
        error: new Error('next page failed'),
      })

      renderWithProviders(<LibraryPage />)
      expect(screen.getByRole('link', { name: 'Alpha' })).toBeTruthy()
      expect(screen.getByText("Couldn't load more. Try again.")).toBeTruthy()
      expect(
        screen.queryByText('Failed to load. Please try again later.')
      ).toBeNull()
    })
  })

  describe('saved-release rows', () => {
    it('matches board C metadata, saved time, count, and remove action', async () => {
      mockSearchParams = new URLSearchParams('tab=releases')
      const savedAt = new Date(
        Date.now() - 2 * 24 * 60 * 60 * 1000
      ).toISOString()
      mockUseSavedReleases.mockReturnValue({
        data: {
          releases: [
            {
              id: 17,
              title: 'Clarity',
              slug: 'clarity',
              release_type: 'lp',
              release_year: 1999,
              cover_art_url: null,
              artist_count: 1,
              artists: [
                { id: 9, name: 'Jimmy Eat World', slug: 'jimmy-eat-world' },
              ],
              label_name: 'Capitol',
              label_slug: 'capitol',
              saved_at: savedAt,
            },
          ],
          total: 1,
          limit: 50,
          offset: 0,
        },
        isLoading: false,
        error: null,
      })

      renderWithProviders(<LibraryPage />)

      expect(
        screen.getByRole('tab', { name: 'Releases, 1 saved' }).textContent
      ).toBe('Releases · 1')
      const row = screen.getByRole('article')
      expect(
        within(row).getByRole('link', { name: 'Clarity' }).getAttribute('href')
      ).toBe('/releases/clarity')
      expect(
        within(row)
          .getByRole('link', { name: 'Jimmy Eat World' })
          .getAttribute('href')
      ).toBe('/artists/jimmy-eat-world')
      expect(within(row).getByText(/1999/)).toBeTruthy()
      expect(
        within(row).getByRole('link', { name: 'Capitol' }).getAttribute('href')
      ).toBe('/labels/capitol')
      expect(within(row).getByText('saved 2d ago')).toBeTruthy()

      fireEvent.click(
        within(row).getByRole('button', {
          name: 'Remove Clarity from saved releases',
        })
      )
      await waitFor(() => expect(mockToggleRelease).toHaveBeenCalledOnce())
    })
  })

  describe('saved-show rows', () => {
    it('toggles to wall view with typographic fallback tiles', async () => {
      mockUseSavedShows.mockImplementation(
        (timeFilter: 'upcoming' | 'past') => ({
          data: {
            pages: [
              {
                shows:
                  timeFilter === 'upcoming'
                    ? [
                        makeSavedShow({
                          id: 90,
                          title: 'Hotline TNT',
                          eventDate: '2026-08-19T20:00:00Z',
                          savedAt: '2026-07-10T12:00:00Z',
                        }),
                      ]
                    : [],
                total: timeFilter === 'upcoming' ? 1 : 0,
                limit: 4,
                offset: 0,
              },
            ],
            pageParams: [{ limit: 4, offset: 0 }],
          },
          isLoading: false,
          error: null,
          hasNextPage: false,
          isFetchingNextPage: false,
          fetchNextPage: mockFetchNextPage,
        })
      )

      renderWithProviders(<LibraryPage />)

      expect(screen.getByRole('radio', { name: 'Table view' })).toBeTruthy()
      fireEvent.click(screen.getByRole('radio', { name: 'Wall view' }))

      await waitFor(() => {
        expect(screen.getByTestId('library-wall-grid')).toBeTruthy()
      })
      expect(screen.getByTestId('library-wall-tile-fallback')).toBeTruthy()
      expect(
        screen.getByRole('button', {
          name: 'Remove Hotline TNT from saved shows',
        })
      ).toBeTruthy()
    })

    // The saved-shows row prints its time under the date badge. On a guessed
    // zone that clock can be hours out, so the row shows the date alone.
    it.each([
      ['America/Phoenix', true],
      [null, false],
    ])('names an hour only when the venue zone is known (%s)', (timezone, expected) => {
      mockUseSavedShows.mockImplementation(
        (timeFilter: 'upcoming' | 'past') => ({
          data: {
            pages: [
              {
                shows:
                  timeFilter === 'upcoming'
                    ? [
                        {
                          ...makeSavedShow({
                            id: 71,
                            title: 'Zoneless',
                            eventDate: '2026-09-10T03:00:00Z',
                            savedAt: '2026-07-10T12:00:00Z',
                          }),
                          state: '',
                          venues: [
                            {
                              id: 71,
                              name: 'Hall Ohne Zone',
                              slug: 'hall-ohne-zone',
                              city: 'Berlin',
                              state: '',
                              timezone,
                            },
                          ],
                        },
                      ]
                    : [],
                total: timeFilter === 'upcoming' ? 1 : 0,
                limit: 4,
                offset: 0,
              },
            ],
            pageParams: [{ limit: 4, offset: 0 }],
          },
          isLoading: false,
          error: null,
          hasNextPage: false,
          isFetchingNextPage: false,
          fetchNextPage: mockFetchNextPage,
        })
      )

      renderWithProviders(<LibraryPage />)

      const row = screen.getByRole('article', { name: 'Zoneless' })
      expect(within(row).queryByText(/8:00\s?PM/) !== null).toBe(expected)
      // The date survives either way: it is the row's ordering cue.
      expect(within(row).getByText('SEP 9')).toBeTruthy()
    })

    it('renders the compact mobile date and two-line show details', () => {
      mockUseSavedShows.mockImplementation(
        (timeFilter: 'upcoming' | 'past') => ({
          data: {
            pages: [
              {
                shows:
                  timeFilter === 'upcoming'
                    ? [
                        {
                          ...makeSavedShow({
                            id: 56,
                            title: 'Calexico',
                            eventDate: '2026-07-12T23:59:00Z',
                            savedAt: '2026-07-10T12:00:00Z',
                          }),
                          title: 'Calexico at E2E Reserved Venue',
                          slug: 'calexico-e2e-reserved-venue',
                          venues: [
                            {
                              id: 2,
                              name: 'E2E Reserved Venue',
                              slug: 'e2e-reserved-venue',
                              city: 'Phoenix',
                              state: 'AZ',
                              timezone: 'America/Phoenix',
                            },
                          ],
                        },
                      ]
                    : [],
                total: timeFilter === 'upcoming' ? 1 : 0,
                limit: 4,
                offset: 0,
              },
            ],
            pageParams: [{ limit: 4, offset: 0 }],
          },
          isLoading: false,
          error: null,
          hasNextPage: false,
          isFetchingNextPage: false,
          fetchNextPage: mockFetchNextPage,
        })
      )

      renderWithProviders(<LibraryPage />)

      const row = screen.getByRole('article', {
        name: 'Calexico at E2E Reserved Venue',
      })
      expect(row.className).toContain('grid-cols-[74px_minmax(0,1fr)]')

      const compactDate = within(row).getByText('JUL 12')
      expect(compactDate.className).toContain('md:hidden')
      expect(
        within(row).getByRole('link', { name: 'Calexico' }).getAttribute('href')
      ).toBe('/shows/calexico-e2e-reserved-venue')
      expect(
        within(row).getByRole('link', { name: 'E2E Reserved Venue' })
      ).toBeTruthy()
      expect(within(row).getByText(/Phoenix, AZ/)).toBeTruthy()
      expect(screen.getByRole('heading', { name: 'Upcoming' })).toBeTruthy()
      expect(screen.getByText('1 show · soonest first')).toBeTruthy()
      expect(screen.getByText(/0 shows · most recent first/)).toBeTruthy()
      expect(
        screen.getByText(
          'Saved shows move here automatically when the date passes.'
        )
      ).toBeTruthy()
    })

    it('renders upcoming and past buckets and removes from either section', () => {
      const upcomingShow = makeSavedShow({
        id: 1,
        title: 'Upcoming Artist',
        eventDate: '2026-07-20T03:00:00Z',
        savedAt: '2026-07-11T12:00:00Z',
      })
      const pastShow = makeSavedShow({
        id: 2,
        title: 'Past Artist',
        eventDate: '2026-06-20T03:00:00Z',
        savedAt: '2026-06-01T12:00:00Z',
      })
      mockUseSavedShows.mockImplementation(
        (timeFilter: 'upcoming' | 'past') => {
          const show = timeFilter === 'past' ? pastShow : upcomingShow
          return {
            data: {
              pages: [{ shows: [show], total: 1, limit: 4, offset: 0 }],
              pageParams: [{ limit: 4, offset: 0 }],
            },
            isLoading: false,
            error: null,
            hasNextPage: false,
            isFetchingNextPage: false,
            fetchNextPage: mockFetchNextPage,
          }
        }
      )

      renderWithProviders(<LibraryPage />)

      const upcomingRow = screen.getByRole('article', {
        name: 'Upcoming Artist',
      })
      const pastRow = screen.getByRole('article', { name: 'Past Artist' })
      expect(within(upcomingRow).getByText('JUL 19').className).toContain(
        'md:hidden'
      )
      expect(
        within(pastRow).getByText('JUN 19').closest('div')?.className
      ).toContain('text-muted-foreground')

      fireEvent.click(
        within(upcomingRow).getByRole('button', {
          name: 'Remove Upcoming Artist from saved shows',
        })
      )
      fireEvent.click(
        within(pastRow).getByRole('button', {
          name: 'Remove Past Artist from saved shows',
        })
      )

      expect(mockUnsaveShow).toHaveBeenNthCalledWith(1, 1)
      expect(mockUnsaveShow).toHaveBeenNthCalledWith(2, 2)
    })

    it('loads every page on expansion and re-expands without refetching', async () => {
      const shows = Array.from({ length: 6 }, (_, index) =>
        makeSavedShow({
          id: index + 1,
          title: `Artist ${index + 1}`,
          eventDate: `2026-07-${String(index + 20).padStart(2, '0')}T03:00:00Z`,
          savedAt: '2026-07-10T12:00:00Z',
        })
      )
      const fetchNextPage = vi.fn(async () => {
        upcomingResult.data.pages.push({
          shows: shows.slice(4),
          total: shows.length,
          limit: 100,
          offset: 4,
        })
        upcomingResult.hasNextPage = false
        return { hasNextPage: false, isFetchNextPageError: false }
      })
      const upcomingResult = {
        data: {
          pages: [
            {
              shows: shows.slice(0, 4),
              total: shows.length,
              limit: 4,
              offset: 0,
            },
          ],
          pageParams: [{ limit: 4, offset: 0 }],
        },
        isLoading: false,
        error: null,
        hasNextPage: true,
        isFetchingNextPage: false,
        fetchNextPage,
      }
      const pastResult = {
        data: {
          pages: [
            {
              shows: [],
              total: 0,
              limit: 4,
              offset: 0,
            },
          ],
          pageParams: [{ limit: 4, offset: 0 }],
        },
        isLoading: false,
        error: null,
        hasNextPage: false,
        isFetchingNextPage: false,
        fetchNextPage: mockFetchNextPage,
      }
      mockUseSavedShows.mockImplementation((timeFilter: 'upcoming' | 'past') =>
        timeFilter === 'upcoming' ? upcomingResult : pastResult
      )

      const { rerender } = renderWithProviders(<LibraryPage />)

      expect(screen.getAllByRole('article')).toHaveLength(4)
      fireEvent.click(screen.getByRole('button', { name: 'View all 6' }))
      await waitFor(() => expect(fetchNextPage).toHaveBeenCalledTimes(1))
      rerender(<LibraryPage />)
      expect(screen.getAllByRole('article')).toHaveLength(6)
      fireEvent.click(screen.getByRole('button', { name: 'Show fewer' }))
      expect(screen.getAllByRole('article')).toHaveLength(4)
      fireEvent.click(screen.getByRole('button', { name: 'View all 6' }))
      expect(screen.getAllByRole('article')).toHaveLength(6)
      expect(fetchNextPage).toHaveBeenCalledTimes(1)
    })
  })

  describe('auth', () => {
    it('redirects settled-anonymous users to /auth with a returnTo', () => {
      mockUseAuthContext.mockReturnValue({
        authStatus: 'anonymous',
        user: null,
      })

      renderWithProviders(<LibraryPage />)

      expect(mockRedirect).toHaveBeenCalledWith(
        expect.stringContaining('/auth?returnTo=')
      )
    })

    it('does not redirect while auth is unsettled', () => {
      // 'pending' is a signed-in viewer whose profile has not arrived as
      // often as it is anyone else, and this guard cannot tell them apart.
      mockUseAuthContext.mockReturnValue({
        authStatus: 'pending',
        user: null,
      })

      renderWithProviders(<LibraryPage />)

      expect(mockRedirect).not.toHaveBeenCalled()
      expect(
        screen.queryByRole('heading', { name: 'Library' })
      ).not.toBeInTheDocument()
    })
  })
})
