import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'
import { SCENE_ARTISTS_ANCHOR } from './SceneGraph'
import type { SceneDetail } from '../types'

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
// stubbed here so the status band's clauses can be driven directly, and the
// component records the scene it was handed so the canonical-slug rule stays
// covered after the calendar moved out of this file.
const mockCalendarWindow = vi.fn()
const calendarScenes: SceneDetail[] = []
vi.mock('./SceneCalendar', () => ({
  SceneCalendar: ({ scene }: { scene: SceneDetail }) => {
    calendarScenes.push(scene)
    return <div data-testid="scene-calendar" />
  },
  useSceneCalendarWindow: () => mockCalendarWindow(),
}))

// Keep the REAL SCENE_ARTISTS_ANCHOR (via importActual) so the anchor-id test
// below exercises the actual constant, not a hand-typed copy. Only the heavy
// canvas component is stubbed.
vi.mock('./SceneGraph', async importOriginal => ({
  ...(await importOriginal<typeof import('./SceneGraph')>()),
  SceneGraph: () => <div data-testid="scene-graph" />,
}))

// The three identity sections each own a request and a suite (SceneRooms /
// SceneNewBands / SceneRoster). Stub them here, same rule as the calendar and
// the graph above: this file is about the view's COMPOSITION — what renders,
// in what order, and what the anchor hangs off.
vi.mock('./SceneRooms', () => ({
  SceneRooms: () => <div data-testid="scene-rooms" />,
}))
vi.mock('./SceneNewBands', () => ({
  SceneNewBands: () => <div data-testid="scene-new-bands" />,
}))
vi.mock('./SceneRoster', () => ({
  SceneRoster: ({ anchorId }: { anchorId?: string }) => (
    <div data-testid="scene-roster" id={anchorId} />
  ),
}))

const mockUseSceneDetail = vi.fn()
vi.mock('../hooks', () => ({
  useSceneDetail: () => mockUseSceneDetail(),
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

describe('SceneDetailView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    calendarScenes.length = 0
    mockCalendarWindow.mockReturnValue({ timeZone: 'America/Phoenix' })
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

    it('names the place, the volume, the coverage and the clock', () => {
      renderWithProviders(<SceneDetailView slug="phoenix-az" />)
      expect(
        screen.getByText(
          'Phoenix, AZ · 45 upcoming shows · 12 rooms tracked · all times MST'
        )
      ).toBeInTheDocument()
    })

    it('pluralizes a single upcoming show', () => {
      mockUseSceneDetail.mockReturnValue({
        data: buildScene({
          stats: {
            venue_count: 1,
            artist_count: 0,
            upcoming_show_count: 1,
            festival_count: 0,
          },
        }),
        isLoading: false,
        error: null,
      })
      renderWithProviders(<SceneDetailView slug="phoenix-az" />)
      expect(
        screen.getByText('Phoenix, AZ · 1 upcoming show · 1 room tracked · all times MST')
      ).toBeInTheDocument()
    })

    // Naming a clock is a claim. A zone the rows could not supply would be the
    // reader's own, which is a confidently wrong answer about a city they are
    // not in.
    it('drops the clock when the zone is unknown', () => {
      mockCalendarWindow.mockReturnValue({ timeZone: undefined })
      renderWithProviders(<SceneDetailView slug="phoenix-az" />)
      expect(
        screen.getByText('Phoenix, AZ · 45 upcoming shows · 12 rooms tracked')
      ).toBeInTheDocument()
    })

    // `GET /scenes/{slug}` has no calendar-week field, and the only count it
    // carries spans every future show. Labelling that "this week" is the exact
    // defect PSY-1623 removed from two other surfaces.
    it('carries no this-week count', () => {
      renderWithProviders(<SceneDetailView slug="phoenix-az" />)
      expect(screen.queryByText(/this week/i)).not.toBeInTheDocument()
    })

    // The window opens at NOW, so any tonight count taken from it undercounts
    // once doors open, and names YESTERDAY between midnight and 6am. Both
    // spellings contradicted /scenes/{slug}/tonight one click away.
    it('carries no tonight count', () => {
      renderWithProviders(<SceneDetailView slug="phoenix-az" />)
      expect(screen.queryByText(/tonight/i)).not.toBeInTheDocument()
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

    it('mounts the calendar as the page primary object', () => {
      renderWithProviders(<SceneDetailView slug="phoenix-az" />)
      expect(screen.getByTestId('scene-calendar')).toBeInTheDocument()
    })

    // A metro MEMBER slug resolves to its principal city, so `/scenes/mesa-az`
    // renders the Phoenix scene. Every link the calendar builds hangs off the
    // slug it is handed; building from the requested spelling would mint a
    // second URL for pages that already have one.
    it('hands the calendar the canonical scene, not the requested slug', () => {
      renderWithProviders(<SceneDetailView slug="mesa-az" />)
      expect(calendarScenes).toHaveLength(1)
      expect(calendarScenes[0].slug).toBe('phoenix-az')
    })

    // AC1 is about ORDER, and the whole page order proves it in one pass: the
    // calendar is the page's object, and the identity that used to outrank it
    // sits underneath in the mock's sequence — the rooms this page speaks for
    // (its coverage disclosure), then who is new, then who lives here, with the
    // graph demoted below all three.
    it('renders the calendar first, then the identity sections in the mock order', () => {
      renderWithProviders(<SceneDetailView slug="phoenix-az" />)
      const order = [
        'scene-calendar',
        'scene-rooms',
        'scene-new-bands',
        'scene-roster',
        'scene-graph',
      ]
      for (let i = 0; i < order.length - 1; i++) {
        expect(
          screen
            .getByTestId(order[i])
            .compareDocumentPosition(screen.getByTestId(order[i + 1])) &
            Node.DOCUMENT_POSITION_FOLLOWING
        ).toBeTruthy()
      }
    })

    // The h1 is followed directly by the stat line. Anything between them would
    // be the tagline or crews slot rendering scaffolding it has no data for.
    it('puts nothing between the heading and the stat line', () => {
      renderWithProviders(<SceneDetailView slug="phoenix-az" />)
      const heading = screen.getByRole('heading', { level: 1, name: 'Phoenix, AZ' })
      expect(heading.nextElementSibling?.textContent).toBe(
        '12 venues · 85 artists based here · 45 upcoming shows'
      )
    })

    // The two stateful sections are keyed by slug so their state resets on a
    // scene-to-scene navigation, and a bare `scene.slug` on both made them two
    // SIBLINGS WITH THE SAME KEY — which React reports only as a console error,
    // so every assertion above still passed while the page logged on every
    // render. Nothing here should be writing to console.error.
    it('renders without a React key or validation warning', () => {
      const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
      renderWithProviders(<SceneDetailView slug="phoenix-az" />)
      expect(consoleError).not.toHaveBeenCalled()
      consoleError.mockRestore()
    })

    // The anchor travels with the ROSTER, which is where the mobile graph
    // teaser is sending the reader. Identity-asserted rather than merely
    // present, so moving it onto some other section fails here rather than
    // silently landing the teaser somewhere else on the page.
    it('hangs the #scene-artists anchor off the roster (PSY-1472)', () => {
      const { container } = renderWithProviders(<SceneDetailView slug="phoenix-az" />)
      expect(container.querySelector(`#${SCENE_ARTISTS_ANCHOR}`)).toBe(
        screen.getByTestId('scene-roster')
      )
    })

    it('mounts the SceneGraph below the calendar substance', () => {
      renderWithProviders(<SceneDetailView slug="phoenix-az" />)
      expect(screen.getByTestId('scene-graph')).toBeInTheDocument()
    })

    // The count-only Venues card the rooms leaderboard replaces. It printed
    // `12 venues in Phoenix` and one link, and named no room — the module the
    // brief called an integer where a list belonged.
    it('no longer renders the count-only venues card', () => {
      renderWithProviders(<SceneDetailView slug="phoenix-az" />)
      expect(
        screen.queryByRole('link', { name: /View all venues/i })
      ).not.toBeInTheDocument()
      expect(screen.queryByText(/12 venues in Phoenix/)).not.toBeInTheDocument()
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
})
