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
const embedProps: { artistName: string; bandcampAlbumUrl?: string | null }[] = []
vi.mock('@/components/shared', async importOriginal => ({
  ...(await importOriginal<typeof import('@/components/shared')>()),
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
    mockUseSceneArtists.mockReturnValue({
      data: { artists: [artist()], total: 17 },
      isLoading: false,
    })
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
    mockUseSceneArtists.mockReturnValue({
      data: { artists: [artist({ slug: '' })], total: 1 },
      isLoading: false,
    })
    renderWithProviders(<SceneRoster scene={buildScene()} />)
    expect(screen.getByText('Gatecreeper')).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: 'Gatecreeper' })).not.toBeInTheDocument()
  })

  describe('players', () => {
    it('renders an OPEN player for each band that has an embed', () => {
      mockUseSceneArtists.mockReturnValue({
        data: {
          artists: [
            artist({ bandcamp_embed_url: 'https://gatecreeper.bandcamp.com/album/deserted' }),
            artist({ id: 2, slug: 'diners', name: 'Diners', bandcamp_embed_url: null }),
          ],
          total: 2,
        },
        isLoading: false,
      })
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
      mockUseSceneArtists.mockReturnValue({
        data: {
          artists: [
            artist({ bandcamp_embed_url: 'https://gatecreeper.bandcamp.com/album/deserted' }),
          ],
          total: 1,
        },
        isLoading: false,
      })
      const { container } = renderWithProviders(<SceneRoster scene={buildScene()} />)
      expect(container.querySelector('details')).toBeNull()
      expect(screen.queryByRole('button', { name: /play|listen|expand/i })).toBeNull()
    })

    it('renders a plain list when no band has music', () => {
      mockUseSceneArtists.mockReturnValue({
        data: { artists: [artist({ bandcamp_embed_url: null })], total: 1 },
        isLoading: false,
      })
      renderWithProviders(<SceneRoster scene={buildScene()} />)
      expect(embedProps).toHaveLength(0)
    })
  })

  describe('the shows figure', () => {
    // The mock draws `6 upcoming · next Aug 8, Nile Theater`. The endpoint
    // carries neither half — only a total, all-time approved count — so the row
    // states what it has under the label that is true of it.
    it('labels the count as LISTED shows, never as upcoming', () => {
      mockUseSceneArtists.mockReturnValue({
        data: {
          artists: [
            artist({ show_count: 6 }),
            artist({ id: 2, slug: 'latter', name: 'Latter', show_count: 1 }),
          ],
          total: 2,
        },
        isLoading: false,
      })
      renderWithProviders(<SceneRoster scene={buildScene()} />)

      expect(screen.getByText('6 shows listed')).toBeInTheDocument()
      expect(screen.getByText('1 show listed')).toBeInTheDocument()
      expect(screen.queryByText(/upcoming/i)).not.toBeInTheDocument()
    })

    it('marks active bands and leaves the rest unmarked', () => {
      mockUseSceneArtists.mockReturnValue({
        data: {
          artists: [
            artist({ is_active: true }),
            artist({ id: 2, slug: 'latter', name: 'Latter', is_active: false }),
          ],
          total: 2,
        },
        isLoading: false,
      })
      renderWithProviders(<SceneRoster scene={buildScene()} />)

      const rows = screen.getAllByRole('listitem')
      expect(within(rows[0]).getByText('Active')).toBeInTheDocument()
      expect(within(rows[1]).queryByText('Active')).not.toBeInTheDocument()
    })
  })

  describe('pagination', () => {
    it('asks for the first page and offers the rest', () => {
      mockUseSceneArtists.mockReturnValue({
        data: { artists: rosterOf(10), total: 17 },
        isLoading: false,
      })
      renderWithProviders(<SceneRoster scene={buildScene()} />)

      expect(mockUseSceneArtists).toHaveBeenCalledWith(
        expect.objectContaining({ slug: 'phoenix-az', limit: 10, keepPreviousPage: true })
      )
      expect(screen.getByRole('button', { name: 'Show all 17' })).toBeInTheDocument()
    })

    it('fetches the whole roster when the reader asks for it', async () => {
      const user = userEvent.setup()
      mockUseSceneArtists.mockReturnValue({
        data: { artists: rosterOf(10), total: 17 },
        isLoading: false,
      })
      renderWithProviders(<SceneRoster scene={buildScene()} />)

      await user.click(screen.getByRole('button', { name: 'Show all 17' }))

      expect(mockUseSceneArtists).toHaveBeenLastCalledWith(
        expect.objectContaining({ limit: 17 })
      )
    })

    it('offers no control when the whole roster already fits', () => {
      mockUseSceneArtists.mockReturnValue({
        data: { artists: rosterOf(9), total: 9 },
        isLoading: false,
      })
      renderWithProviders(<SceneRoster scene={buildScene()} />)
      expect(screen.queryByRole('button', { name: /Show all/ })).not.toBeInTheDocument()
    })

    // The endpoint caps `limit` at 100, so a control promising "Show all 340"
    // would be a promise this page cannot keep.
    it('states the ceiling instead of promising a roster it cannot fetch', async () => {
      const user = userEvent.setup()
      mockUseSceneArtists.mockReturnValue({
        data: { artists: rosterOf(10), total: 340 },
        isLoading: false,
      })
      renderWithProviders(<SceneRoster scene={buildScene()} />)

      await user.click(screen.getByRole('button', { name: 'Show all 340' }))
      expect(mockUseSceneArtists).toHaveBeenLastCalledWith(
        expect.objectContaining({ limit: 100 })
      )

      mockUseSceneArtists.mockReturnValue({
        data: { artists: rosterOf(100), total: 340 },
        isLoading: false,
      })
      renderWithProviders(<SceneRoster scene={buildScene()} />)
      expect(
        screen.getByText('Showing 100 of 340 bands based in Phoenix')
      ).toBeInTheDocument()
    })
  })

  describe('the zero state', () => {
    // London: 197 upcoming shows and 0 based-here artists. The retired shape
    // was a titled card over a 130px collapsed stub.
    it('renders nothing when no band is based here', () => {
      mockUseSceneArtists.mockReturnValue({
        data: { artists: [], total: 0 },
        isLoading: false,
      })
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
    mockUseSceneArtists.mockReturnValue({
      data: { artists: [artist()], total: 1 },
      isLoading: false,
    })
    const { container } = renderWithProviders(
      <SceneRoster scene={buildScene()} anchorId="scene-artists" />
    )
    expect(container.querySelector('#scene-artists')).toBeInTheDocument()
  })

  it('uses no em dashes', () => {
    mockUseSceneArtists.mockReturnValue({
      data: { artists: rosterOf(10), total: 17 },
      isLoading: false,
    })
    const { container } = renderWithProviders(<SceneRoster scene={buildScene()} />)
    expect(container.textContent).not.toContain('—')
  })
})
