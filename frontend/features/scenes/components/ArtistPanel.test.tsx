import { describe, it, expect, vi, beforeEach } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import type { ArtistGraphCard } from '@/features/artists/types'
import type { ArtistStep } from '../artistDrillIn'
import { ARTIST_SHOWS_PAGE_LIMIT } from '@/features/artists/api'
import { ARTIST_PANEL_NEXT_SHOW_ROWS } from '../cityView'

vi.mock('next/link', () => ({
  default: ({ href, children }: { href: string; children: ReactNode }) => (
    <a href={href}>{children}</a>
  ),
}))

// MusicEmbed owns a TanStack query + a third-party iframe; the panel's job is
// deciding WHETHER to head a Listen section, so the embed is mocked at the
// module boundary and its props asserted instead.
vi.mock('@/components/shared/MusicEmbed', () => ({
  MusicEmbed: ({
    bandcampAlbumUrl,
    spotifyUrl,
  }: {
    bandcampAlbumUrl?: string | null
    spotifyUrl?: string | null
  }) => (
    <div
      data-testid="music-embed"
      data-bandcamp={bandcampAlbumUrl ?? ''}
      data-spotify={spotifyUrl ?? ''}
    />
  ),
}))

const mockUseArtistGraphCard =
  vi.fn<(args: { artistId: number | string | null }) => Record<string, unknown>>(
    () => ({ data: undefined, isError: false }),
  )
vi.mock('@/features/artists/hooks/useArtistGraphCard', () => ({
  useArtistGraphCard: (args: { artistId: number | string | null }) =>
    mockUseArtistGraphCard(args),
}))

const mockUseArtistShows = vi.fn<(args: unknown) => Record<string, unknown>>(
  () => ({ data: { shows: [], artist_id: 0, total: 0 } }),
)
vi.mock('@/features/artists/hooks/useArtists', () => ({
  useArtistShows: (args: unknown) => mockUseArtistShows(args),
}))

import { ArtistPanel } from './ArtistPanel'

const STEPS: ArtistStep[] = [
  { artistId: 10, artistSlug: 'die-spitz', artistName: 'Die Spitz', showId: 1 },
  {
    artistId: 11,
    artistSlug: 'farmers-wife',
    artistName: 'Farmer’s Wife',
    showId: 1,
  },
  {
    artistId: 12,
    artistSlug: 'gouge-away',
    artistName: 'Gouge Away',
    showId: 2,
  },
]

function card(overrides: Partial<ArtistGraphCard> = {}): ArtistGraphCard {
  return {
    id: 10,
    name: 'Die Spitz',
    slug: 'die-spitz',
    city: 'Austin',
    state: 'TX',
    bandcamp_embed_url: null,
    spotify: null,
    next_show: null,
    labels: [],
    radio: null,
    connections: { bills: 0, similar: 0, members: 0, radio: 0, shared_labels: 0 },
    ...overrides,
  } as ArtistGraphCard
}

function renderPanel(
  props: Partial<React.ComponentProps<typeof ArtistPanel>> = {},
) {
  const defaults: React.ComponentProps<typeof ArtistPanel> = {
    steps: STEPS,
    index: 0,
    onStep: vi.fn(),
    scopeLabel: 'upcoming at Hotel Vegas',
    backLabel: 'Hotel Vegas',
    onBack: vi.fn(),
    onClose: vi.fn(),
  }
  const merged = { ...defaults, ...props }
  return { ...render(<ArtistPanel {...merged} />), props: merged }
}

beforeEach(() => {
  mockUseArtistGraphCard.mockReset()
  mockUseArtistGraphCard.mockReturnValue({ data: undefined, isError: false })
  mockUseArtistShows.mockReset()
  mockUseArtistShows.mockReturnValue({
    data: { shows: [], artist_id: 10, total: 0 },
  })
})

describe('ArtistPanel', () => {
  it('renders the step’s artist before the card lands', () => {
    renderPanel()
    expect(
      screen.getByRole('heading', { name: 'Die Spitz' }),
    ).toBeInTheDocument()
    expect(screen.getByTestId('artist-panel-kicker')).toHaveTextContent(
      'ARTIST · 1 OF 3 UPCOMING AT HOTEL VEGAS',
    )
  })

  it('fetches the card for the CURRENT step and prefetches the next', () => {
    renderPanel({ index: 0 })
    const requested = mockUseArtistGraphCard.mock.calls.map((c) => c[0].artistId)
    expect(requested).toEqual([10, 11])
  })

  // Metadata one step ahead, media on demand: prefetching the iframe would
  // leave a hidden player resident per step of a walk down the bill.
  it('does not mount a player for the prefetched next artist', () => {
    mockUseArtistGraphCard.mockImplementation(({ artistId }) =>
      artistId === 10
        ? { data: card({ bandcamp_embed_url: 'https://x.bandcamp.com/album/a' }), isError: false }
        : { data: card({ id: 11, bandcamp_embed_url: 'https://y.bandcamp.com/album/b' }), isError: false },
    )
    renderPanel()
    expect(screen.getAllByTestId('music-embed')).toHaveLength(1)
  })

  it('stops prefetching at the end of the list', () => {
    renderPanel({ index: 2 })
    const requested = mockUseArtistGraphCard.mock.calls.map((c) => c[0].artistId)
    expect(requested).toEqual([12, null])
  })

  describe('the LISTEN section', () => {
    it('renders a player for a Bandcamp embed URL', () => {
      mockUseArtistGraphCard.mockReturnValue({
        data: card({ bandcamp_embed_url: 'https://diespitz.bandcamp.com/album/x' }),
        isError: false,
      })
      renderPanel()
      expect(screen.getByTestId('artist-panel-listen')).toBeInTheDocument()
      expect(screen.getByTestId('music-embed')).toHaveAttribute(
        'data-bandcamp',
        'https://diespitz.bandcamp.com/album/x',
      )
    })

    // PSY-1966: a stored value MusicEmbed refuses to render must not open the
    // headed LISTEN block. The case that fails if the gate reverts to
    // Boolean(card.bandcamp_embed_url).
    it.each([
      'https://evil.test/album/checkout',
      'https://bandcamp.com.attacker.test/album/x',
      'http://diespitz.bandcamp.com/album/x',
    ])('renders no LISTEN block for an unrenderable embed URL: %s', (url) => {
      mockUseArtistGraphCard.mockReturnValue({
        data: card({ bandcamp_embed_url: url, spotify: null }),
        isError: false,
      })
      renderPanel()
      expect(screen.queryByTestId('artist-panel-listen')).not.toBeInTheDocument()
      expect(screen.queryByTestId('music-embed')).not.toBeInTheDocument()
    })

    it('renders a player for an embeddable Spotify link', () => {
      mockUseArtistGraphCard.mockReturnValue({
        data: card({
          spotify: 'https://open.spotify.com/artist/1vCWHaC5f2uS3yhpwWbIA6',
        }),
        isError: false,
      })
      renderPanel()
      expect(screen.getByTestId('artist-panel-listen')).toBeInTheDocument()
    })

    // The failure mode the gate exists to prevent: a headed section with no
    // player under it. Mirrors ArtistContextPanel's hasPlayableAudio exactly.
    it('is absent entirely when the artist has no playable audio', () => {
      mockUseArtistGraphCard.mockReturnValue({ data: card(), isError: false })
      renderPanel()
      expect(screen.queryByTestId('artist-panel-listen')).not.toBeInTheDocument()
      expect(screen.queryByText('Listen')).not.toBeInTheDocument()
    })

    it('is absent for a Spotify link that does not parse to an embeddable id', () => {
      mockUseArtistGraphCard.mockReturnValue({
        data: card({ spotify: 'https://open.spotify.example.com/artist/evil' }),
        isError: false,
      })
      renderPanel()
      expect(screen.queryByTestId('artist-panel-listen')).not.toBeInTheDocument()
    })

    it('is absent while the card is still loading', () => {
      renderPanel()
      expect(screen.queryByTestId('artist-panel-listen')).not.toBeInTheDocument()
    })
  })

  describe('the stepper', () => {
    it('steps forward and back through the originating list', () => {
      const onStep = vi.fn()
      renderPanel({ index: 1, onStep })
      fireEvent.click(screen.getByTestId('artist-panel-step-next'))
      expect(onStep).toHaveBeenCalledWith(2)
      fireEvent.click(screen.getByTestId('artist-panel-step-previous'))
      expect(onStep).toHaveBeenCalledWith(0)
    })

    // aria-disabled, NOT the native attribute: `disabled` takes the control out
    // of the tab order, so a keyboard user never learns the stepper exists or
    // why it does nothing (PSY-1540's review found exactly this).
    it('keeps edge controls focusable and explains why they are inert', () => {
      const onStep = vi.fn()
      renderPanel({ index: 0, onStep })
      const previous = screen.getByTestId('artist-panel-step-previous')
      expect(previous).toHaveAttribute('aria-disabled', 'true')
      expect(previous).not.toHaveAttribute('disabled')
      expect(previous).toHaveAccessibleName(
        'Previous artist — already at the first',
      )
      fireEvent.click(previous)
      expect(onStep).not.toHaveBeenCalled()
    })

    it('marks the forward control inert on the last step', () => {
      renderPanel({ index: 2 })
      expect(screen.getByTestId('artist-panel-step-next')).toHaveAccessibleName(
        'Next artist — already at the last',
      )
    })

    // Stepping deliberately does NOT move focus, so this live region is the
    // only signal a screen-reader user gets that the panel changed.
    it('announces the new position and artist in a live region', () => {
      renderPanel({ index: 1 })
      expect(screen.getByRole('status')).toHaveTextContent(
        'Farmer’s Wife, artist 2 of 3 upcoming at Hotel Vegas',
      )
    })

    it('hides the stepper for a single-entry list', () => {
      renderPanel({ steps: [STEPS[0]], index: 0 })
      expect(
        screen.queryByTestId('artist-panel-step-next'),
      ).not.toBeInTheDocument()
      expect(screen.getByTestId('artist-panel-kicker')).toHaveTextContent(
        'ARTIST',
      )
    })

    // The stepper is the ONLY forward affordance. The panel used to also end
    // with a "NEXT UP <name> — hear them →" row, removed because it implied a
    // click was required to hear music — the current artist's player is
    // already visible above it — and duplicated these controls. Nothing may
    // name the next artist as a second way forward.
    it('is the only forward affordance — no NEXT UP row', () => {
      renderPanel({ index: 0 })
      expect(screen.queryByText(/hear them/)).not.toBeInTheDocument()
      expect(screen.queryByText(/Next up/i)).not.toBeInTheDocument()
      expect(
        screen.queryByRole('button', { name: /Farmer’s Wife/ }),
      ).not.toBeInTheDocument()
    })
  })

  describe('dismissal', () => {
    // Escape pops ONE level: back to the venue panel, not out of the Atlas.
    it('pops one level on Escape', () => {
      const onBack = vi.fn()
      const onClose = vi.fn()
      renderPanel({ onBack, onClose })
      fireEvent.keyDown(document, { key: 'Escape' })
      expect(onBack).toHaveBeenCalledTimes(1)
      expect(onClose).not.toHaveBeenCalled()
    })

    it('returns to the originating panel from the breadcrumb', () => {
      const onBack = vi.fn()
      renderPanel({ onBack })
      fireEvent.click(screen.getByRole('button', { name: /Hotel Vegas/ }))
      expect(onBack).toHaveBeenCalledTimes(1)
    })

    it('closes the whole stack from the ✕', () => {
      const onClose = vi.fn()
      const onBack = vi.fn()
      renderPanel({ onClose, onBack })
      fireEvent.click(
        screen.getByRole('button', { name: 'Close Die Spitz panel' }),
      )
      expect(onClose).toHaveBeenCalledTimes(1)
      expect(onBack).not.toHaveBeenCalled()
    })

    // The panel opens from a show ROW; a keyboard user left standing there
    // would have to tab past every remaining show to reach what they opened.
    it('focuses the breadcrumb on open', () => {
      renderPanel()
      expect(screen.getByRole('button', { name: /Hotel Vegas/ })).toHaveFocus()
    })
  })

  describe('NEXT SHOWS', () => {
    it('lists the artist’s upcoming shows with venue and co-headliners', () => {
      mockUseArtistShows.mockReturnValue({
        data: {
          shows: [
            {
              id: 900,
              slug: 's',
              title: '',
              event_date: '2026-08-02T02:00:00Z',
              price: null,
              age_requirement: null,
              venue: {
                id: 1,
                slug: 'here',
                name: 'HERE',
                city: 'Austin',
                state: 'TX',
                timezone: 'America/Chicago',
              },
              artists: [
                { id: 10, slug: 'die-spitz', name: 'Die Spitz' },
                { id: 11, slug: 'farmers-wife', name: 'Farmer’s Wife' },
              ],
            },
          ],
          artist_id: 10,
          total: 1,
        },
      })
      renderPanel()
      expect(screen.getByText(/HERE/)).toHaveTextContent(
        'SAT 8/1 · HERE · w/ Farmer’s Wife · 9:00 PM',
      )
    })

    // The row joins its parts with a middot, so a withheld clock has to take
    // its separator with it rather than leaving a trailing "· ".
    it('names no hour when the venue zone is a guess, and no trailing separator', () => {
      mockUseArtistShows.mockReturnValue({
        data: {
          shows: [
            {
              id: 900,
              slug: 's',
              title: '',
              event_date: '2026-08-02T02:00:00Z',
              price: null,
              age_requirement: null,
              venue: {
                id: 1,
                slug: 'hall',
                name: 'HALL',
                city: 'Berlin',
                state: '',
                timezone: null,
              },
              artists: [{ id: 10, slug: 'die-spitz', name: 'Die Spitz' }],
            },
          ],
          artist_id: 10,
          total: 1,
        },
      })
      renderPanel()
      expect(screen.getByText(/HALL/)).toHaveTextContent('SAT 8/1 · HALL')
      expect(screen.queryByText(/PM/)).not.toBeInTheDocument()
    })

    // Requests a full page and slices for display, rather than a two-row page.
    //
    // Originally a correctness requirement: `artistQueryKeys.shows()` keyed
    // only on artist id + time filter, so a two-row request here handed the
    // artist page a two-row list for the whole 5-minute staleTime whenever a
    // reader arrived via "Open artist page →". PSY-1754 put every
    // response-shaping param in the key, so it is now a warm-cache trade
    // instead — but the panel must still not silently ask for a partial page,
    // which is what this pins.
    it('requests a full page of shows and slices it for display', () => {
      renderPanel()
      expect(mockUseArtistShows).toHaveBeenCalledWith(
        expect.objectContaining({
          artistId: 10,
          timeFilter: 'upcoming',
          limit: ARTIST_SHOWS_PAGE_LIMIT,
        }),
      )
      expect(ARTIST_SHOWS_PAGE_LIMIT).toBeGreaterThan(ARTIST_PANEL_NEXT_SHOW_ROWS)
    })

    it('still draws only the two rows the mock calls for', () => {
      mockUseArtistShows.mockReturnValue({
        data: {
          shows: Array.from({ length: 9 }, (_, i) => ({
            id: 900 + i,
            slug: `s${i}`,
            title: '',
            event_date: '2026-08-02T02:00:00Z',
            price: null,
            age_requirement: null,
            venue: {
              id: 1,
              slug: 'here',
              name: `Venue ${i}`,
              city: 'Austin',
              state: 'TX',
              timezone: 'America/Chicago',
            },
            artists: [],
          })),
          artist_id: 10,
          total: 9,
        },
      })
      renderPanel()
      // The rows render as one joined line ("SAT 8/1 · Venue 0 · 9:00 PM"),
      // so match on a substring rather than the venue name alone.
      expect(screen.getByText(/Venue 0/)).toBeInTheDocument()
      expect(screen.getByText(/Venue 1/)).toBeInTheDocument()
      expect(screen.queryByText(/Venue 2/)).not.toBeInTheDocument()
    })

    // No route error boundary on /atlas — an absent bill must degrade, not
    // take down the app shell.
    it('survives a show served without a bill or a venue', () => {
      mockUseArtistShows.mockReturnValue({
        data: {
          shows: [
            {
              id: 901,
              slug: 's',
              title: '',
              event_date: 'not-a-date',
              price: null,
              age_requirement: null,
              venue: null,
              artists: undefined,
            },
          ],
          artist_id: 10,
          total: 1,
        },
      })
      expect(() => renderPanel()).not.toThrow()
      expect(screen.queryByText(/Invalid Date/)).not.toBeInTheDocument()
    })
  })

  describe('degradation', () => {
    it('keeps a path out when the card fetch fails', () => {
      mockUseArtistGraphCard.mockReturnValue({ data: undefined, isError: true })
      renderPanel()
      expect(screen.getByText(/Details couldn’t load/)).toBeInTheDocument()
      expect(
        screen.getByRole('link', { name: 'Open artist page →' }),
      ).toHaveAttribute('href', '/artists/die-spitz')
    })

    it('falls back to the artist id when the step carries no slug', () => {
      renderPanel({
        steps: [{ ...STEPS[0], artistSlug: '' }],
        index: 0,
      })
      expect(
        screen.getByRole('link', { name: 'Open artist page →' }),
      ).toHaveAttribute('href', '/artists/10')
    })

    it('renders nothing rather than empty chrome for an empty list', () => {
      const { container } = renderPanel({ steps: [], index: 0 })
      expect(container).toBeEmptyDOMElement()
    })

    it('shows CONNECTIONS only when there is something to say', () => {
      mockUseArtistGraphCard.mockReturnValue({ data: card(), isError: false })
      const { unmount } = renderPanel()
      expect(screen.queryByText('Connections')).not.toBeInTheDocument()
      unmount()

      mockUseArtistGraphCard.mockReturnValue({
        data: card({
          connections: { bills: 14, similar: 6, members: 0, radio: 0, shared_labels: 0 },
        }),
        isError: false,
      })
      renderPanel()
      expect(screen.getByText('14 bills · 6 similar artists')).toBeInTheDocument()
    })
  })
})
