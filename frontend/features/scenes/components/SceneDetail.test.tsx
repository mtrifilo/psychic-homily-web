import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, within } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'
import type {
  SceneDetail,
  SceneArtistsResponse,
  SceneGenreResponse,
} from '../types'

// PSY-690: SceneDetailView orchestrates the scene page — loading/not-found
// branches, the header + stat summary, and the content cards (which delegate
// to useSceneArtists / useSceneGenres and the ScenePulse / SceneGraph
// children). Mock the data hooks and the two child components so this test
// covers the view's own composition without dragging in the canvas.

// FollowButton pulls AuthContext (unavailable here) — mock at the module
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

// Child components are covered by their own sibling tests; stub them so the
// canvas (SceneGraph → SceneGraphVisualization → ForceGraphView) never mounts
// and ScenePulse logic isn't re-asserted here.
vi.mock('./ScenePulse', () => ({
  ScenePulse: () => <div data-testid="scene-pulse" />,
}))
vi.mock('./SceneGraph', () => ({
  SceneGraph: () => <div data-testid="scene-graph" />,
}))

const mockUseSceneDetail = vi.fn()
const mockUseSceneArtists = vi.fn()
const mockUseSceneGenres = vi.fn()
vi.mock('../hooks', () => ({
  useSceneDetail: () => mockUseSceneDetail(),
  useSceneArtists: () => mockUseSceneArtists(),
  useSceneGenres: () => mockUseSceneGenres(),
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
    ...overrides,
  }
}

const emptyArtists: SceneArtistsResponse = { artists: [], total: 0 }
const emptyGenres: SceneGenreResponse = {
  genres: [],
  diversity_index: 0,
  diversity_label: '',
}

describe('SceneDetailView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    // Sensible defaults for the inner lists; individual tests override.
    mockUseSceneArtists.mockReturnValue({ data: emptyArtists, isLoading: false })
    mockUseSceneGenres.mockReturnValue({ data: emptyGenres, isLoading: false })
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

  describe('populated scene', () => {
    it('renders the city/state heading and the stat summary line', () => {
      mockUseSceneDetail.mockReturnValue({
        data: buildScene(),
        isLoading: false,
        error: null,
      })
      renderWithProviders(<SceneDetailView slug="phoenix-az" />)

      const heading = screen.getByRole('heading', { level: 1, name: 'Phoenix, AZ' })
      expect(heading).toBeInTheDocument()

      // The stat summary is the single `<p>` directly after the heading ROW
      // (the h1 shares a flex row with the follow button since PSY-1340); the
      // venue/artist/show counts also appear in the cards below, so match the
      // whole joined line rather than substrings.
      const summary = heading.closest('div')!.nextElementSibling as HTMLElement
      const sep = ' · ' // statParts.join separator
      expect(summary.textContent).toBe(
        ['12 venues', '85 artists', '45 upcoming shows'].join(sep)
      )
    })

    it('renders the scene description when present', () => {
      mockUseSceneDetail.mockReturnValue({
        data: buildScene({ description: 'A desert DIY scene.' }),
        isLoading: false,
        error: null,
      })
      renderWithProviders(<SceneDetailView slug="phoenix-az" />)
      expect(screen.getByText('A desert DIY scene.')).toBeInTheDocument()
    })

    it('mounts the ScenePulse and SceneGraph children', () => {
      mockUseSceneDetail.mockReturnValue({
        data: buildScene(),
        isLoading: false,
        error: null,
      })
      renderWithProviders(<SceneDetailView slug="phoenix-az" />)
      expect(screen.getByTestId('scene-pulse')).toBeInTheDocument()
      expect(screen.getByTestId('scene-graph')).toBeInTheDocument()
    })

    it('hides the festivals card when festival_count is 0', () => {
      mockUseSceneDetail.mockReturnValue({
        data: buildScene({
          stats: {
            venue_count: 12,
            artist_count: 85,
            upcoming_show_count: 45,
            festival_count: 0,
          },
        }),
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

    it('deep-links shows and venues via the canonical ?cities= param', () => {
      mockUseSceneDetail.mockReturnValue({
        data: buildScene({ city: 'Los Angeles', state: 'CA', slug: 'los-angeles-ca' }),
        isLoading: false,
        error: null,
      })
      renderWithProviders(<SceneDetailView slug="los-angeles-ca" />)

      expect(screen.getByRole('link', { name: /View upcoming shows/i })).toHaveAttribute(
        'href',
        '/shows?cities=Los%20Angeles%2CCA'
      )
      expect(screen.getByRole('link', { name: /View all venues/i })).toHaveAttribute(
        'href',
        '/venues?cities=Los%20Angeles%2CCA'
      )
    })
  })

  describe('active artists list', () => {
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
      // The overflow copy is `total - 10` (the list caps display at 10).
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

  describe('genre distribution', () => {
    beforeEach(() => {
      mockUseSceneDetail.mockReturnValue({
        data: buildScene(),
        isLoading: false,
        error: null,
      })
    })

    it('renders the genre section with a diversity label and tag pills', () => {
      mockUseSceneGenres.mockReturnValue({
        data: {
          genres: [
            { tag_id: 1, name: 'punk', slug: 'punk', count: 12 },
            { tag_id: 2, name: 'metal', slug: 'metal', count: 8 },
          ],
          diversity_index: 0.8,
          diversity_label: 'High diversity',
        } as SceneGenreResponse,
        isLoading: false,
      })
      renderWithProviders(<SceneDetailView slug="phoenix-az" />)

      expect(screen.getByText('Genre Distribution')).toBeInTheDocument()
      expect(screen.getByText('High diversity')).toBeInTheDocument()
      expect(screen.getByText('punk')).toBeInTheDocument()
      expect(screen.getByText('metal')).toBeInTheDocument()
    })

    it('renders nothing for the genre section when there are no genres', () => {
      mockUseSceneGenres.mockReturnValue({ data: emptyGenres, isLoading: false })
      renderWithProviders(<SceneDetailView slug="phoenix-az" />)
      expect(screen.queryByText('Genre Distribution')).not.toBeInTheDocument()
    })
  })
})
