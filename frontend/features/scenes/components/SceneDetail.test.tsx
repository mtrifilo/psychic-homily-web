import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, within } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'
import { SCENE_ARTISTS_ANCHOR } from './SceneGraph'
import type { SceneDetail, SceneArtistsResponse } from '../types'

// SceneDetailView orchestrates the scene page: loading / not-found branches,
// the status band, the header, and the modules under the calendar. The calendar
// itself has its own suite (SceneCalendar.test.tsx); stub it here so this test
// covers the view's own composition.

// FollowButton pulls AuthContext (unavailable here), so mock at the module
// boundary, same idiom as VenueDetail/LabelDetail tests.
vi.mock('@/components/shared/FollowButton', () => ({
  FollowButton: ({ entityType, entityId }: { entityType: string; entityId: number | string }) => (
    <button data-testid="follow-button">
      Follow {entityType} {String(entityId)}
    </button>
  ),
}))

// SceneNotifyModeToggle also pulls AuthContext and has focused coverage in its
// own suite; keep this view composition test isolated from that auth concern.
vi.mock('./SceneNotifyModeToggle', () => ({
  SceneNotifyModeToggle: () => null,
}))

vi.mock('next/link', () => ({
  default: ({
    href,
    children,
    ...rest
  }: {
    href: string
    children: React.ReactNode
  }) => (
    <a href={href} {...rest}>
      {children}
    </a>
  ),
}))

// The calendar owns its own request and its own suite. The window hook is
// stubbed with a resolved zone so the status band's zone clause is exercised
// here without a network round-trip.
vi.mock('./SceneCalendar', () => ({
  SceneCalendar: () => <div data-testid="scene-calendar" />,
  useSceneCalendarWindow: () => ({ timeZone: 'America/Phoenix' }),
}))

// Keep the REAL SCENE_ARTISTS_ANCHOR (via importActual) so the anchor-id test
// below exercises the actual constant, not a hand-typed copy. Only the heavy
// canvas component is stubbed.
vi.mock('./SceneGraph', async importOriginal => ({
  ...(await importOriginal<typeof import('./SceneGraph')>()),
  SceneGraph: () => <div data-testid="scene-graph" />,
}))

const mockUseSceneDetail = vi.fn()
const mockUseSceneArtists = vi.fn()
vi.mock('../hooks', () => ({
  useSceneDetail: () => mockUseSceneDetail(),
  useSceneArtists: () => mockUseSceneArtists(),
}))

import { SceneDetailView } from './SceneDetail'

function buildScene(overrides: Partial<SceneDetail> = {}): SceneDetail {
  return {
    city: 'Phoenix',
    state: 'AZ',
    slug: 'phoenix-az',
    description: null,
    stats: {
      venue_count: 12,
      artist_count: 85,
      upcoming_show_count: 45,
      festival_count: 0,
    },
    pulse: {
      shows_this_month: 30,
      shows_prev_month: 25,
      shows_trend: 5,
      new_artists_30d: 8,
      active_venues_this_month: 10,
      shows_by_month: [20, 22, 25, 28, 30, 30],
    },
    // Tracked rooms, busiest first. The zero-count, slugless second room is the
    // sparse shape the API really sends, so a component that renders this list
    // is exercised against it by default rather than only against the happy row.
    venues: [
      {
        id: 1,
        name: 'Crescent Ballroom',
        slug: 'crescent-ballroom',
        website: 'https://crescentphx.com',
        city: 'Phoenix',
        state: 'AZ',
        upcoming_show_count: 12,
      },
      { id: 2, name: 'Quiet Room', city: 'Tempe', state: 'AZ', upcoming_show_count: 0 },
    ],
    ...overrides,
  }
}

const emptyArtists: SceneArtistsResponse = { artists: [], total: 0 }

describe('SceneDetailView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseSceneArtists.mockReturnValue({ data: emptyArtists, isLoading: false })
  })

  it('renders a loading spinner while the scene detail is loading', () => {
    mockUseSceneDetail.mockReturnValue({ data: undefined, isLoading: true, error: null })
    const { container } = renderWithProviders(<SceneDetailView slug="phoenix-az" />)
    expect(container.querySelector('.animate-spin')).toBeInTheDocument()
  })

  it('renders the not-found state on error', () => {
    mockUseSceneDetail.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error('nope'),
    })
    renderWithProviders(<SceneDetailView slug="missing" />)
    expect(screen.getByText('Scene not found')).toBeInTheDocument()
    expect(screen.getByText('Browse all scenes')).toBeInTheDocument()
  })

  it('renders the not-found state when there is no data and no error', () => {
    mockUseSceneDetail.mockReturnValue({ data: undefined, isLoading: false, error: null })
    renderWithProviders(<SceneDetailView slug="missing" />)
    expect(screen.getByText('Scene not found')).toBeInTheDocument()
  })

  describe('status band', () => {
    beforeEach(() => {
      mockUseSceneDetail.mockReturnValue({
        data: buildScene(),
        isLoading: false,
        error: null,
      })
    })

    it('names the place, the inventory and the clock', () => {
      const { container } = renderWithProviders(<SceneDetailView slug="phoenix-az" />)
      const band = container.querySelector('p.font-mono')!
      expect(band.textContent).toBe(
        'Phoenix, AZ · 45 upcoming shows · 12 rooms tracked · all times MST'
      )
    })

    // Neither number exists honestly on this payload: the detail response has
    // no calendar-week field, and the calendar window opens at NOW so a
    // tonight count taken from it is "still to come", not "tonight".
    it('carries no tonight count and no this-week count', () => {
      const { container } = renderWithProviders(<SceneDetailView slug="phoenix-az" />)
      const band = container.querySelector('p.font-mono')!
      expect(band.textContent).not.toMatch(/tonight/i)
      expect(band.textContent).not.toMatch(/this week/i)
    })
  })

  describe('populated scene', () => {
    beforeEach(() => {
      mockUseSceneDetail.mockReturnValue({
        data: buildScene(),
        isLoading: false,
        error: null,
      })
    })

    it('renders the city/state heading', () => {
      renderWithProviders(<SceneDetailView slug="phoenix-az" />)
      expect(
        screen.getByRole('heading', { level: 1, name: 'Phoenix, AZ' })
      ).toBeInTheDocument()
    })

    it('renders the stat line with every category named', () => {
      renderWithProviders(<SceneDetailView slug="phoenix-az" />)
      expect(
        screen.getByText('12 venues · 85 artists based here · 45 upcoming shows')
      ).toBeInTheDocument()
    })

    // The bug this replaces dropped zero-valued parts, so London read
    // "2 venues · 197 upcoming shows" as though artists were never a category.
    it('keeps a zero-valued part rather than dropping the category', () => {
      mockUseSceneDetail.mockReturnValue({
        data: buildScene({
          city: 'London',
          state: 'England',
          slug: 'london-england',
          stats: {
            venue_count: 2,
            artist_count: 0,
            upcoming_show_count: 197,
            festival_count: 0,
          },
        }),
        isLoading: false,
        error: null,
      })
      renderWithProviders(<SceneDetailView slug="london-england" />)
      expect(
        screen.getByText('2 venues · 0 artists based here · 197 upcoming shows')
      ).toBeInTheDocument()
    })

    it('mounts the calendar as the page’s primary object', () => {
      renderWithProviders(<SceneDetailView slug="phoenix-az" />)
      expect(screen.getByTestId('scene-calendar')).toBeInTheDocument()
    })

    it('renders the #scene-artists anchor the mobile graph teaser links to (PSY-1472)', () => {
      const { container } = renderWithProviders(<SceneDetailView slug="phoenix-az" />)
      expect(container.querySelector(`#${SCENE_ARTISTS_ANCHOR}`)).toBeInTheDocument()
    })

    it('mounts the SceneGraph below the calendar substance', () => {
      renderWithProviders(<SceneDetailView slug="phoenix-az" />)
      expect(screen.getByTestId('scene-graph')).toBeInTheDocument()
    })

    it('deep-links venues via the canonical ?cities= param', () => {
      mockUseSceneDetail.mockReturnValue({
        data: buildScene({ city: 'Los Angeles', state: 'CA', slug: 'los-angeles-ca' }),
        isLoading: false,
        error: null,
      })
      renderWithProviders(<SceneDetailView slug="los-angeles-ca" />)

      expect(screen.getByRole('link', { name: /View all venues/i })).toHaveAttribute(
        'href',
        '/venues?cities=Los%20Angeles%2CCA'
      )
    })
  })

  // Every kill-set item, asserted as ABSENT. These are not incidental removals:
  // Scene Pulse was catalog telemetry in a scene-health costume (and shipped
  // the PSY-1730 `++70` bug, retired by this deletion), Genre Distribution
  // rendered on 2 of 28 scenes, and `description` is populated on 0 of 28.
  describe('the kill set', () => {
    it('never renders the Scene Pulse card or its sparkline', () => {
      mockUseSceneDetail.mockReturnValue({
        data: buildScene(),
        isLoading: false,
        error: null,
      })
      renderWithProviders(<SceneDetailView slug="phoenix-az" />)
      expect(screen.queryByText(/scene pulse/i)).not.toBeInTheDocument()
      expect(screen.queryByText(/this month/i)).not.toBeInTheDocument()
    })

    it('never renders Genre Distribution', () => {
      mockUseSceneDetail.mockReturnValue({
        data: buildScene(),
        isLoading: false,
        error: null,
      })
      renderWithProviders(<SceneDetailView slug="phoenix-az" />)
      expect(screen.queryByText(/genre distribution/i)).not.toBeInTheDocument()
    })

    it('never renders the scene description, even when the payload carries one', () => {
      mockUseSceneDetail.mockReturnValue({
        data: buildScene({ description: 'A desert DIY scene.' }),
        isLoading: false,
        error: null,
      })
      renderWithProviders(<SceneDetailView slug="phoenix-az" />)
      expect(screen.queryByText('A desert DIY scene.')).not.toBeInTheDocument()
    })
  })

  // The tagline and crews slots are RESERVED, not built: no field exists behind
  // either, so the absent state is nothing at all, never a header over blank
  // space and never derived filler.
  describe('reserved slots', () => {
    beforeEach(() => {
      mockUseSceneDetail.mockReturnValue({
        data: buildScene(),
        isLoading: false,
        error: null,
      })
    })

    it('renders no crews header when there are no crews', () => {
      renderWithProviders(<SceneDetailView slug="phoenix-az" />)
      expect(screen.queryByText(/crews/i)).not.toBeInTheDocument()
    })

    it('renders no tagline prompt or placeholder', () => {
      renderWithProviders(<SceneDetailView slug="phoenix-az" />)
      expect(screen.queryByText(/add a tagline/i)).not.toBeInTheDocument()
    })
  })

  describe('conditional modules', () => {
    it('hides the festivals card when festival_count is 0', () => {
      mockUseSceneDetail.mockReturnValue({
        data: buildScene(),
        isLoading: false,
        error: null,
      })
      renderWithProviders(<SceneDetailView slug="phoenix-az" />)
      expect(screen.queryByText('Festivals')).not.toBeInTheDocument()
    })

    it('shows the festivals card when festival_count > 0', () => {
      mockUseSceneDetail.mockReturnValue({
        data: buildScene({
          stats: {
            venue_count: 12,
            artist_count: 85,
            upcoming_show_count: 45,
            festival_count: 3,
          },
        }),
        isLoading: false,
        error: null,
      })
      renderWithProviders(<SceneDetailView slug="phoenix-az" />)
      expect(screen.getByText('Festivals')).toBeInTheDocument()
      expect(screen.getByText(/3 festivals in Phoenix/)).toBeInTheDocument()
    })
  })

  describe('bands based here', () => {
    beforeEach(() => {
      mockUseSceneDetail.mockReturnValue({
        data: buildScene(),
        isLoading: false,
        error: null,
      })
    })

    it('renders an artist row per result with a pluralized show-count badge', () => {
      mockUseSceneArtists.mockReturnValue({
        data: {
          artists: [
            { id: 1, slug: 'gatecreeper', name: 'Gatecreeper', city: 'Phoenix', state: 'AZ', show_count: 5 },
            { id: 2, slug: 'sundressed', name: 'Sundressed', city: 'Phoenix', state: 'AZ', show_count: 1 },
          ],
          total: 2,
        } as SceneArtistsResponse,
        isLoading: false,
      })
      renderWithProviders(<SceneDetailView slug="phoenix-az" />)

      const gatecreeper = screen.getByText('Gatecreeper').closest('a')!
      expect(gatecreeper).toHaveAttribute('href', '/artists/gatecreeper')
      expect(within(gatecreeper).getByText('5 shows')).toBeInTheDocument()

      const sundressed = screen.getByText('Sundressed').closest('a')!
      expect(within(sundressed).getByText('1 show')).toBeInTheDocument()
    })

    it('renders an empty-state message when the roster is empty', () => {
      mockUseSceneArtists.mockReturnValue({ data: emptyArtists, isLoading: false })
      renderWithProviders(<SceneDetailView slug="phoenix-az" />)
      expect(
        screen.getByText('No artists based in this scene yet.')
      ).toBeInTheDocument()
    })

    it('marks active roster members with an "Active" badge, inactive ones without', () => {
      mockUseSceneArtists.mockReturnValue({
        data: {
          artists: [
            { id: 1, slug: 'gatecreeper', name: 'Gatecreeper', city: 'Phoenix', state: 'AZ', show_count: 5, is_active: true },
            { id: 2, slug: 'sundressed', name: 'Sundressed', city: 'Phoenix', state: 'AZ', show_count: 1, is_active: false },
          ],
          total: 2,
        } as SceneArtistsResponse,
        isLoading: false,
      })
      renderWithProviders(<SceneDetailView slug="phoenix-az" />)

      const gatecreeper = screen.getByText('Gatecreeper').closest('a')!
      expect(within(gatecreeper).getByText('Active')).toBeInTheDocument()

      const sundressed = screen.getByText('Sundressed').closest('a')!
      expect(within(sundressed).queryByText('Active')).not.toBeInTheDocument()
    })

    it('renders the "and N more" overflow line when total exceeds 10', () => {
      mockUseSceneArtists.mockReturnValue({
        data: {
          artists: [
            { id: 1, slug: 'a-1', name: 'Artist 1', city: 'Phoenix', state: 'AZ', show_count: 9 },
          ],
          total: 14,
        } as SceneArtistsResponse,
        isLoading: false,
      })
      renderWithProviders(<SceneDetailView slug="phoenix-az" />)
      expect(screen.getByText(/and 4 more artists/)).toBeInTheDocument()
    })
  })
})
