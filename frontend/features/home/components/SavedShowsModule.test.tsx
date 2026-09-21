import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { SavedShowsModule } from './SavedShowsModule'
import type { SavedShowResponse } from '@/features/shows/types'

const mockUseSavedShows = vi.fn()
const mockUseShowSaveCountBatch = vi.fn(() => ({ data: undefined }))

vi.mock('@/features/shows/hooks/useSavedShows', () => ({
  SAVED_SHOWS_COLLAPSED_COUNT: 4,
  useSavedShows: (...args: unknown[]) => mockUseSavedShows(...args),
  useShowSaveCountBatch: (...args: unknown[]) =>
    mockUseShowSaveCountBatch(...(args as [])),
}))

vi.mock('@/components/shared/SaveButton', () => ({
  SaveButton: ({ showId }: { showId: number }) => (
    <button type="button">save-{showId}</button>
  ),
}))

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
})

describe('SavedShowsModule', () => {
  it('leads with the saved rows and the total, not the fetched row count', () => {
    mockUseSavedShows.mockReturnValue({
      data: { shows: [savedShow(1), savedShow(2)], total: 9 },
      isPending: false,
      error: null,
    })

    render(<SavedShowsModule nearbySectionId="nearby" />)

    expect(
      screen.getByRole('heading', { name: 'Your upcoming shows', level: 1 })
    ).toBeInTheDocument()
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

  it('asks for at most four rows, soonest-first upcoming, scoped to the viewer', () => {
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

  it('shows the prompt row and no footer when nothing is saved', () => {
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
    expect(
      screen.queryByText(/Past saved shows move to Library/)
    ).not.toBeInTheDocument()
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
