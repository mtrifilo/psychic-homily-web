import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderWithProviders, screen, within } from '@/test/utils'

const mockReplace = vi.fn()
const mockRedirect = vi.fn()
const mockUseAuthContext = vi.fn()
const mockUseSavedShows = vi.fn()
const mockUseMyFollowing = vi.fn()
const mockScrollTo = vi.fn()

let mockSearchParams = new URLSearchParams()

vi.mock('next/navigation', () => ({
  useRouter: () => ({ replace: mockReplace }),
  useSearchParams: () => mockSearchParams,
  redirect: (path: string) => mockRedirect(path),
}))

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => mockUseAuthContext(),
}))

// Stub the heavy feature modules so this suite stays focused on the Library
// chrome and the compact saved-show row contract introduced by PSY-1440.
vi.mock('@/features/shows', () => ({
  useSavedShows: (opts?: unknown) => mockUseSavedShows(opts),
  useMySubmissions: () => ({ data: undefined, isLoading: false, error: null }),
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

vi.mock('@/lib/hooks/common/useFollow', () => ({
  useMyFollowing: (opts?: { type?: string }) => mockUseMyFollowing(opts),
  useUnfollow: () => ({ mutate: vi.fn(), isPending: false }),
}))

vi.mock('@/features/venues', () => ({
  VenueDeniedDialog: () => null,
}))

vi.mock('@/features/collections', () => ({
  CalendarFeedSection: () => <div data-testid="calendar-feed" />,
}))

vi.mock('@/lib/hooks/admin/useAdminShows', () => ({
  useSetShowSoldOut: () => ({ mutate: vi.fn(), isPending: false }),
  useSetShowCancelled: () => ({ mutate: vi.fn(), isPending: false }),
}))

import LibraryPage from './page'

function setAuthenticated() {
  mockUseAuthContext.mockReturnValue({
    isAuthenticated: true,
    isLoading: false,
    user: { id: '1', email: 'alice@example.com', is_admin: false },
  })
}

function setLoadedData() {
  mockUseSavedShows.mockReturnValue({
    data: { shows: [], total: 8, limit: 50, offset: 0 },
    isLoading: false,
    error: null,
  })
  mockUseMyFollowing.mockReturnValue({
    data: {
      following: [],
      total: 0,
      limit: 20,
      offset: 0,
    },
    isLoading: false,
    isFetching: false,
    error: null,
  })
}

describe('LibraryPage chrome (PSY-1440)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    Object.defineProperty(HTMLElement.prototype, 'scrollTo', {
      configurable: true,
      value: mockScrollTo,
    })
    mockSearchParams = new URLSearchParams()
    setAuthenticated()
    setLoadedData()
  })

  describe('header', () => {
    it('renders a plain Library title with the one-line subtitle', () => {
      renderWithProviders(<LibraryPage />)

      expect(
        screen.getByRole('heading', { level: 1, name: 'Library' })
      ).toBeTruthy()
      expect(
        screen.getByText(
          'Your saved shows, and the artists, venues, scenes and labels you follow.'
        )
      ).toBeTruthy()
    })
  })

  describe('tab row', () => {
    it('renders stable count-less labels when no batch count source exists', () => {
      renderWithProviders(<LibraryPage />)

      const tablist = screen.getByRole('tablist')
      const tabs = within(tablist).getAllByRole('tab')
      expect(tabs.map((t) => t.textContent)).toEqual([
        'Shows',
        'Artists',
        'Venues',
        'Releases',
        'Labels',
        'Festivals',
        'Submissions',
      ])
      expect(mockUseSavedShows).toHaveBeenCalledTimes(1)
      expect(mockUseMyFollowing).not.toHaveBeenCalled()
    })

    it('uses the horizontally scrollable underline tab row (no wrap)', () => {
      renderWithProviders(<LibraryPage />)

      const tablist = screen.getByRole('tablist')
      expect(tablist.className).toContain('overflow-x-auto')
      expect(tablist.className).toContain('flex-nowrap')
      expect(tablist.className).toContain('border-b')
    })

    it('scrolls a deep-linked trailing tab into the mobile tab viewport', () => {
      mockSearchParams = new URLSearchParams('tab=submissions')
      const defaultBounds = HTMLElement.prototype.getBoundingClientRect
      vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(
        function (this: HTMLElement) {
          if (this.getAttribute('role') === 'tablist') {
            return { ...defaultBounds.call(this), left: 0, right: 358 }
          }
          if (this.getAttribute('role') === 'tab' && this.textContent === 'Submissions') {
            return { ...defaultBounds.call(this), left: 500, right: 570 }
          }
          return defaultBounds.call(this)
        }
      )

      renderWithProviders(<LibraryPage />)

      expect(
        screen.getByRole('tab', { name: 'Submissions' }).getAttribute('data-state')
      ).toBe('active')
      expect(mockScrollTo).toHaveBeenCalledWith({
        behavior: 'instant',
        left: 212,
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
      expect(graph.getAttribute('href')).toBe('/explore')
      const atlas = screen.getByRole('link', { name: 'the atlas' })
      expect(atlas.getAttribute('href')).toBe('/atlas')
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
        'releases',
        'No releases saved yet.',
        'Release bookmarks are coming soon. Browse releases in the meantime.',
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
        'submissions',
        'No submissions yet.',
        'Shows you submit will appear here.',
        'Submit a show',
        '/shows/submit',
      ],
    ])(
      'renders exact %s empty-state copy and CTA',
      (tab, title, description, cta, href) => {
        mockSearchParams = new URLSearchParams(`tab=${tab}`)

        renderWithProviders(<LibraryPage />)

        expect(screen.getByText(title)).toBeTruthy()
        expect(screen.getByText(description)).toBeTruthy()
        expect(screen.getByRole('link', { name: cta }).getAttribute('href')).toBe(
          href
        )
      }
    )
  })

  describe('saved-show rows', () => {
    it('renders the compact mobile date and two-line show details', () => {
      mockUseSavedShows.mockReturnValue({
        data: {
          shows: [
            {
              id: 56,
              title: 'Calexico at E2E Reserved Venue',
              slug: 'calexico-e2e-reserved-venue',
              event_date: '2026-07-12T23:59:00Z',
              state: 'AZ',
              artists: [{ id: 1, name: 'Calexico', slug: 'calexico' }],
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
          ],
          total: 1,
          limit: 50,
          offset: 0,
        },
        isLoading: false,
        error: null,
      })

      renderWithProviders(<LibraryPage />)

      const row = screen.getByRole('article', {
        name: 'Calexico at E2E Reserved Venue',
      })
      expect(row.className).toContain('grid-cols-[74px_minmax(0,1fr)]')

      const compactDate = within(row).getByText('JUL 12')
      expect(compactDate.className).toContain('md:hidden')
      expect(within(row).getByRole('link', { name: 'Calexico' }).getAttribute('href')).toBe(
        '/shows/calexico-e2e-reserved-venue'
      )
      expect(within(row).getByRole('link', { name: 'E2E Reserved Venue' })).toBeTruthy()
      expect(within(row).getByText(/Phoenix, AZ/)).toBeTruthy()
    })
  })

  describe('auth', () => {
    it('redirects unauthenticated users to /auth', () => {
      mockUseAuthContext.mockReturnValue({
        isAuthenticated: false,
        isLoading: false,
        user: null,
      })

      renderWithProviders(<LibraryPage />)

      expect(mockRedirect).toHaveBeenCalledWith('/auth')
    })
  })
})
