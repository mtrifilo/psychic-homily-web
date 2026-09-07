import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/utils'
import type { SceneArtist, SceneDetail, SceneRepresentativeEmbed } from '../types'

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

const EMBED_URL = 'https://gatecreeper.bandcamp.com/album/deserted'

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

function representativeEmbed(
  overrides: Partial<SceneRepresentativeEmbed> = {}
): SceneRepresentativeEmbed {
  return {
    embed_url: EMBED_URL,
    artist_name: 'Gatecreeper',
    artist_slug: 'gatecreeper',
    ...overrides,
  }
}

/** The only things that vary between these tests are the roster and the pick. */
function givenRoster(
  artists: SceneArtist[],
  total = artists.length,
  embed: SceneRepresentativeEmbed | null = null
) {
  mockUseSceneArtists.mockReturnValue({
    data: { artists, total, representative_embed: embed },
    isLoading: false,
  })
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
      screen.getByRole('heading', { name: /Bands based here · 17/i })
    ).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Gatecreeper' })).toHaveAttribute(
      'href',
      '/artists/gatecreeper'
    )
  })

  describe('the preview line', () => {
    // The locked front page draws the roster as ONE line of names, not a row
    // per band: the calendar above it owns the page's height.
    it('separates the names with middots on a single line', () => {
      givenRoster(
        [
          artist(),
          artist({ id: 2, slug: 'diners', name: 'Diners' }),
          artist({ id: 3, slug: 'playboy-manbaby', name: 'Playboy Manbaby' }),
        ],
        3
      )
      renderWithProviders(<SceneRoster scene={buildScene()} />)

      const line = screen.getByText(/Gatecreeper/).closest('p')
      expect(line?.textContent).toBe(
        'Gatecreeper · Diners · Playboy Manbaby'
      )
    })

    // `show_count` is every approved show all time, anywhere, and `is_active`
    // is not an upcoming figure. Neither may be printed against a calendar.
    it('prints no per-band show figure and no activity marker', () => {
      givenRoster([artist({ show_count: 6, is_active: true })], 1)
      const { container } = renderWithProviders(<SceneRoster scene={buildScene()} />)

      expect(container.textContent).not.toContain('6 shows')
      expect(container.textContent).not.toMatch(/upcoming/i)
      expect(screen.queryByText('Active')).not.toBeInTheDocument()
    })

    it('names a slugless band without linking it to the artists index', () => {
      givenRoster([artist({ slug: '' })], 1)
      renderWithProviders(<SceneRoster scene={buildScene()} />)
      expect(screen.getByText('Gatecreeper')).toBeInTheDocument()
      expect(screen.queryByRole('link', { name: 'Gatecreeper' })).not.toBeInTheDocument()
    })
  })

  describe('the scene player', () => {
    it("plays the backend's representative pick, open", () => {
      givenRoster(rosterOf(3), 3, representativeEmbed())
      const { container } = renderWithProviders(<SceneRoster scene={buildScene()} />)

      expect(embedProps).toEqual([
        {
          artistName: 'Gatecreeper',
          bandcampAlbumUrl: EMBED_URL,
          compact: true,
        },
      ])
      // Never behind a disclosure. There is no toggle, summary or details
      // element to fail open on, which is the point of asserting it.
      expect(container.querySelector('details')).toBeNull()
      expect(screen.queryByRole('button', { name: /play|listen|expand/i })).toBeNull()
    })

    // The pick is computed over the FULL roster, so it is regularly a band the
    // preview line above does not name.
    it('names and links the band whose player it is', () => {
      givenRoster(rosterOf(3), 340, representativeEmbed({
        artist_name: 'Deep Cut',
        artist_slug: 'deep-cut',
      }))
      renderWithProviders(<SceneRoster scene={buildScene()} />)

      expect(screen.getByRole('link', { name: 'Deep Cut' })).toHaveAttribute(
        'href',
        '/artists/deep-cut'
      )
      expect(screen.getByText(/Bandcamp/)).toBeInTheDocument()
    })

    // `bandcamp_embed_url` is fill-when-empty and can be manual or
    // profile-resolved, so the field never establishes recency. Anchored to the
    // whole caption, not a blocklist of phrasings.
    it('makes no claim about which release is playing', () => {
      givenRoster(rosterOf(3), 3, representativeEmbed())
      renderWithProviders(<SceneRoster scene={buildScene()} />)
      expect(screen.getByText(/Bandcamp/).closest('p')).toHaveTextContent(
        /^Bandcamp · Gatecreeper$/
      )
    })

    // A fence against reintroducing the per-row scan, not evidence the pick
    // works: the component no longer reads `bandcamp_embed_url`, so these two
    // rows can only produce a player if someone puts that scan back.
    it('renders one player, not one per band', () => {
      givenRoster(
        [
          artist({ bandcamp_embed_url: EMBED_URL }),
          artist({
            id: 2,
            slug: 'diners',
            name: 'Diners',
            bandcamp_embed_url: 'https://diners.bandcamp.com/album/four-wheels',
          }),
        ],
        2,
        representativeEmbed()
      )
      renderWithProviders(<SceneRoster scene={buildScene()} />)
      expect(embedProps).toHaveLength(1)
      expect(screen.queryByTestId('embed-Diners')).not.toBeInTheDocument()
    })

    it('renders nothing at all when no band based here has an embed', () => {
      givenRoster([artist()], 1, null)
      const { container } = renderWithProviders(<SceneRoster scene={buildScene()} />)
      expect(embedProps).toHaveLength(0)
      expect(container.textContent).not.toMatch(/Bandcamp/i)
    })

    // Artist slugs are nullable and can generate as "", and the caption is the
    // only place this component links the pick.
    it('names an unlinkable pick without linking it', () => {
      givenRoster(rosterOf(3), 3, representativeEmbed({ artist_slug: '' }))
      renderWithProviders(<SceneRoster scene={buildScene()} />)

      expect(screen.getByText(/Bandcamp/).closest('p')).toHaveTextContent(
        /^Bandcamp · Gatecreeper$/
      )
      expect(screen.queryByRole('link', { name: 'Gatecreeper' })).not.toBeInTheDocument()
    })

    // The host anchor is the half of the strand guard this component owns: a
    // value it rejects suppresses the caption as well as the player, so the two
    // never disagree about whether there is anything here (PSY-1966).
    it('renders nothing when the pick is not a renderable Bandcamp URL', () => {
      givenRoster(
        [artist()],
        1,
        representativeEmbed({ embed_url: 'https://example.com/not-bandcamp' })
      )
      const { container } = renderWithProviders(<SceneRoster scene={buildScene()} />)
      expect(embedProps).toHaveLength(0)
      expect(container.textContent).not.toMatch(/Bandcamp/i)
    })
  })

  describe('pagination', () => {
    it('asks for the first page and offers the rest', () => {
      givenRoster(rosterOf(10), 17)
      renderWithProviders(<SceneRoster scene={buildScene()} />)

      expect(mockUseSceneArtists).toHaveBeenCalledWith(
        expect.objectContaining({ slug: 'phoenix-az', limit: 10 })
      )
      expect(screen.getByRole('button', { name: 'Show all 17 →' })).toBeInTheDocument()
    })

    it('fetches the whole roster when the reader asks for it', async () => {
      const user = userEvent.setup()
      givenRoster(rosterOf(10), 17)
      renderWithProviders(<SceneRoster scene={buildScene()} />)

      await user.click(screen.getByRole('button', { name: 'Show all 17 →' }))

      expect(mockUseSceneArtists).toHaveBeenLastCalledWith(
        expect.objectContaining({ limit: 17 })
      )
    })

    it('offers no control, and elides nothing, when the whole roster fits', () => {
      givenRoster(rosterOf(9), 9)
      const { container } = renderWithProviders(<SceneRoster scene={buildScene()} />)
      expect(screen.queryByRole('button', { name: /Show all/ })).not.toBeInTheDocument()
      expect(container.textContent).not.toContain('…')
    })

    // The endpoint caps `limit` at 100. A control labelled "Show all 340" that
    // then delivers 100 breaks its promise ON THE CLICK, so the ceiling is
    // named BEFORE the reader commits, not after.
    it('names the ceiling in the label rather than over-promising', async () => {
      const user = userEvent.setup()
      givenRoster(rosterOf(10), 340)
      renderWithProviders(<SceneRoster scene={buildScene()} />)

      expect(screen.queryByRole('button', { name: 'Show all 340 →' })).toBeNull()
      await user.click(screen.getByRole('button', { name: 'Show 100 of 340 →' }))
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

    // Not even the player, which would otherwise be a section with a caption
    // and no list above it.
    it('renders nothing when the roster is empty but a pick exists', () => {
      givenRoster([], 0, representativeEmbed())
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
    givenRoster(rosterOf(10), 17, representativeEmbed())
    const { container } = renderWithProviders(<SceneRoster scene={buildScene()} />)
    expect(container.textContent).not.toContain('—')
  })
})
