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

type RosterPage = {
  artists: SceneArtist[]
  total: number
  embed: SceneRepresentativeEmbed | null
}

/**
 * A different answer per page size, which is the only way to tell the two
 * hook calls apart: the component asks for the current page and, separately,
 * for the first one.
 */
function givenRosterByLimit(byLimit: Record<number, RosterPage | 'error'>) {
  mockUseSceneArtists.mockImplementation((options: { limit: number }) => {
    const page = byLimit[options.limit]
    if (!page) return { data: undefined, isLoading: true, isError: false }
    if (page === 'error') {
      return { data: undefined, isLoading: false, isError: true }
    }
    return {
      data: {
        artists: page.artists,
        total: page.total,
        representative_embed: page.embed,
      },
      isLoading: false,
      isError: false,
    }
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
    // Reset, not just clear: `givenRosterByLimit` installs an implementation,
    // and `clearAllMocks` leaves implementations in place.
    mockUseSceneArtists.mockReset()
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

    // The payload's per-band figures are all-time or derived, so none of them
    // may sit under the calendar. Anchored to the whole line rather than
    // blocklisting spellings: a blocklist passes on the next one.
    it('prints the names and nothing else about each band', () => {
      givenRoster([artist({ show_count: 6, is_active: true })], 1)
      renderWithProviders(<SceneRoster scene={buildScene()} />)

      expect(screen.getByText('Gatecreeper').closest('p')).toHaveTextContent(
        /^Gatecreeper$/
      )
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

    // The pick is scoped to the whole roster while the line above is one page
    // of it, so the player's band can be one the line never names.
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

    // The pick is derived from the rows the response carries, so a wider page
    // can nominate a different band. Expanding the names must not swap the
    // player under a reader who has pressed play.
    it('keeps the first page pick after the reader expands', async () => {
      const user = userEvent.setup()
      givenRosterByLimit({
        10: {
          artists: rosterOf(10),
          total: 40,
          embed: representativeEmbed({
            artist_name: 'Deep Cut',
            artist_slug: 'deep-cut',
          }),
        },
        40: {
          artists: rosterOf(40),
          total: 40,
          embed: representativeEmbed({
            embed_url: 'https://someoneelse.bandcamp.com/album/other',
            artist_name: 'Someone Else',
            artist_slug: 'someone-else',
          }),
        },
      })
      renderWithProviders(<SceneRoster scene={buildScene()} />)

      await user.click(screen.getByRole('button', { name: 'Show all 40 →' }))

      // One URL across every render. MusicEmbed is stubbed here, so this pins
      // the prop it would build its iframe src from, not the iframe itself.
      expect(new Set(embedProps.map(props => props.bandcampAlbumUrl))).toEqual(
        new Set([EMBED_URL])
      )
      expect(screen.getByText(/Bandcamp/).closest('p')).toHaveTextContent(
        /^Bandcamp · Deep Cut$/
      )
    })

    it('draws no player and no caption when no band based here has an embed', () => {
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
    // value it rejects suppresses the caption as well as the player, so neither
    // is ever drawn without the other.
    it('draws neither when the pick is not a renderable Bandcamp URL', () => {
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

      // Not the LAST call: every render also asks for the first page, which is
      // where the player comes from.
      expect(mockUseSceneArtists).toHaveBeenCalledWith(
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
      expect(mockUseSceneArtists).toHaveBeenCalledWith(
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

  // A read that fails after the reader has widened the list must not be
  // mistaken for a scene with no bands: the anonymous per-IP limiter makes a
  // 429 on that click ordinary traffic, and the section holds a player that may
  // be sounding.
  describe('a failed widening', () => {
    function givenFailedExpansion() {
      givenRosterByLimit({
        10: {
          artists: rosterOf(10),
          total: 340,
          embed: representativeEmbed(),
        },
        100: 'error',
      })
    }

    it('falls back to the page already on screen instead of emptying', async () => {
      const user = userEvent.setup()
      givenFailedExpansion()
      renderWithProviders(<SceneRoster scene={buildScene()} />)

      await user.click(screen.getByRole('button', { name: 'Show 100 of 340 →' }))

      expect(
        screen.getByRole('heading', { name: /Bands based here · 340/i })
      ).toBeInTheDocument()
      expect(screen.getByRole('link', { name: 'Band 1' })).toBeInTheDocument()
      expect(
        screen.getByText('Showing 10 of 340 bands based in Phoenix')
      ).toBeInTheDocument()
    })

    it('keeps the player mounted', async () => {
      const user = userEvent.setup()
      givenFailedExpansion()
      renderWithProviders(<SceneRoster scene={buildScene()} />)

      await user.click(screen.getByRole('button', { name: 'Show 100 of 340 →' }))

      expect(screen.getByTestId('embed-Gatecreeper')).toBeInTheDocument()
    })

    // `limit` already holds the value the control would set, so a second press
    // would change no state and fetch nothing.
    it('withdraws the control rather than offering an inert one', async () => {
      const user = userEvent.setup()
      givenFailedExpansion()
      const { container } = renderWithProviders(<SceneRoster scene={buildScene()} />)

      await user.click(screen.getByRole('button', { name: 'Show 100 of 340 →' }))

      expect(screen.queryByRole('button', { name: /Show/ })).toBeNull()
      expect(container.textContent).not.toContain('…')
    })
  })

  // The owner decision of 2026-09-07: the per-band line renders on the EXPANDED
  // roster only. The preview stays an identity line, which is why every
  // assertion below is paired with one that the same payload prints nothing
  // extra before the reader asks for the rest.
  describe('the expanded roster', () => {
    const BOOKED = artist({
      id: 1,
      slug: 'gatecreeper',
      name: 'Gatecreeper',
      upcoming_show_count: 2,
      next_show: {
        id: 42,
        slug: 'gatecreeper-valley-bar',
        event_date: '2026-09-09',
        venue_name: 'Valley Bar',
        venue_slug: 'valley-bar',
      },
    })
    const QUIET = artist({
      id: 2,
      slug: 'diners',
      name: 'Diners',
      upcoming_show_count: 0,
      show_count: 12,
      is_active: true,
    })

    /**
     * Widen to the whole roster, which is where the dated rows live. `head` is
     * the bands under test; the roster is padded past the first page size so
     * there is something to expand INTO.
     */
    async function expand(head: SceneArtist[]) {
      const roster = [...head, ...rosterOf(12).slice(head.length)]
      const user = userEvent.setup()
      givenRosterByLimit({
        10: { artists: roster.slice(0, 10), total: roster.length, embed: null },
        12: { artists: roster, total: roster.length, embed: null },
      })
      renderWithProviders(<SceneRoster scene={buildScene()} />)
      await user.click(screen.getByRole('button', { name: 'Show all 12 →' }))
    }

    function rowOf(name: string): HTMLElement {
      const row = screen.getByText(name).closest('li')
      expect(row).not.toBeNull()
      return row as HTMLElement
    }

    it('states what each band has booked, and links the date and the room', async () => {
      await expand([BOOKED])

      expect(rowOf('Gatecreeper')).toHaveTextContent(
        '2 upcoming · next Sep 9, Valley Bar'
      )
      expect(screen.getByRole('link', { name: 'Sep 9' })).toHaveAttribute(
        'href',
        '/shows/gatecreeper-valley-bar'
      )
      expect(screen.getByRole('link', { name: 'Valley Bar' })).toHaveAttribute(
        'href',
        '/venues/valley-bar'
      )
    })

    // Anchored to the whole row rather than blocklisting spellings: a blocklist
    // passes on the next one.
    it('gives a band with nothing booked its name and nothing else', async () => {
      await expand([BOOKED, QUIET])
      expect(rowOf('Diners')).toHaveTextContent(/^Diners$/)
    })

    it('leaves the preview a names line even when the payload is dated', () => {
      givenRoster([BOOKED, QUIET], 2)
      renderWithProviders(<SceneRoster scene={buildScene()} />)

      expect(screen.getByText('Gatecreeper').closest('p')?.textContent).toBe(
        'Gatecreeper · Diners'
      )
      expect(screen.queryByText(/upcoming/)).toBeNull()
      expect(screen.queryByRole('link', { name: 'Valley Bar' })).toBeNull()
    })

    it('dates a show with no room without a dangling comma', async () => {
      const roomless = artist({
        id: 1,
        slug: 'gatecreeper',
        name: 'Gatecreeper',
        upcoming_show_count: 1,
        next_show: { id: 42, event_date: '2026-09-09' },
      })
      await expand([roomless])
      expect(rowOf('Gatecreeper')).toHaveTextContent('1 upcoming · next Sep 9')
      expect(rowOf('Gatecreeper').textContent).not.toContain(',')
    })

    it('names a room it cannot date without reading as a date', async () => {
      const undateable = artist({
        id: 1,
        slug: 'gatecreeper',
        name: 'Gatecreeper',
        upcoming_show_count: 1,
        next_show: { id: 42, event_date: '', venue_name: 'Valley Bar', venue_slug: 'valley-bar' },
      })
      await expand([undateable])
      expect(rowOf('Gatecreeper')).toHaveTextContent('1 upcoming · next at Valley Bar')
    })

    // Show slugs are nullable, and `/shows/` with an empty one resolves to the
    // index rather than 404ing.
    it('links a slugless show by id', async () => {
      const noSlug = artist({
        id: 1,
        slug: 'gatecreeper',
        name: 'Gatecreeper',
        upcoming_show_count: 1,
        next_show: { id: 42, event_date: '2026-09-09', venue_name: 'Valley Bar' },
      })
      await expand([noSlug])
      expect(screen.getByRole('link', { name: 'Sep 9' })).toHaveAttribute('href', '/shows/42')
      expect(screen.queryByRole('link', { name: 'Valley Bar' })).toBeNull()
      expect(rowOf('Gatecreeper')).toHaveTextContent('1 upcoming · next Sep 9, Valley Bar')
    })

    // A widening that FAILS leaves the reader in the shape they asked for, over
    // the page already on screen, rather than snapping back to a preview.
    it('keeps the expanded shape when the widening fails', async () => {
      const user = userEvent.setup()
      givenRosterByLimit({
        10: { artists: [BOOKED, ...rosterOf(10).slice(1)], total: 340, embed: null },
        100: 'error',
      })
      renderWithProviders(<SceneRoster scene={buildScene()} />)

      await user.click(screen.getByRole('button', { name: 'Show 100 of 340 →' }))

      expect(rowOf('Gatecreeper')).toHaveTextContent(
        '2 upcoming · next Sep 9, Valley Bar'
      )
      expect(
        screen.getByText('Showing 10 of 340 bands based in Phoenix')
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
