import { describe, expect, it } from 'vitest'
import {
  creditedReleaseArtists,
  knownReleaseYear,
  releaseLabelName,
  releaseListenServiceNames,
  releaseTypeLabel,
} from './releaseSearchFacts'

const artist = (name: string, role: string) => ({ id: 1, slug: name, name, role })

describe('creditedReleaseArtists', () => {
  it('keeps only main artists, in payload order', () => {
    const credited = creditedReleaseArtists([
      artist('Producer', 'producer'),
      artist('First Main', 'main'),
      artist('Guest', 'featured'),
      artist('Second Main', 'main'),
    ])
    expect(credited.map(a => a.name)).toEqual(['First Main', 'Second Main'])
  })

  it('falls back to the first credited artist when none is main', () => {
    const credited = creditedReleaseArtists([
      artist('Guest', 'featured'),
      artist('Producer', 'producer'),
    ])
    expect(credited.map(a => a.name)).toEqual(['Guest'])
  })

  it('skips artists with a blank name', () => {
    expect(
      creditedReleaseArtists([artist(' ', 'main'), artist('Named', 'featured')]).map(
        a => a.name
      )
    ).toEqual(['Named'])
  })

  it('is empty for no artists, including a null list', () => {
    expect(creditedReleaseArtists([])).toEqual([])
    expect(creditedReleaseArtists(null)).toEqual([])
    expect(creditedReleaseArtists(undefined)).toEqual([])
  })
})

describe('releaseListenServiceNames', () => {
  const link = (platform: string, url: string) => ({ id: 1, platform, url })

  it('names each service by the label the page prints, in page order', () => {
    expect(
      releaseListenServiceNames([
        link('spotify', 'https://open.spotify.com/album/1'),
        link('apple_music', 'https://music.apple.com/us/album/1'),
        link('bandcamp', 'https://band.bandcamp.com/album/x'),
      ])
    ).toEqual(['Spotify', 'Apple Music', 'Bandcamp'])
  })

  it('leaves out Discogs, which is a reference database, not a place to listen', () => {
    expect(
      releaseListenServiceNames([
        link('discogs', 'https://www.discogs.com/master/1'),
        link('bandcamp', 'https://band.bandcamp.com/album/x'),
      ])
    ).toEqual(['Bandcamp'])
    expect(
      releaseListenServiceNames([link('Discogs', 'https://www.discogs.com/master/1')])
    ).toEqual([])
  })

  it('names a service once however many links it has', () => {
    expect(
      releaseListenServiceNames([
        link('bandcamp', 'https://band.bandcamp.com/album/x'),
        link('spotify', 'https://open.spotify.com/album/1'),
        link('bandcamp', 'https://band.bandcamp.com/album/x-deluxe'),
      ])
    ).toEqual(['Bandcamp', 'Spotify'])
  })

  it('leaves out links the page does not render', () => {
    expect(
      releaseListenServiceNames([
        link('spotify', 'https://evil.example.com/album/1'),
        link('myspace', 'https://myspace.com/band'),
      ])
    ).toEqual([])
  })

  it('is empty for no links, including a null list', () => {
    expect(releaseListenServiceNames([])).toEqual([])
    expect(releaseListenServiceNames(null)).toEqual([])
  })
})

describe('releaseTypeLabel', () => {
  it('is the label the page shows', () => {
    expect(releaseTypeLabel('lp')).toBe('LP')
    expect(releaseTypeLabel('ep')).toBe('EP')
    expect(releaseTypeLabel('single')).toBe('Single')
    expect(releaseTypeLabel('compilation')).toBe('Compilation')
    expect(releaseTypeLabel('boxset')).toBe('BOXSET')
  })

  it('is null for an empty type', () => {
    expect(releaseTypeLabel('')).toBeNull()
    expect(releaseTypeLabel('  ')).toBeNull()
    expect(releaseTypeLabel(null)).toBeNull()
  })
})

describe('releaseLabelName', () => {
  it('is the first label with a name, in page order', () => {
    expect(releaseLabelName([{ name: 'Nettwerk' }, { name: 'Second' }])).toBe('Nettwerk')
    expect(releaseLabelName([{ name: ' ' }, { name: 'Named' }])).toBe('Named')
  })

  it('is null for no labels, including a null list', () => {
    expect(releaseLabelName([])).toBeNull()
    expect(releaseLabelName(null)).toBeNull()
  })
})

describe('knownReleaseYear', () => {
  it('passes a positive integer year through', () => {
    expect(knownReleaseYear(2019)).toBe(2019)
  })

  it('is null for a missing, zero, negative or fractional year', () => {
    expect(knownReleaseYear(null)).toBeNull()
    expect(knownReleaseYear(undefined)).toBeNull()
    expect(knownReleaseYear(0)).toBeNull()
    expect(knownReleaseYear(-1)).toBeNull()
    expect(knownReleaseYear(2019.5)).toBeNull()
  })
})
