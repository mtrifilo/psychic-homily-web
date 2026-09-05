import { describe, it, expect } from 'vitest'
import { playableMusicSources } from './playableMusicSources'

const SPOTIFY_ARTIST = 'https://open.spotify.com/artist/1a2b3c4d5e6f7g8h9i0jkl'
const RELEASE = 'https://band.bandcamp.com/album/record'
const ON_PLATFORM_NON_RELEASE = 'https://band.bandcamp.com/music'

describe('playableMusicSources', () => {
  it('offers Bandcamp before Spotify when both are proven', () => {
    expect(
      playableMusicSources({
        bandcampUrl: RELEASE,
        spotifyUrl: SPOTIFY_ARTIST,
        bandcampScope: 'release',
      })
    ).toEqual([
      { service: 'bandcamp', url: RELEASE },
      {
        service: 'spotify',
        url: SPOTIFY_ARTIST,
        kind: 'artist',
        id: '1a2b3c4d5e6f7g8h9i0jkl',
      },
    ])
  })

  it("admits an on-platform non-release page only at the 'platform' scope", () => {
    // The scope is the whole difference between the two callers. A surface that
    // will hand the URL to the embed resolver takes it, because only the page
    // itself can say whether it carries a player; a surface that will name it
    // or point a Buy at it does not, because neither claim is true of a page
    // that sells nothing.
    expect(
      playableMusicSources({
        bandcampUrl: ON_PLATFORM_NON_RELEASE,
        bandcampScope: 'platform',
      })
    ).toEqual([{ service: 'bandcamp', url: ON_PLATFORM_NON_RELEASE }])

    expect(
      playableMusicSources({
        bandcampUrl: ON_PLATFORM_NON_RELEASE,
        bandcampScope: 'release',
      })
    ).toEqual([])
  })

  it('drops a Bandcamp value neither scope can prove, at both scopes', () => {
    for (const bandcampUrl of [
      'https://evil.test/album/x',
      'https://bandcamp.com.attacker.test/album/x',
      'http://band.bandcamp.com/album/record',
      'not a url',
    ]) {
      for (const bandcampScope of ['release', 'platform'] as const) {
        expect(
          playableMusicSources({ bandcampUrl, bandcampScope })
        ).toEqual([])
      }
    }
  })

  it('drops a Spotify URL that host-anchored parsing refuses', () => {
    for (const spotifyUrl of [
      'https://open.spotify.com.evil.test/artist/1a2b3c4d5e6f7g8h9i0jkl',
      'https://open.spotify.com/artist/too-short',
      'https://open.spotify.com/playlist/1a2b3c4d5e6f7g8h9i0jkl',
    ]) {
      expect(
        playableMusicSources({ spotifyUrl, bandcampScope: 'platform' })
      ).toEqual([])
    }
  })

  it('yields nothing when an artist has neither URL', () => {
    expect(
      playableMusicSources({
        bandcampUrl: null,
        spotifyUrl: undefined,
        bandcampScope: 'platform',
      })
    ).toEqual([])
  })
})
