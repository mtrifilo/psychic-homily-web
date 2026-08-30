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

  it('falls back to Spotify with no buy target', () => {
    const cards = listenCardsForBill([
      makeArtist({ socials: { spotify: SPOTIFY_ARTIST } }),
    ])

    expect(cards).toHaveLength(1)
    expect(cards[0].source).toBe('Spotify')
    expect(cards[0].buyHref).toBeNull()
  })

  it('falls back to a bare Bandcamp profile with no buy target', () => {
    // MusicEmbed renders its own visible "Listen on Bandcamp" link for this
    // case, so a [Buy] bracket here would say the same thing twice.
    const cards = listenCardsForBill([
      makeArtist({ socials: { bandcamp: 'https://band.bandcamp.com' } }),
    ])

    expect(cards).toHaveLength(1)
    expect(cards[0].source).toBe('Bandcamp')
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

  it('returns cards in bill order, breaking ties on id', () => {
    const cards = listenCardsForBill([
      makeArtist({ id: 9, name: 'Support', position: 1, socials: { bandcamp: 'https://s.bandcamp.com' } }),
      makeArtist({ id: 7, name: 'Co-headliner', position: 0, socials: { bandcamp: 'https://c.bandcamp.com' } }),
      makeArtist({ id: 3, name: 'Headliner', position: 0, socials: { bandcamp: 'https://h.bandcamp.com' } }),
    ])

    expect(cards.map(card => card.artist.name)).toEqual([
      'Headliner',
      'Co-headliner',
      'Support',
    ])
  })

  it('leaves the caller array unmutated', () => {
    const artists = [
      makeArtist({ id: 2, position: 1, socials: { bandcamp: 'https://b.bandcamp.com' } }),
      makeArtist({ id: 1, position: 0, socials: { bandcamp: 'https://a.bandcamp.com' } }),
    ]
    listenCardsForBill(artists)
    expect(artists.map(artist => artist.id)).toEqual([2, 1])
  })
})
