import { describe, it, expect } from 'vitest'
import { hasRenderableMusic } from './musicAvailability'

// Callers render their own heading above <MusicEmbed>, so they need this answer
// BEFORE the component runs. A heading over nothing is PSY-1302's failure mode,
// and the outbound-link gate (PSY-1966) is what made a stored URL stop
// guaranteeing content.
describe('hasRenderableMusic', () => {
  it('is false when nothing can render', () => {
    expect(hasRenderableMusic({})).toBe(false)
    expect(hasRenderableMusic({ bandcampAlbumUrl: 'https://evil.test/album/x' })).toBe(false)
    expect(hasRenderableMusic({ bandcampProfileUrl: 'https://evil.test/band' })).toBe(false)
    // A Spotify URL that does not parse to an embeddable id.
    expect(hasRenderableMusic({ spotifyUrl: 'https://open.spotify.com/playlist/abc' })).toBe(false)
  })

  it('is true for anything MusicEmbed can turn into a player or a link', () => {
    expect(hasRenderableMusic({ bandcampAlbumUrl: 'https://band.bandcamp.com/album/x' })).toBe(true)
    expect(hasRenderableMusic({ bandcampProfileUrl: 'https://band.bandcamp.com' })).toBe(true)
    expect(
      hasRenderableMusic({ spotifyUrl: 'https://open.spotify.com/artist/4Z8W4fKeB5YxbusRsdQVPb' })
    ).toBe(true)
  })

  // NECESSARY, not sufficient: an on-platform non-release page still reaches the
  // resolver, which can return a playable id, so this must not pre-emptively
  // hide the section. Argued on the function.
  it('accepts an on-platform page that is not a release', () => {
    expect(hasRenderableMusic({ bandcampAlbumUrl: 'https://band.bandcamp.com/music' })).toBe(true)
  })

  // http reaches neither renderer, so a heading over it would strand empty.
  it('is false for an http Bandcamp URL', () => {
    expect(hasRenderableMusic({ bandcampAlbumUrl: 'http://band.bandcamp.com/album/x' })).toBe(false)
  })
})
