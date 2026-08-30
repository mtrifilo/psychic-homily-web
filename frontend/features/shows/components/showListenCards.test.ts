import { describe, it, expect } from 'vitest'
import { listenCardsForBill } from './showListenCards'
import type { ArtistResponse } from '../types'

const SPOTIFY_ARTIST = 'https://open.spotify.com/artist/1a2b3c4d5e6f7g8h9i0jkl'

function makeArtist(overrides: Partial<ArtistResponse> = {}): ArtistResponse {
  return {
    id: 1,
    slug: 'artist-one',
    name: 'Artist One',
    set_type: 'headliner',
    position: 0,
    socials: {},
    ...overrides,
  }
}

describe('listenCardsForBill', () => {
  it('prefers a Bandcamp album/track URL and makes it the buy target', () => {
    const cards = listenCardsForBill([
      makeArtist({
        bandcamp_embed_url: 'https://band.bandcamp.com/album/record',
        socials: { spotify: SPOTIFY_ARTIST, bandcamp: 'https://band.bandcamp.com' },
      }),
    ])

    expect(cards).toHaveLength(1)
    expect(cards[0].source).toBe('Bandcamp')
    expect(cards[0].buyHref).toBe('https://band.bandcamp.com/album/record')
  })

  it('claims nothing for an embed URL that is not a Bandcamp release page', () => {
    // `bandcamp_embed_url` is contributor-writable and the write paths do not
    // host-check it, so the column can hold an arbitrary host. An ungated
    // branch would put that host behind the word "Bandcamp", behind a [Buy]
    // bracket announced as "on Bandcamp", and inside MusicEmbed's own outbound
    // fallback link. None of the three may happen.
    for (const bandcamp_embed_url of [
      'https://evil.test/checkout',
      'https://evil.test/album/x',
      'https://bandcamp.com.attacker.test/album/x',
      // A real Bandcamp host, but a page that sells nothing.
      'https://band.bandcamp.com',
      'https://band.bandcamp.com/music',
    ]) {
      expect(listenCardsForBill([makeArtist({ bandcamp_embed_url })])).toEqual(
        []
      )
    }
  })

  it('falls back to Spotify with no buy target', () => {
    const cards = listenCardsForBill([
      makeArtist({ socials: { spotify: SPOTIFY_ARTIST } }),
    ])

    expect(cards).toHaveLength(1)
    expect(cards[0].source).toBe('Spotify')
    expect(cards[0].buyHref).toBeNull()
  })

  it('drops an artist whose only link is a bare Bandcamp profile', () => {
    // A profile is not a player. MusicEmbed would render an outbound text link
    // for it, and a link wearing the same card border as the players above it
    // is a card that misrepresents itself.
    expect(
      listenCardsForBill([
        makeArtist({ socials: { bandcamp: 'https://band.bandcamp.com' } }),
      ])
    ).toEqual([])
  })

  it('prefers a playable Spotify URL over a junk Bandcamp embed value', () => {
    // The card must not be stranded on the untrusted column: an artist with a
    // real Spotify player still gets one, correctly labelled.
    const cards = listenCardsForBill([
      makeArtist({
        bandcamp_embed_url: 'https://evil.test/checkout',
        socials: { spotify: SPOTIFY_ARTIST },
      }),
    ])

    expect(cards).toHaveLength(1)
    expect(cards[0].source).toBe('Spotify')
    expect(cards[0].buyHref).toBeNull()
  })

  it('drops an artist whose only link is an unembeddable Spotify URL', () => {
    // The regression this gate exists for: the page's old predicate accepted
    // any truthy `socials.spotify`, MusicEmbed then rendered nothing, and the
    // reader got a card with a meta line and silence under it.
    expect(
      listenCardsForBill([
        makeArtist({ socials: { spotify: 'https://spotify.com/band' } }),
      ])
    ).toEqual([])
  })

  it('drops a look-alike Spotify host', () => {
    expect(
      listenCardsForBill([
        makeArtist({
          socials: {
            spotify: 'https://open.spotify.com.evil.test/artist/1a2b3c4d5e6f7g8h9i0jkl',
          },
        }),
      ])
    ).toEqual([])
  })

  it('drops an artist with no music links at all', () => {
    expect(listenCardsForBill([makeArtist()])).toEqual([])
  })

  it('hoists a curated headliner that sits below the openers', () => {
    // `set_type` is authoritative at ANY position, and the show form writes
    // position from row order, so a bill entered in stage order has its
    // headliner last. Sorting on position alone would print the running order
    // upside down relative to the bill block on the same page.
    const cards = listenCardsForBill([
      makeArtist({
        id: 1,
        name: 'Opener',
        position: 0,
        set_type: 'opener',
        socials: { bandcamp: 'https://o.bandcamp.com' },
        bandcamp_embed_url: 'https://o.bandcamp.com/album/o',
      }),
      makeArtist({
        id: 2,
        name: 'Support',
        position: 1,
        set_type: 'performer',
        bandcamp_embed_url: 'https://s.bandcamp.com/album/s',
      }),
      makeArtist({
        id: 3,
        name: 'Headliner',
        position: 2,
        set_type: 'headliner',
        bandcamp_embed_url: 'https://h.bandcamp.com/album/h',
      }),
    ])

    expect(cards.map(card => card.artist.name)).toEqual([
      'Headliner',
      'Opener',
      'Support',
    ])
  })

  it('returns cards in bill order, breaking ties on id', () => {
    const cards = listenCardsForBill([
      // All three are curated headliners, so `splitBill` hoists none of them
      // past another and what remains under test is the position sort itself.
      makeArtist({ id: 9, name: 'Support', position: 1, bandcamp_embed_url: 'https://s.bandcamp.com/album/s' }),
      makeArtist({ id: 7, name: 'Co-headliner', position: 0, bandcamp_embed_url: 'https://c.bandcamp.com/album/c' }),
      makeArtist({ id: 3, name: 'Headliner', position: 0, bandcamp_embed_url: 'https://h.bandcamp.com/album/h' }),
    ])

    expect(cards.map(card => card.artist.name)).toEqual([
      'Headliner',
      'Co-headliner',
      'Support',
    ])
  })

  it('leaves the caller array unmutated', () => {
    const artists = [
      makeArtist({ id: 2, position: 1, bandcamp_embed_url: 'https://b.bandcamp.com/album/b' }),
      makeArtist({ id: 1, position: 0, bandcamp_embed_url: 'https://a.bandcamp.com/album/a' }),
    ]
    listenCardsForBill(artists)
    expect(artists.map(artist => artist.id)).toEqual([2, 1])
  })
})
