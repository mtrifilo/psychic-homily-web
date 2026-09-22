import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { SavedShowsModule } from './SavedShowsModule'
import type { SavedShowResponse } from '@/features/shows/types'

const mockUseSavedShows = vi.fn()
type SaveCountBatchResult = {
  data?: Record<string, { save_count: number; is_saved: boolean }>
}
const mockUseShowSaveCountBatch = vi.fn<() => SaveCountBatchResult>(() => ({
  data: undefined,
}))

const mockRefresh = vi.fn()
vi.mock('next/navigation', () => ({
  useRouter: () => ({ refresh: mockRefresh, push: vi.fn() }),
}))
vi.mock('@/features/shows/hooks/useSavedShows', () => ({
  SAVED_SHOWS_COLLAPSED_COUNT: 4,
  useSavedShows: (...args: unknown[]) => mockUseSavedShows(...args),
  useShowSaveCountBatch: () => mockUseShowSaveCountBatch(),
}))

const saveButtonProps = vi.fn()
vi.mock('@/components/shared/SaveButton', () => ({
  SaveButton: (props: { showId: number }) => {
    saveButtonProps(props)
    return <button type="button">save-{props.showId}</button>
  },
}))

// PSY-2104: the relocated city link resolves its own city selection. Stubbed
// so this suite stays about the module, and so the slot can be driven directly.
vi.mock('./HomeCityShowsLink', async importOriginal => ({
  ...(await importOriginal<object>()),
  useHomeCityLinkSlot: () => mockCityLinkSlot(),
  ResolvedHomeCityShowsLink: () => (
    <a href="/shows?cities=Phoenix%2CAZ">All upcoming shows in Phoenix, AZ →</a>
  ),
}))
const mockCityLinkSlot = vi.fn<() => 'nearby' | 'saved' | 'toolbar'>(
  () => 'nearby'
)

const mockAuthStatus = vi.fn<() => 'pending' | 'authenticated' | 'anonymous'>(
  () => 'authenticated'
)
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => {
    const authStatus = mockAuthStatus()
    return {
      authStatus,
      isAuthenticated: authStatus === 'authenticated',
      user: authStatus === 'authenticated' ? { id: '42' } : null,
    }
  },
}))

function savedShow(
  id: number,
  overrides: Partial<SavedShowResponse> = {}
): SavedShowResponse {
  return {
    id,
    title: `Show ${id}`,
    slug: `show-${id}`,
    event_date: '2026-10-01T20:00:00-07:00',
    state: 'AZ',
    saved_at: '2026-09-20T12:00:00Z',
    artists: [{ id, name: `Artist ${id}` }],
    venues: [{ id, name: 'Valley Bar', city: 'Phoenix', state: 'AZ' }],
    ...overrides,
  } as SavedShowResponse
}

beforeEach(() => {
  mockAuthStatus.mockReturnValue('authenticated')
  mockUseShowSaveCountBatch.mockReturnValue({ data: undefined })
  mockCityLinkSlot.mockReturnValue('nearby')
  saveButtonProps.mockClear()
})

describe('SavedShowsModule', () => {
  it('leads with the saved rows and the total, not the fetched row count', () => {
    mockUseSavedShows.mockReturnValue({
      data: {
        shows: [1, 2, 3, 4, 5, 6].map(id => savedShow(id)),
        total: 9,
      },
      isPending: false,
      error: null,
    })

    render(<SavedShowsModule nearbySectionId="nearby" />)

    expect(
      screen.getByRole('heading', { name: 'Your upcoming shows', level: 1 })
    ).toBeInTheDocument()
    // The read is the full page; only the collapsed count is painted.
    expect(screen.getAllByRole('article')).toHaveLength(4)
    // The cap is 4 rows; the subline must still report every save.
    expect(screen.getByText(/9 saved/)).toBeInTheDocument()
    expect(screen.getByText(/soonest first/)).toBeInTheDocument()
    expect(screen.getByRole('article', { name: 'Show 1' })).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: 'View all in Library →' })
    ).toBeInTheDocument()
    expect(
      screen.getByText('Past saved shows move to Library → Past automatically')
    ).toBeInTheDocument()
  })

  it('reads exactly the rows it paints, soonest-first, scoped to the viewer', () => {
    mockUseSavedShows.mockReturnValue({
      data: { shows: [], total: 0 },
      isPending: false,
      error: null,
    })

    render(<SavedShowsModule nearbySectionId="nearby" />)

    expect(mockUseSavedShows).toHaveBeenCalledWith({
      timeFilter: 'upcoming',
      limit: 4,
      userId: '42',
      enabled: true,
    })
  })

  it('shows the prompt row and keeps the past-shows footer when nothing is saved', () => {
    mockUseSavedShows.mockReturnValue({
      data: { shows: [], total: 0 },
      isPending: false,
      error: null,
    })

    render(<SavedShowsModule nearbySectionId="nearby" />)

    expect(
      screen.getByText('Nothing saved yet · tap ♡ on any show and it lands here')
    ).toBeInTheDocument()
    expect(
      screen.getByText('Save a show and it shows up here.')
    ).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: 'Pick from this week ↓' })
    ).toHaveAttribute('href', '#nearby')
    // The zero-state viewer is exactly the one asking where past saves went.
    expect(
      screen.getByText('Past saved shows move to Library → Past automatically')
    ).toBeInTheDocument()
  })

  it('claims nothing about the viewer while the read is unresolved', () => {
    mockUseSavedShows.mockReturnValue({
      data: undefined,
      isPending: true,
      error: null,
    })

    render(<SavedShowsModule nearbySectionId="nearby" />)

    expect(
      screen.getByRole('heading', { name: 'Your upcoming shows', level: 1 })
    ).toBeInTheDocument()
    expect(screen.queryByText(/0 saved/)).not.toBeInTheDocument()
    expect(screen.queryByText(/Nothing saved yet/)).not.toBeInTheDocument()
    expect(
      screen.queryByText('Save a show and it shows up here.')
    ).not.toBeInTheDocument()
  })

  it('reports a failed read instead of painting an empty list', () => {
    mockUseSavedShows.mockReturnValue({
      data: undefined,
      isPending: false,
      error: new Error('boom'),
    })

    render(<SavedShowsModule nearbySectionId="nearby" />)

    expect(
      screen.getByText('Unable to load your saved shows.')
    ).toBeInTheDocument()
    expect(screen.queryByText(/Nothing saved yet/)).not.toBeInTheDocument()
  })

  it('keeps the rows it already has when a refetch fails, and says so', () => {
    mockUseSavedShows.mockReturnValue({
      data: { shows: [savedShow(1)], total: 1 },
      isPending: false,
      error: new Error('boom'),
    })

    render(<SavedShowsModule nearbySectionId="nearby" />)

    expect(screen.getAllByRole('article')).toHaveLength(1)
    expect(
      screen.queryByText('Unable to load your saved shows.')
    ).not.toBeInTheDocument()
    expect(screen.getByRole('status')).toHaveTextContent(
      'Could not refresh your saved shows.'
    )
  })

  it('leaves the query disabled until the viewer is settled', () => {
    mockAuthStatus.mockReturnValue('pending')
    mockUseSavedShows.mockReturnValue({
      data: undefined,
      isPending: true,
      error: null,
    })

    render(<SavedShowsModule nearbySectionId="nearby" />)

    expect(mockUseSavedShows).toHaveBeenCalledWith(
      expect.objectContaining({ enabled: false, userId: undefined })
    )
  })
})

describe('SavedShowsModule saved-state and viewer transitions', () => {
  it('paints the rows as saved without waiting on the save-count batch', () => {
    mockUseSavedShows.mockReturnValue({
      data: { shows: [savedShow(1)], total: 1 },
      isPending: false,
      error: null,
    })
    // Batch still in flight: the row is saved by construction, so the control
    // must not offer to save it again.
    mockUseShowSaveCountBatch.mockReturnValue({ data: undefined })

    render(<SavedShowsModule nearbySectionId="nearby" />)

    expect(saveButtonProps).toHaveBeenCalledWith(
      expect.objectContaining({ saveData: { save_count: 0, is_saved: true } })
    )
  })

  it('takes the public count from the batch once it lands', () => {
    mockUseSavedShows.mockReturnValue({
      data: { shows: [savedShow(1)], total: 1 },
      isPending: false,
      error: null,
    })
    mockUseShowSaveCountBatch.mockReturnValue({
      data: { '1': { save_count: 7, is_saved: true } },
    })

    render(<SavedShowsModule nearbySectionId="nearby" />)

    expect(saveButtonProps).toHaveBeenCalledWith(
      expect.objectContaining({ saveData: { save_count: 7, is_saved: true } })
    )
  })

  it('says nothing at all once the viewer is settled anonymous', () => {
    mockAuthStatus.mockReturnValue('anonymous')
    mockUseSavedShows.mockReturnValue({
      data: undefined,
      isPending: true,
      error: null,
    })

    const { container } = render(<SavedShowsModule nearbySectionId="nearby" />)

    // Not a skeleton: an `enabled: false` query is pending forever, and a
    // signed-out viewer must not be left under "Welcome back".
    expect(container).toBeEmptyDOMElement()
  })

  it('asks the server to re-pick the variant once the viewer signs out', () => {
    mockAuthStatus.mockReturnValue('anonymous')
    mockUseSavedShows.mockReturnValue({
      data: undefined,
      isPending: true,
      error: null,
    })

    render(<SavedShowsModule nearbySectionId="nearby" />)

    expect(mockRefresh).toHaveBeenCalledTimes(1)
  })

  it('keeps the skeleton while the viewer is unsettled', () => {
    mockAuthStatus.mockReturnValue('pending')
    mockUseSavedShows.mockReturnValue({
      data: undefined,
      isPending: true,
      error: null,
    })

    render(<SavedShowsModule nearbySectionId="nearby" />)

    expect(
      screen.getByRole('heading', { name: 'Your upcoming shows', level: 1 })
    ).toBeInTheDocument()
  })

  describe('the relocated city link (PSY-2104)', () => {
    it('stays off the footer while the nearby section still carries it', () => {
      mockUseSavedShows.mockReturnValue({
        data: { shows: [savedShow(1)], total: 1 },
        isPending: false,
        error: null,
      })

      render(<SavedShowsModule nearbySectionId="nearby" />)

      expect(
        screen.queryByRole('link', { name: /All upcoming shows/ })
      ).not.toBeInTheDocument()
    })

    it('takes it into the footer when the nearby section is hidden', () => {
      mockCityLinkSlot.mockReturnValue('saved')
      mockUseSavedShows.mockReturnValue({
        data: { shows: [savedShow(1)], total: 1 },
        isPending: false,
        error: null,
      })

      render(<SavedShowsModule nearbySectionId="nearby" />)

      expect(
        screen.getByRole('link', { name: 'All upcoming shows in Phoenix, AZ →' })
      ).toHaveAttribute('href', '/shows?cities=Phoenix%2CAZ')
    })

    it('keeps it even when the saved-shows read failed, so it is never lost', () => {
      mockCityLinkSlot.mockReturnValue('saved')
      mockUseSavedShows.mockReturnValue({
        data: undefined,
        isPending: false,
        error: new Error('nope'),
      })

      render(<SavedShowsModule nearbySectionId="nearby" />)

      expect(
        screen.getByRole('link', { name: 'All upcoming shows in Phoenix, AZ →' })
      ).toBeInTheDocument()
      // The lines that belong to a list it does not have stay away.
      expect(
        screen.queryByRole('link', { name: 'Subscribe to calendar →' })
      ).not.toBeInTheDocument()
    })
  })
})
