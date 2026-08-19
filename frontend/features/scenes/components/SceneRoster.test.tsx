import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import type { SceneArtist, SceneDetail } from '../types'

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

// MusicEmbed resolves a Bandcamp URL through a route handler and has its own
// suite. Stub it at the boundary and record what it was handed, so "the player
// renders, open, for the right band" is assertable without a network.
//
// Mocked at the MODULE the component imports, not at the `@/components/shared`
// barrel: SceneRoster deep-imports to keep the barrel off its dependency path
// (PSY-1772), so a barrel mock would silently stop intercepting.
const embedProps: { artistName: string; bandcampAlbumUrl?: string | null }[] = []
vi.mock('@/components/shared/MusicEmbed', () => ({
  MusicEmbed: (props: { artistName: string; bandcampAlbumUrl?: string | null }) => {
    embedProps.push(props)
    return <div data-testid={`embed-${props.artistName}`} />
  },
}))

const mockUseSceneArtists = vi.fn()
vi.mock('../hooks', () => ({
  useSceneArtists: (options: unknown) => mockUseSceneArtists(options),
}))

import { SceneRoster } from './SceneRoster'

function buildScene(overrides: Partial<SceneDetail> = {}): SceneDetail {
  return {
    city: 'Phoenix',
    state: 'AZ',
    slug: 'phoenix-az',
    description: null,
    tagline: null,
    stats: {
      venue_count: 12,
      artist_count: 17,
      upcoming_show_count: 328,
      festival_count: 0,
    },
    pulse: {
      shows_this_month: 0,
      shows_prev_month: 0,
      shows_trend: 0,
      new_artists_30d: 0,
      active_venues_this_month: 0,
      shows_by_month: [],
    },
    venues: [],
    ...overrides,
  }
}

function artist(overrides: Partial<SceneArtist> = {}): SceneArtist {
  return {
    id: 1,
    slug: 'gatecreeper',
    name: 'Gatecreeper',
    city: 'Phoenix',
    state: 'AZ',
    show_count: 6,
    is_active: true,
    ...overrides,
  }
}

/** The only thing that varies between these tests is the roster itself. */
function givenRoster(artists: SceneArtist[], total = artists.length) {
  mockUseSceneArtists.mockReturnValue({ data: { artists, total }, isLoading: false })
}

function rosterOf(count: number): SceneArtist[] {
  return Array.from({ length: count }, (_, i) =>
    artist({ id: i + 1, slug: `band-${i + 1}`, name: `Band ${i + 1}` })
  )
}

describe('SceneRoster', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    embedProps.length = 0
  })

  it('names the bands based here and how many there are', () => {
    givenRoster([artist()], 17)
    renderWithProviders(<SceneRoster scene={buildScene()} />)

    expect(
      screen.getByRole('heading', { name: /Bands \/ based in Phoenix · 17/i })
    ).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Gatecreeper' })).toHaveAttribute(
      'href',
      '/artists/gatecreeper'
    )
  })

  it('names a slugless band without linking it to the artists index', () => {
    givenRoster([artist({ slug: '' })], 1)
    renderWithProviders(<SceneRoster scene={buildScene()} />)
    expect(screen.getByText('Gatecreeper')).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: 'Gatecreeper' })).not.toBeInTheDocument()
  })

  describe('players', () => {
    it('renders an OPEN player for each band that has an embed', () => {
      givenRoster([ artist({ bandcamp_embed_url: 'https://gatecreeper.bandcamp.com/album/deserted' }), artist({ id: 2, slug: 'diners', name: 'Diners', bandcamp_embed_url: null }), ], 2)
      renderWithProviders(<SceneRoster scene={buildScene()} />)

      expect(screen.getByTestId('embed-Gatecreeper')).toBeInTheDocument()
      expect(screen.queryByTestId('embed-Diners')).not.toBeInTheDocument()
      expect(embedProps).toEqual([
        {
          artistName: 'Gatecreeper',
          bandcampAlbumUrl: 'https://gatecreeper.bandcamp.com/album/deserted',
          compact: true,
        },
      ])
    })

    // Never behind a disclosure. There is no toggle, summary or details
    // element to fail open on, which is the point of asserting it.
    it('puts no expand control between the reader and the player', () => {
      givenRoster([ artist({ bandcamp_embed_url: 'https://gatecreeper.bandcamp.com/album/deserted' }), ], 1)
      const { container } = renderWithProviders(<SceneRoster scene={buildScene()} />)
      expect(container.querySelector('details')).toBeNull()
      expect(screen.queryByRole('button', { name: /play|listen|expand/i })).toBeNull()
    })

    it('renders a plain list when no band has music', () => {
      givenRoster([artist({ bandcamp_embed_url: null })], 1)
      renderWithProviders(<SceneRoster scene={buildScene()} />)
      expect(embedProps).toHaveLength(0)
    })
  })

  describe('the shows figure', () => {
    // The mock draws `6 upcoming · next Aug 8, Nile Theater`. The endpoint
    // carries neither half — only a total, all-time approved count — so the row
    // states what it has under the label that is true of it.
    it('labels the count as LISTED shows, never as upcoming', () => {
      givenRoster([ artist({ show_count: 6 }), artist({ id: 2, slug: 'latter', name: 'Latter', show_count: 1 }), ], 2)
      renderWithProviders(<SceneRoster scene={buildScene()} />)

      expect(screen.getByText('6 shows listed')).toBeInTheDocument()
      expect(screen.getByText('1 show listed')).toBeInTheDocument()
      expect(screen.queryByText(/upcoming/i)).not.toBeInTheDocument()
    })

    it('marks active bands and leaves the rest unmarked', () => {
      givenRoster([ artist({ is_active: true }), artist({ id: 2, slug: 'latter', name: 'Latter', is_active: false }), ], 2)
      renderWithProviders(<SceneRoster scene={buildScene()} />)

      const rows = screen.getAllByRole('listitem')
      expect(within(rows[0]).getByText('Active')).toBeInTheDocument()
      expect(within(rows[1]).queryByText('Active')).not.toBeInTheDocument()
    })
  })

  describe('pagination', () => {
    it('asks for the first page and offers the rest', () => {
      givenRoster(rosterOf(10), 17)
      renderWithProviders(<SceneRoster scene={buildScene()} />)

      expect(mockUseSceneArtists).toHaveBeenCalledWith(
        expect.objectContaining({ slug: 'phoenix-az', limit: 10 })
      )
      expect(screen.getByRole('button', { name: 'Show all 17' })).toBeInTheDocument()
    })

    it('fetches the whole roster when the reader asks for it', async () => {
      const user = userEvent.setup()
      givenRoster(rosterOf(10), 17)
      renderWithProviders(<SceneRoster scene={buildScene()} />)

      await user.click(screen.getByRole('button', { name: 'Show all 17' }))

      expect(mockUseSceneArtists).toHaveBeenLastCalledWith(
        expect.objectContaining({ limit: 17 })
      )
    })

    it('offers no control when the whole roster already fits', () => {
      givenRoster(rosterOf(9), 9)
      renderWithProviders(<SceneRoster scene={buildScene()} />)
      expect(screen.queryByRole('button', { name: /Show all/ })).not.toBeInTheDocument()
    })

    // The endpoint caps `limit` at 100. A control labelled "Show all 340" that
    // then delivers 100 breaks its promise ON THE CLICK, so the ceiling is
    // named BEFORE the reader commits, not after.
    it('names the ceiling in the label rather than over-promising', async () => {
      const user = userEvent.setup()
      givenRoster(rosterOf(10), 340)
      renderWithProviders(<SceneRoster scene={buildScene()} />)

      expect(screen.queryByRole('button', { name: 'Show all 340' })).toBeNull()
      await user.click(screen.getByRole('button', { name: 'Show 100 of 340' }))
      expect(mockUseSceneArtists).toHaveBeenLastCalledWith(
        expect.objectContaining({ limit: 100 })
      )
    })

    it('stops offering the control once the ceiling is what is withholding bands', () => {
      givenRoster(rosterOf(100), 340)
      renderWithProviders(<SceneRoster scene={buildScene()} />)

      expect(screen.queryByRole('button', { name: /Show/ })).toBeNull()
      expect(
        screen.getByText('Showing 100 of 340 bands based in Phoenix')
      ).toBeInTheDocument()
    })
  })

  describe('the zero state', () => {
    // London: 197 upcoming shows and 0 based-here artists. The retired shape
    // was a titled card over a 130px collapsed stub.
    it('renders nothing when no band is based here', () => {
      givenRoster([], 0)
      const { container } = renderWithProviders(<SceneRoster scene={buildScene()} />)
      expect(container).toBeEmptyDOMElement()
    })

    it('renders nothing while the first page is in flight', () => {
      mockUseSceneArtists.mockReturnValue({ data: undefined, isLoading: true })
      const { container } = renderWithProviders(<SceneRoster scene={buildScene()} />)
      expect(container).toBeEmptyDOMElement()
    })
  })

  it('carries the anchor the mobile graph teaser links to', () => {
    givenRoster([artist()], 1)
    const { container } = renderWithProviders(
      <SceneRoster scene={buildScene()} anchorId="scene-artists" />
    )
    expect(container.querySelector('#scene-artists')).toBeInTheDocument()
  })

  it('uses no em dashes', () => {
    givenRoster(rosterOf(10), 17)
    const { container } = renderWithProviders(<SceneRoster scene={buildScene()} />)
    expect(container.textContent).not.toContain('—')
  })
})
