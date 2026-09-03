import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ShowCard } from './ShowCard'
import type { ShowResponse, ArtistResponse } from '../types'

// Mock AuthContext.
// Return type widened so individual tests can override `user`/`isAuthenticated`
// without TS narrowing from the default-null literal.
type MockAuthContextValue = {
  user: { id: string; is_admin: boolean } | null
  isAuthenticated: boolean
  isLoading: boolean
  logout: () => void
}
const mockAuthContext = vi.fn<() => MockAuthContextValue>(() => ({
  user: null,
  isAuthenticated: false,
  isLoading: false,
  logout: vi.fn(),
}))
vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => mockAuthContext(),
}))

// Mock next/link
vi.mock('next/link', () => ({
  default: ({ href, children, ...props }: { href: string; children: React.ReactNode; [key: string]: unknown }) => (
    <a href={href} {...props}>{children}</a>
  ),
}))

// Mock child components to keep tests focused
// The barrel is stubbed to keep unrelated shared components out of this suite,
// but ShowPrice is spliced back in for REAL: the price assertions below observe
// the card's behaviour through it, and a stub would pass while the card rendered
// nothing.
vi.mock('@/components/shared', async () => {
  const showPrice = await vi.importActual<
    typeof import('@/components/shared/ShowPrice')
  >('@/components/shared/ShowPrice')
  return {
    SaveButton: ({ showId }: { showId: number }) => (
      <button data-testid="save-button">Save {showId}</button>
    ),
    SocialLinks: () => <div data-testid="social-links" />,
    MusicEmbed: () => <div data-testid="music-embed" />,
    ShowPrice: showPrice.ShowPrice,
  }
})

vi.mock('./ShowForm', () => ({
  ShowForm: ({ onCancel }: { onCancel: () => void }) => (
    <div data-testid="show-form">
      <button onClick={onCancel}>Cancel Form</button>
    </div>
  ),
}))

vi.mock('./DeleteShowDialog', () => ({
  DeleteShowDialog: ({ open }: { open: boolean }) => (
    open ? <div data-testid="delete-dialog">Delete Dialog</div> : null
  ),
}))

vi.mock('./ExportShowButton', () => ({
  ExportShowButton: () => <button data-testid="export-button">Export</button>,
}))

function makeArtist(overrides: Partial<ArtistResponse> = {}): ArtistResponse {
  return {
    id: 1,
    slug: 'artist-one',
    name: 'Artist One',
    // Default to a neutral set_type so tests can opt in to headliner status
    // via `is_headliner: true` or `set_type: 'headliner'` per-case.
    set_type: 'performer',
    position: 1,
    socials: {},
    ...overrides,
  }
}

function makeShow(overrides: Partial<ShowResponse> = {}): ShowResponse {
  return {
    id: 1,
    slug: 'test-show',
    title: 'Test Show',
    event_date: '2026-04-15T20:00:00Z',
    status: 'approved',
    city: 'Phoenix',
    state: 'AZ',
    price: 20,
    age_requirement: '21+',
    venues: [
      {
        id: 1,
        slug: 'the-venue',
        name: 'The Venue',
        city: 'Phoenix',
        state: 'AZ',
        verified: true,
      },
    ],
    artists: [
      makeArtist({ id: 1, name: 'Headliner', slug: 'headliner', is_headliner: true }),
      makeArtist({ id: 2, name: 'Opener', slug: 'opener', is_headliner: false }),
    ],
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    is_sold_out: false,
    is_cancelled: false,
    ...overrides,
  }
}

describe('ShowCard', () => {
  beforeEach(() => {
    mockAuthContext.mockReturnValue({
      user: null,
      isAuthenticated: false,
      isLoading: false,
      logout: vi.fn(),
    })
  })

  it('renders as an article element', () => {
    render(<ShowCard show={makeShow()} isAdmin={false} />)
    expect(screen.getByRole('article')).toBeInTheDocument()
  })

  it('renders headliner artist name', () => {
    render(<ShowCard show={makeShow()} isAdmin={false} />)
    expect(screen.getByText('Headliner')).toBeInTheDocument()
  })

  it('renders support artist with w/ prefix', () => {
    render(<ShowCard show={makeShow()} isAdmin={false} />)
    expect(screen.getByText('w/')).toBeInTheDocument()
    expect(screen.getByText('Opener')).toBeInTheDocument()
  })

  it('treats first artist as headliner when no is_headliner flags set', () => {
    const show = makeShow({
      artists: [
        makeArtist({ id: 1, name: 'Band A', is_headliner: undefined }),
        makeArtist({ id: 2, name: 'Band B', is_headliner: undefined }),
      ],
    })
    render(<ShowCard show={show} isAdmin={false} />)
    // Band A is shown as headliner (in h2), Band B as support (with w/)
    expect(screen.getByText('Band A')).toBeInTheDocument()
    expect(screen.getByText('w/')).toBeInTheDocument()
    expect(screen.getByText('Band B')).toBeInTheDocument()
  })

  it('shows TBA when no artists', () => {
    const show = makeShow({ artists: [] })
    render(<ShowCard show={show} isAdmin={false} />)
    expect(screen.getByText('TBA')).toBeInTheDocument()
  })

  it('renders venue name as a link', () => {
    render(<ShowCard show={makeShow()} isAdmin={false} />)
    const venueLink = screen.getByText('The Venue')
    expect(venueLink.closest('a')).toHaveAttribute('href', '/venues/the-venue')
  })

  it('renders venue name as plain text when no slug', () => {
    const show = makeShow({
      venues: [
        { id: 1, slug: '', name: 'No Slug Venue', city: 'Phoenix', state: 'AZ', verified: true },
      ],
    })
    render(<ShowCard show={show} isAdmin={false} />)
    const venue = screen.getByText('No Slug Venue')
    expect(venue.closest('a')).toBeNull()
  })

  it('renders city and state', () => {
    render(<ShowCard show={makeShow()} isAdmin={false} />)
    expect(screen.getByText(/Phoenix, AZ/)).toBeInTheDocument()
  })

  it('renders price', () => {
    render(<ShowCard show={makeShow()} isAdmin={false} />)
    expect(screen.getByText('$20')).toBeInTheDocument()
  })

  // A card that showed only the advance half told a reader $35 for a show whose
  // door is $40 (PSY-1962). The compact register is the slash pair; the
  // qualified `$35 ADV · DOOR $40` belongs to the detail page, which has room.
  it('renders both halves of a split price, with the pair spelled out for a screen reader', () => {
    render(
      <ShowCard show={makeShow({ price: 35, door_price: 40 })} isAdmin={false} />
    )
    expect(screen.getByText('$35/$40')).toBeInTheDocument()
    // The spelling a screen reader reaches. `aria-label` on a bare span is
    // ARIA-prohibited, so asserting the attribute would pass against a version
    // that announces "thirty five slash forty".
    expect(screen.getByText('$35 advance, $40 at the door')).toBeInTheDocument()
  })

  // The separator guard has to ask "is a price shown", not "is `price` set".
  // Spelling it `show.price != null` drops the middot for a door-only show and
  // the meta row reads "$15 21+".
  it('keeps the separator between a door-only price and the age requirement', () => {
    render(
      <ShowCard
        show={makeShow({ price: null, door_price: 15, age_requirement: '21+' })}
        isAdmin={false}
      />
    )
    const meta = screen.getByText('$15').parentElement
    expect(meta?.textContent).toContain('·')
  })

  // A door price with no advance price renders bare: with one number there is
  // nothing to tell it apart from.
  it('renders a door-only price bare', () => {
    render(
      <ShowCard
        show={makeShow({ price: null, door_price: 40 })}
        isAdmin={false}
      />
    )
    expect(screen.getByText('$40')).toBeInTheDocument()
  })

  it('renders age requirement', () => {
    render(<ShowCard show={makeShow()} isAdmin={false} />)
    expect(screen.getByText('21+')).toBeInTheDocument()
  })

  it('does not render price when null', () => {
    render(<ShowCard show={makeShow({ price: null })} isAdmin={false} />)
    expect(screen.queryByText('$')).not.toBeInTheDocument()
  })

  it('does not render age requirement when not set', () => {
    render(<ShowCard show={makeShow({ age_requirement: null })} isAdmin={false} />)
    expect(screen.queryByText('21+')).not.toBeInTheDocument()
  })

  it('links artist with slug to artist page', () => {
    render(<ShowCard show={makeShow()} isAdmin={false} />)
    const link = screen.getByText('Headliner').closest('a')
    expect(link).toHaveAttribute('href', '/artists/headliner')
  })

  it('renders artist without slug as plain text', () => {
    const show = makeShow({
      artists: [
        makeArtist({ id: 1, name: 'No Slug Artist', slug: '', is_headliner: true }),
      ],
    })
    render(<ShowCard show={show} isAdmin={false} />)
    const artist = screen.getByText('No Slug Artist')
    expect(artist.closest('a')).toBeNull()
  })

  it('links date badge to show detail page', () => {
    render(<ShowCard show={makeShow()} isAdmin={false} />)
    const links = screen.getAllByRole('link')
    const showLink = links.find(l => l.getAttribute('href') === '/shows/test-show')
    expect(showLink).toBeDefined()
  })

  it('uses show ID in link when slug is missing', () => {
    render(<ShowCard show={makeShow({ slug: '' })} isAdmin={false} />)
    const links = screen.getAllByRole('link')
    const showLink = links.find(l => l.getAttribute('href') === '/shows/1')
    expect(showLink).toBeDefined()
  })

  it('applies cancelled opacity', () => {
    render(<ShowCard show={makeShow({ is_cancelled: true })} isAdmin={false} />)
    const article = screen.getByRole('article')
    expect(article.className).toContain('opacity-60')
  })

  it('does not show admin edit button for non-admin', () => {
    render(<ShowCard show={makeShow()} isAdmin={false} />)
    expect(
      screen.queryByRole('button', { name: /edit show/i })
    ).not.toBeInTheDocument()
  })

  it('shows admin edit button for admin', () => {
    render(<ShowCard show={makeShow()} isAdmin={true} />)
    expect(
      screen.getByRole('button', { name: /edit show/i })
    ).toBeInTheDocument()
  })

  it('toggles inline edit form when admin clicks edit', async () => {
    const user = userEvent.setup()
    render(<ShowCard show={makeShow()} isAdmin={true} />)

    expect(screen.queryByTestId('show-form')).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /edit show/i }))
    expect(screen.getByTestId('show-form')).toBeInTheDocument()

    // Label changes to "Cancel editing"
    await user.click(screen.getByRole('button', { name: /cancel editing/i }))
    expect(screen.queryByTestId('show-form')).not.toBeInTheDocument()
  })

  it('shows delete button for admin', () => {
    render(<ShowCard show={makeShow()} isAdmin={true} />)
    expect(
      screen.getByRole('button', { name: /delete show/i })
    ).toBeInTheDocument()
  })

  it('shows delete button for show owner', () => {
    mockAuthContext.mockReturnValue({
      user: { id: '42', is_admin: false },
      isAuthenticated: true,
      isLoading: false,
      logout: vi.fn(),
    })
    const show = makeShow({ submitted_by: 42 })
    render(<ShowCard show={show} isAdmin={false} userId="42" />)
    expect(
      screen.getByRole('button', { name: /delete show/i })
    ).toBeInTheDocument()
  })

  it('does not show delete button for non-owner non-admin', () => {
    mockAuthContext.mockReturnValue({
      user: { id: '99', is_admin: false },
      isAuthenticated: true,
      isLoading: false,
      logout: vi.fn(),
    })
    const show = makeShow({ submitted_by: 42 })
    render(<ShowCard show={show} isAdmin={false} userId="99" />)
    expect(
      screen.queryByRole('button', { name: /delete show/i })
    ).not.toBeInTheDocument()
  })

  it('opens delete dialog when delete button clicked', async () => {
    const user = userEvent.setup()
    render(<ShowCard show={makeShow()} isAdmin={true} />)

    expect(screen.queryByTestId('delete-dialog')).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /delete show/i }))
    expect(screen.getByTestId('delete-dialog')).toBeInTheDocument()
  })

  it('renders with compact density as a borderless row', () => {
    render(<ShowCard show={makeShow()} isAdmin={false} density="compact" />)
    const article = screen.getByRole('article')
    // Compact mode uses a flat row layout without card borders
    expect(article.className).toContain('hover:bg-muted/50')
    expect(article.className).not.toContain('border')
  })

  it('renders with comfortable density by default', () => {
    render(<ShowCard show={makeShow()} isAdmin={false} />)
    const article = screen.getByRole('article')
    // Comfortable mode uses card layout with border
    expect(article.className).toContain('border')
    expect(article.className).toContain('px-3')
  })

  it('renders with expanded density with more spacious padding', () => {
    render(<ShowCard show={makeShow()} isAdmin={false} density="expanded" />)
    const article = screen.getByRole('article')
    // Expanded mode uses card layout with more generous padding
    expect(article.className).toContain('border')
    expect(article.className).toContain('px-5')
  })

  it('shows export button for admin', () => {
    render(<ShowCard show={makeShow()} isAdmin={true} />)
    expect(screen.getByTestId('export-button')).toBeInTheDocument()
  })

  it('does not show export button for non-admin', () => {
    render(<ShowCard show={makeShow()} isAdmin={false} />)
    expect(screen.queryByTestId('export-button')).not.toBeInTheDocument()
  })

  describe('expand music section', () => {
    it('shows expand button when artist has music', () => {
      const show = makeShow({
        artists: [
          makeArtist({
            id: 1,
            name: 'Band',
            is_headliner: true,
            socials: { spotify: 'https://open.spotify.com/artist/4Z8W4fKeB5YxbusRsdQVPb' },
          }),
        ],
      })
      render(<ShowCard show={show} isAdmin={false} />)
      expect(
        screen.getByRole('button', { name: /discover artist music/i })
      ).toBeInTheDocument()
    })

    // PSY-1966. The expand button promises a music section; a stored value the
    // player refuses to render must not offer one, or the button opens onto
    // nothing.
    it.each([
      'https://evil.test/album/checkout',
      'https://bandcamp.com.attacker.test/album/x',
      'http://band.bandcamp.com/album/test',
    ])('does not show expand button for an unrenderable stored value: %s', (url) => {
      const show = makeShow({
        artists: [
          makeArtist({
            id: 1,
            name: 'Band',
            is_headliner: true,
            bandcamp_embed_url: url,
            socials: {},
          }),
        ],
      })
      render(<ShowCard show={show} isAdmin={false} />)
      expect(
        screen.queryByRole('button', { name: /discover artist music/i })
      ).not.toBeInTheDocument()
    })

    it('shows the expand button for a real release URL', () => {
      const show = makeShow({
        artists: [
          makeArtist({
            id: 1,
            name: 'Band',
            is_headliner: true,
            bandcamp_embed_url: 'https://band.bandcamp.com/album/test',
            socials: {},
          }),
        ],
      })
      render(<ShowCard show={show} isAdmin={false} />)
      expect(
        screen.getByRole('button', { name: /discover artist music/i })
      ).toBeInTheDocument()
    })

    it('does not show expand button when no artist has music', () => {
      const show = makeShow({
        artists: [
          makeArtist({
            id: 1,
            name: 'Band',
            is_headliner: true,
            socials: {},
            bandcamp_embed_url: null,
          }),
        ],
      })
      render(<ShowCard show={show} isAdmin={false} />)
      expect(
        screen.queryByRole('button', { name: /discover artist music/i })
      ).not.toBeInTheDocument()
    })

    it('toggles expanded music section on click', async () => {
      const user = userEvent.setup()
      const show = makeShow({
        artists: [
          makeArtist({
            id: 1,
            name: 'Band',
            is_headliner: true,
            socials: { bandcamp: 'https://band.bandcamp.com' },
          }),
        ],
      })
      render(<ShowCard show={show} isAdmin={false} />)

      expect(screen.queryByTestId('music-embed')).not.toBeInTheDocument()

      await user.click(
        screen.getByRole('button', { name: /discover artist music/i })
      )
      expect(screen.getByTestId('music-embed')).toBeInTheDocument()

      await user.click(
        screen.getByRole('button', { name: /hide artist music/i })
      )
      expect(screen.queryByTestId('music-embed')).not.toBeInTheDocument()
    })

    // The expanded card prints the act's home city right beside the SHOW's
    // city, which is exactly the ambiguity the words remove. Same formatter as
    // the show page, so one act reads the same on both.
    it('prefixes the act home city with "based in"', async () => {
      const user = userEvent.setup()
      const show = makeShow({
        artists: [
          makeArtist({
            id: 1,
            name: 'Band',
            is_headliner: true,
            city: 'Tempe',
            state: 'AZ',
            socials: { bandcamp: 'https://band.bandcamp.com' },
          }),
        ],
      })
      render(<ShowCard show={show} isAdmin={false} />)

      await user.click(
        screen.getByRole('button', { name: /discover artist music/i })
      )
      expect(screen.getByText('based in Tempe, AZ')).toBeInTheDocument()
    })

    // Same parts rule as the show page: country included unless the state is
    // set and the country is USA/US. Sharing only the prefix would have one
    // act read two ways across the card and the page it opens.
    it('obeys the shared location rule for an act outside the US', async () => {
      const user = userEvent.setup()
      const show = makeShow({
        artists: [
          makeArtist({
            id: 1,
            name: 'Band',
            is_headliner: true,
            city: 'Melbourne',
            state: '',
            country: 'Australia',
            socials: { bandcamp: 'https://band.bandcamp.com' },
          }),
        ],
      })
      render(<ShowCard show={show} isAdmin={false} />)

      await user.click(
        screen.getByRole('button', { name: /discover artist music/i })
      )
      expect(
        screen.getByText('based in Melbourne, Australia')
      ).toBeInTheDocument()
    })

    // COUNTRY ALONE now counts, because the shared rule counts it. This card
    // printed nothing for such an act before it adopted `billHometown`.
    it('states a country-only location', async () => {
      const user = userEvent.setup()
      const show = makeShow({
        artists: [
          makeArtist({
            id: 1,
            name: 'Band',
            is_headliner: true,
            city: '',
            state: '',
            country: 'Japan',
            socials: { bandcamp: 'https://band.bandcamp.com' },
          }),
        ],
      })
      render(<ShowCard show={show} isAdmin={false} />)

      await user.click(
        screen.getByRole('button', { name: /discover artist music/i })
      )
      expect(screen.getByText('based in Japan')).toBeInTheDocument()
    })

    it('prints nothing for an act with no placeable location at all', async () => {
      const user = userEvent.setup()
      const show = makeShow({
        artists: [
          makeArtist({
            id: 1,
            name: 'Band',
            is_headliner: true,
            city: '',
            state: '',
            country: '',
            socials: { bandcamp: 'https://band.bandcamp.com' },
          }),
        ],
      })
      render(<ShowCard show={show} isAdmin={false} />)

      await user.click(
        screen.getByRole('button', { name: /discover artist music/i })
      )
      expect(screen.queryByText(/based in/)).not.toBeInTheDocument()
    })
  })

  describe('the clock is withheld on a guessed zone', () => {
    // 03:00Z is the previous evening in the fallback zone (UTC-7), so a card
    // reading this row on the guess names both a wrong hour and a wrong day.
    const GUESSED = {
      event_date: '2026-09-10T03:00:00Z',
      state: '',
      venues: [
        {
          id: 1,
          slug: 'hall-ohne-zone',
          name: 'Hall Ohne Zone',
          city: 'Berlin',
          state: '',
          verified: true,
        },
      ],
    }

    // The clock the row would have printed before the gate existed, and the one
    // a control row at a zoned venue still prints.
    const FALLBACK_CLOCK = /8:00\s?PM/

    it.each(['default', 'compact', 'expanded'] as const)(
      'names no hour at %s density',
      density => {
        render(
          <ShowCard
            show={makeShow(GUESSED)}
            isAdmin={false}
            density={density === 'default' ? undefined : density}
          />
        )
        expect(screen.queryByText(FALLBACK_CLOCK)).not.toBeInTheDocument()
      }
    )

    it.each(['default', 'compact', 'expanded'] as const)(
      'still names the hour at %s density when the venue carries a zone',
      density => {
        render(
          <ShowCard
            show={makeShow({
              ...GUESSED,
              venues: [{ ...GUESSED.venues[0], timezone: 'America/Phoenix' }],
            })}
            isAdmin={false}
            density={density === 'default' ? undefined : density}
          />
        )
        expect(screen.getByText(FALLBACK_CLOCK)).toBeInTheDocument()
      }
    )

    it('withholds the hour for a state outside the US map', () => {
      render(
        <ShowCard
          show={makeShow({
            ...GUESSED,
            venues: [{ ...GUESSED.venues[0], state: 'England' }],
          })}
          isAdmin={false}
        />
      )
      expect(screen.queryByText(FALLBACK_CLOCK)).not.toBeInTheDocument()
    })

    it('keeps the date badge on the row whose time it withheld', () => {
      render(<ShowCard show={makeShow(GUESSED)} isAdmin={false} />)
      expect(screen.getByText(/SEP 9/i)).toBeInTheDocument()
    })

    it('keeps the price, which the zone has nothing to do with', () => {
      render(<ShowCard show={makeShow(GUESSED)} isAdmin={false} />)
      expect(screen.getAllByText('$20').length).toBeGreaterThan(0)
    })
  })

  describe('multiple headliners', () => {
    it('renders multiple headliners separated by bullets', () => {
      const show = makeShow({
        artists: [
          makeArtist({ id: 1, name: 'Band A', is_headliner: true }),
          makeArtist({ id: 2, name: 'Band B', is_headliner: true }),
        ],
      })
      render(<ShowCard show={show} isAdmin={false} />)
      expect(screen.getByText('Band A')).toBeInTheDocument()
      expect(screen.getByText('Band B')).toBeInTheDocument()
    })

    it('does not show w/ section when no support acts', () => {
      const show = makeShow({
        artists: [
          makeArtist({ id: 1, name: 'Solo', is_headliner: true }),
        ],
      })
      render(<ShowCard show={show} isAdmin={false} />)
      expect(screen.queryByText('w/')).not.toBeInTheDocument()
    })
  })
})
