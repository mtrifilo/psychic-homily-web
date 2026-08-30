import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import { ShowListenModule } from './ShowListenModule'
import type { ArtistResponse } from '../types'

// The player itself is a third-party iframe with its own resolve query; this
// suite is about the chrome the module owns around it. Mocked at the leaf
// module rather than the barrel so BracketLink / ShareButton / SectionHeader
// stay REAL — their accessible names are half of what is under test here.
vi.mock('@/components/shared/MusicEmbed', () => ({
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

describe('ShowListenModule', () => {
  it('renders nothing when no bill artist has a playable source', () => {
    const { container } = render(<ShowListenModule artists={[makeArtist()]} />)
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
      />
    )

    expect(
      screen.getByRole('heading', { name: 'Listen / Before you go' })
    ).toBeInTheDocument()
    expect(cards()).toHaveLength(2)
    expect(screen.getAllByTestId('music-embed')).toHaveLength(2)
  })

  it('orders cards by bill position, headliner first', () => {
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
      />
    )

    const meta = screen.getByTestId('listen-card-meta')
    expect(
      within(meta).getByRole('link', { name: 'Modest Mouse' })
    ).toHaveAttribute('href', '/artists/modest-mouse')
    expect(meta).toHaveTextContent('Bandcamp')

    // BracketLink owns the outbound announcement; the caller never writes it.
    const buy = within(meta).getByRole('link', {
      name: 'Buy Modest Mouse on Bandcamp (opens in a new tab)',
    })
    expect(buy).toHaveAttribute('href', BANDCAMP_ALBUM)
    expect(buy).toHaveAttribute('target', '_blank')
    expect(buy).toHaveAttribute('rel', 'noopener noreferrer')

    expect(
      within(meta).getByRole('button', { name: 'Share Modest Mouse' })
    ).toBeInTheDocument()
  })

  it('offers no buy bracket for a Spotify card', () => {
    render(
      <ShowListenModule
        artists={[makeArtist({ socials: { spotify: SPOTIFY_ARTIST } })]}
      />
    )

    const meta = screen.getByTestId('listen-card-meta')
    expect(meta).toHaveTextContent('Spotify')
    expect(within(meta).queryByRole('link', { name: /^Buy/ })).toBeNull()
    expect(
      within(meta).getByRole('button', { name: 'Share Modest Mouse' })
    ).toBeInTheDocument()
  })

  it('offers no buy bracket for a bare Bandcamp profile', () => {
    render(
      <ShowListenModule
        artists={[
          makeArtist({ socials: { bandcamp: 'https://band.bandcamp.com' } }),
        ]}
      />
    )

    const meta = screen.getByTestId('listen-card-meta')
    expect(meta).toHaveTextContent('Bandcamp')
    expect(within(meta).queryByRole('link', { name: /^Buy/ })).toBeNull()
  })

  it('renders a slugless artist as text and drops its share bracket', () => {
    // `/artists/` resolves to the INDEX, not a 404, so an empty slug must never
    // reach an href — neither the name link nor the share URL.
    render(
      <ShowListenModule
        artists={[makeArtist({ slug: '', bandcamp_embed_url: BANDCAMP_ALBUM })]}
      />
    )

    const meta = screen.getByTestId('listen-card-meta')
    expect(within(meta).queryByRole('link', { name: 'Modest Mouse' })).toBeNull()
    expect(meta).toHaveTextContent('Modest Mouse')
    expect(within(meta).queryByRole('button', { name: /^Share/ })).toBeNull()
    // The buy bracket is independent of the slug and must survive.
    expect(
      within(meta).getByRole('link', { name: /^Buy Modest Mouse/ })
    ).toBeInTheDocument()
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
      />
    )

    const embeds = screen.getAllByTestId('music-embed')
    expect(embeds).toHaveLength(2)
    for (const embed of embeds) {
      expect(embed).toHaveAttribute('data-compact', 'true')
    }
    expect(embeds[0]).toHaveAttribute('data-bandcamp', BANDCAMP_ALBUM)
    expect(embeds[1]).toHaveAttribute('data-spotify', SPOTIFY_ARTIST)
    expect(screen.queryByRole('button', { name: /load|play|listen/i })).toBeNull()
  })
})
