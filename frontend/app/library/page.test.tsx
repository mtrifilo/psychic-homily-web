import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderWithProviders, screen, within } from '@/test/utils'

const mockReplace = vi.fn()
const mockRedirect = vi.fn()
const mockUseAuthContext = vi.fn()
const mockUseSavedShows = vi.fn()
const mockUseMyFollowing = vi.fn()

let mockSearchParams = new URLSearchParams()

vi.mock('next/navigation', () => ({
  useRouter: () => ({ replace: mockReplace }),
  useSearchParams: () => mockSearchParams,
  redirect: (path: string) => mockRedirect(path),
}))

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => mockUseAuthContext(),
}))

// The tab CONTENTS (saved-show rows, submission cards, follow rows) are
// out of scope for this chrome-focused surface (PSY-1435/1436 own them);
// stub the heavy feature modules and keep the test on header/tabs/empty states.
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
