import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import { ShowListenModule } from './ShowListenModule'
import type { ArtistResponse } from '../types'

// The player itself is a third-party iframe with its own resolve query; this
// suite is about the chrome the module owns around it. Mocked at the leaf
// module rather than the barrel so BracketLink / ShareButton / SectionHeader
// stay REAL — their accessible names are half of what is under test here.
vi.mock('@/components/shared/MusicEmbed', () => ({
  // Re-declared because the barrel re-exports it from this module, and the
  // module under test sizes its card list from it.
  BANDCAMP_EMBED_MAX_WIDTH_PX: 700,
  MusicEmbed: ({
    artistName,
    bandcampAlbumUrl,
    spotifyUrl,
    compact,
  }: {
    artistName: string
    bandcampAlbumUrl?: string | null
    spotifyUrl?: string | null
    compact?: boolean
  }) => (
    <div
      data-testid="music-embed"
      data-artist={artistName}
      data-bandcamp={bandcampAlbumUrl ?? ''}
      data-spotify={spotifyUrl ?? ''}
      data-compact={String(!!compact)}
    />
  ),
}))

const SPOTIFY_ARTIST = 'https://open.spotify.com/artist/1a2b3c4d5e6f7g8h9i0jkl'
const BANDCAMP_ALBUM = 'https://band.bandcamp.com/album/an-eraser-and-a-maze'

// jsdom ships neither `navigator.share` nor `navigator.clipboard`, and
// ShareButton renders NOTHING when it has neither. Give it a clipboard so the
// share bracket exists to be asserted on; ShareButton.test covers the
// capability matrix itself.
beforeEach(() => {
  Object.defineProperty(navigator, 'clipboard', {
    value: { writeText: vi.fn().mockResolvedValue(undefined) },
    configurable: true,
    writable: true,
  })
})

function makeArtist(overrides: Partial<ArtistResponse> = {}): ArtistResponse {
  return {
    id: 1,
    slug: 'modest-mouse',
    name: 'Modest Mouse',
    set_type: 'headliner',
    position: 0,
    socials: {},
    ...overrides,
  }
}

function cards() {
  return within(screen.getByTestId('show-listen-module')).getAllByRole(
    'listitem'
  )
}

/**
 * The meta line of one card. Addressed by the artist's id because the testid is
 * per-card: a bare `getByTestId` would throw the moment a case renders the
 * two-act bill this module actually exists for.
 */
function meta(artistId = 1) {
  return screen.getByTestId(`listen-card-meta-${artistId}`)
}

describe('ShowListenModule', () => {
  it('renders nothing when no bill artist has a playable source', () => {
    const { container } = render(<ShowListenModule artists={[makeArtist()]} lifecycle="upcoming" />)
    expect(container).toBeEmptyDOMElement()
  })

  it('renders the mock section label above a card per playable artist', () => {
    render(
      <ShowListenModule
        artists={[
          makeArtist({ bandcamp_embed_url: BANDCAMP_ALBUM }),
          makeArtist({
            id: 2,
            slug: 'califone',
            name: 'Califone',
            position: 1,
            socials: { spotify: SPOTIFY_ARTIST },
          }),
          // No playable source: contributes no card rather than an empty one.
          makeArtist({ id: 3, slug: 'quiet', name: 'Quiet', position: 2 }),
        ]}
        lifecycle="upcoming"
      />
    )

    expect(
      screen.getByRole('heading', { name: 'Listen / Before you go' })
    ).toBeInTheDocument()
    expect(cards()).toHaveLength(2)
    expect(screen.getAllByTestId('music-embed')).toHaveLength(2)
  })

  // "Before you go" instructs a reader who can still go, so it does not
  // survive the show. Only the HEADING changes: the cards and their open
  // players are the archive's whole point and are identical in both states.
  it('drops the forward-looking qualifier on a past show', () => {
    const bill = [makeArtist({ bandcamp_embed_url: BANDCAMP_ALBUM })]
    const { rerender } = render(
      <ShowListenModule artists={bill} lifecycle="past" />
    )

    expect(screen.getByRole('heading', { name: 'Listen' })).toBeInTheDocument()
    expect(
      screen.queryByRole('heading', { name: /Before you go/ })
    ).not.toBeInTheDocument()
    const pastCards = cards().length
    const pastPlayers = screen.getAllByTestId('music-embed').length

    rerender(<ShowListenModule artists={bill} lifecycle="upcoming" />)
    expect(cards()).toHaveLength(pastCards)
    expect(screen.getAllByTestId('music-embed')).toHaveLength(pastPlayers)
  })

  // The running order itself (position sort, then the curated-headliner hoist)
  // is pinned in showListenCards.test.ts. This only checks that the module
  // renders the cards in the order it is handed them.
  it('renders cards in the order the bill derivation returns them', () => {
    render(
      <ShowListenModule
        artists={[
          makeArtist({
            id: 2,
            slug: 'califone',
            name: 'Califone',
            position: 1,
            bandcamp_embed_url: 'https://califone.bandcamp.com/album/heron',
          }),
          makeArtist({ bandcamp_embed_url: BANDCAMP_ALBUM }),
        ]}
        lifecycle="upcoming"
      />
    )

    expect(
      cards().map(card => card.querySelector('[data-testid="music-embed"]')?.getAttribute('data-artist'))
    ).toEqual(['Modest Mouse', 'Califone'])
  })

  it('names the artist, the source, and both verbs on a Bandcamp card', () => {
    render(
      <ShowListenModule
        artists={[makeArtist({ bandcamp_embed_url: BANDCAMP_ALBUM })]}
        lifecycle="upcoming"
      />
    )

    const card = meta()
    expect(
      within(card).getByRole('link', { name: 'Modest Mouse' })
    ).toHaveAttribute('href', '/artists/modest-mouse')
    expect(card).toHaveTextContent('Bandcamp')

    // BracketLink owns the outbound announcement; the caller never writes it.
    const buy = within(card).getByRole('link', {
      name: 'Buy Modest Mouse on Bandcamp (opens in a new tab)',
    })
    expect(buy).toHaveAttribute('href', BANDCAMP_ALBUM)
    expect(buy).toHaveAttribute('target', '_blank')
    expect(buy).toHaveAttribute('rel', 'noopener noreferrer')

    expect(
      within(card).getByRole('button', { name: 'Share Modest Mouse' })
    ).toBeInTheDocument()
  })

  it('offers no buy bracket for a Spotify card', () => {
    render(
      <ShowListenModule
        artists={[makeArtist({ socials: { spotify: SPOTIFY_ARTIST } })]}
        lifecycle="upcoming"
      />
    )

    const card = meta()
    expect(card).toHaveTextContent('Spotify')
    expect(within(card).queryByRole('link', { name: /^Buy/ })).toBeNull()
    expect(
      within(card).getByRole('button', { name: 'Share Modest Mouse' })
    ).toBeInTheDocument()
  })

  it('renders no card for a bare Bandcamp profile', () => {
    // A profile has no player behind it, only an outbound link, and a link
    // wearing the same border as the players above it is a card that
    // misrepresents itself.
    const { container } = render(
      <ShowListenModule
        artists={[
          makeArtist({ socials: { bandcamp: 'https://band.bandcamp.com' } }),
        ]}
        lifecycle="upcoming"
      />
    )
    expect(container).toBeEmptyDOMElement()
  })

  it('drops the whole verb cluster rather than trail a separator', () => {
    // Neither bracket can render here: Spotify sells nothing, and an empty slug
    // means ShareButton gets a null path. MiddotSegments puts a separator
    // BETWEEN whatever it is handed, so a verb segment that renders empty would
    // leave the line ending in a middot.
    render(
      <ShowListenModule
        artists={[
          makeArtist({ slug: '', socials: { spotify: SPOTIFY_ARTIST } }),
        ]}
        lifecycle="upcoming"
      />
    )

    const card = meta()
    expect(within(card).queryByRole('link', { name: /^Buy/ })).toBeNull()
    expect(within(card).queryByRole('button', { name: /^Share/ })).toBeNull()
    expect(card.textContent?.trim()).toBe('Modest Mouse · Spotify')
  })

  it('renders a slugless artist as text and drops its share bracket', () => {
    // `/artists/` resolves to the INDEX, not a 404, so an empty slug must never
    // reach an href — neither the name link nor the share URL.
    render(
      <ShowListenModule
        artists={[makeArtist({ slug: '', bandcamp_embed_url: BANDCAMP_ALBUM })]}
        lifecycle="upcoming"
      />
    )

    const card = meta()
    expect(within(card).queryByRole('link', { name: 'Modest Mouse' })).toBeNull()
    expect(card).toHaveTextContent('Modest Mouse')
    expect(within(card).queryByRole('button', { name: /^Share/ })).toBeNull()
    // The buy bracket is independent of the slug and must survive.
    expect(
      within(card).getByRole('link', { name: /^Buy Modest Mouse/ })
    ).toBeInTheDocument()
  })

  // Restored against the mock, which draws neither, by owner decision
  // 2026-08-30. Both are absence-gated, so both halves are pinned.
  describe('hometown', () => {
    it('states where the act is from, on the meta line', () => {
      render(
        <ShowListenModule
          artists={[
            makeArtist({
              bandcamp_embed_url: BANDCAMP_ALBUM,
              city: 'Issaquah',
              state: 'WA',
              country: 'USA',
            }),
          ]}
          lifecycle="upcoming"
        />
      )

      // "USA" suppressed by the locked display rule: country is included
      // UNLESS the state is set and the country is USA/US.
      expect(meta()).toHaveTextContent(
        'Modest Mouse · from Issaquah, WA · Bandcamp'
      )
    })

    it('states the country for an act outside the US', () => {
      render(
        <ShowListenModule
          artists={[
            makeArtist({
              bandcamp_embed_url: BANDCAMP_ALBUM,
              city: 'Melbourne',
              country: 'Australia',
            }),
          ]}
          lifecycle="upcoming"
        />
      )

      expect(meta()).toHaveTextContent('from Melbourne, Australia')
    })

    it('renders no hometown segment for an act with nothing placeable', () => {
      // Not "Location Unknown": the placeholder stands alone in a location
      // field, and mid-line it states something the card was not asked to
      // state. The line must close up rather than carry an empty segment.
      render(
        <ShowListenModule
          artists={[makeArtist({ bandcamp_embed_url: BANDCAMP_ALBUM })]}
          lifecycle="upcoming"
        />
      )

      const card = meta()
      expect(card).not.toHaveTextContent('Location Unknown')
      expect(card).not.toHaveTextContent('from')
      expect(card.textContent).toContain('Modest Mouse · Bandcamp')
    })
  })

  describe('social links', () => {
    it("carries the act's own social links", () => {
      render(
        <ShowListenModule
          artists={[
            makeArtist({
              bandcamp_embed_url: BANDCAMP_ALBUM,
              socials: {
                instagram: 'https://instagram.com/modestmouse',
                website: 'https://modestmouse.com',
              },
            }),
          ]}
          lifecycle="upcoming"
        />
      )

      const card = cards()[0]
      // Named by the platform, not by the icon: the glyph is `aria-hidden` and
      // the sr-only label is the whole accessible name.
      expect(within(card).getByRole('link', { name: 'Instagram' })).toHaveAttribute(
        'href',
        'https://instagram.com/modestmouse'
      )
      expect(within(card).getByRole('link', { name: 'Website' })).toHaveAttribute(
        'href',
        'https://modestmouse.com'
      )
    })

    it('renders no social links for an act with none', () => {
      render(
        <ShowListenModule
          artists={[makeArtist({ bandcamp_embed_url: BANDCAMP_ALBUM })]}
          lifecycle="upcoming"
        />
      )

      // `bandcamp_embed_url` is a release URL, not a social; an act whose only
      // music link is that column has an empty socials object and must get no
      // icon row rather than an empty one.
      const card = cards()[0]
      expect(within(card).queryByRole('link', { name: 'Instagram' })).toBeNull()
      expect(within(card).queryByRole('link', { name: 'Bandcamp' })).toBeNull()
    })
  })

  it('loads every player open and compact, with no activation step', () => {
    // Locked decision 9: no facade, no click-to-load. Every card ships a real
    // embed on first render, and `compact` suppresses MusicEmbed's own "Music"
    // heading so the section header is the only one.
    render(
      <ShowListenModule
        artists={[
          makeArtist({ bandcamp_embed_url: BANDCAMP_ALBUM }),
          makeArtist({
            id: 2,
            slug: 'califone',
            name: 'Califone',
            position: 1,
            socials: { spotify: SPOTIFY_ARTIST },
          }),
        ]}
        lifecycle="upcoming"
      />
    )

    const embeds = screen.getAllByTestId('music-embed')
    expect(embeds).toHaveLength(2)
    for (const embed of embeds) {
      expect(embed).toHaveAttribute('data-compact', 'true')
    }
    expect(embeds[0]).toHaveAttribute('data-bandcamp', BANDCAMP_ALBUM)
    expect(embeds[1]).toHaveAttribute('data-spotify', SPOTIFY_ARTIST)
    // The only buttons in the module are the share brackets. Anything else
    // sitting between the reader and the audio would be the facade.
    expect(
      within(screen.getByTestId('show-listen-module'))
        .getAllByRole('button')
        .map(button => button.getAttribute('aria-label'))
    ).toEqual(['Share Modest Mouse', 'Share Califone'])
  })
})
