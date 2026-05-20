import { describe, it, expect } from 'vitest'
import { radioEndpoints, radioQueryKeys } from './api'

// API_BASE_URL resolves to 'http://localhost:8080' under vitest
// (NEXT_PUBLIC_API_URL set in vitest.config.mts), so endpoint strings
// below are asserted against that fully-resolved base.
const BASE = 'http://localhost:8080'

describe('radioEndpoints', () => {
  it('STATIONS is the collection URL', () => {
    expect(radioEndpoints.STATIONS).toBe(`${BASE}/radio-stations`)
  })

  it('STATION(slug) appends the slug', () => {
    expect(radioEndpoints.STATION('kexp')).toBe(`${BASE}/radio-stations/kexp`)
  })

  it('SHOWS is the collection URL', () => {
    expect(radioEndpoints.SHOWS).toBe(`${BASE}/radio-shows`)
  })

  it('SHOW(slug) appends the slug', () => {
    expect(radioEndpoints.SHOW('wfmu-drummer')).toBe(`${BASE}/radio-shows/wfmu-drummer`)
  })

  it('SHOW_EPISODES(slug) nests under the show', () => {
    expect(radioEndpoints.SHOW_EPISODES('wfmu-drummer')).toBe(
      `${BASE}/radio-shows/wfmu-drummer/episodes`
    )
  })

  it('SHOW_EPISODE_BY_DATE(slug, date) nests the date segment', () => {
    expect(radioEndpoints.SHOW_EPISODE_BY_DATE('wfmu-drummer', '2026-05-01')).toBe(
      `${BASE}/radio-shows/wfmu-drummer/episodes/2026-05-01`
    )
  })

  it('SHOW_TOP_ARTISTS(slug) nests under the show', () => {
    expect(radioEndpoints.SHOW_TOP_ARTISTS('wfmu-drummer')).toBe(
      `${BASE}/radio-shows/wfmu-drummer/top-artists`
    )
  })

  it('SHOW_TOP_LABELS(slug) nests under the show', () => {
    expect(radioEndpoints.SHOW_TOP_LABELS('wfmu-drummer')).toBe(
      `${BASE}/radio-shows/wfmu-drummer/top-labels`
    )
  })

  it('ARTIST_RADIO_PLAYS(slug) nests under the artist', () => {
    expect(radioEndpoints.ARTIST_RADIO_PLAYS('gatecreeper')).toBe(
      `${BASE}/artists/gatecreeper/radio-plays`
    )
  })

  it('RELEASE_RADIO_PLAYS(slug) nests under the release', () => {
    expect(radioEndpoints.RELEASE_RADIO_PLAYS('an-unkindness')).toBe(
      `${BASE}/releases/an-unkindness/radio-plays`
    )
  })

  it('NEW_RELEASES is the aggregation URL', () => {
    expect(radioEndpoints.NEW_RELEASES).toBe(`${BASE}/radio/new-releases`)
  })

  it('STATS is the aggregation URL', () => {
    expect(radioEndpoints.STATS).toBe(`${BASE}/radio/stats`)
  })
})

describe('radioQueryKeys', () => {
  it('stations() is a stable single-segment key', () => {
    expect(radioQueryKeys.stations()).toEqual(['radio-stations'])
  })

  it('station(slug) scopes by slug', () => {
    expect(radioQueryKeys.station('kexp')).toEqual(['radio-stations', 'kexp'])
  })

  it('shows(stationId) wraps the id in a params object', () => {
    expect(radioQueryKeys.shows(7)).toEqual(['radio-shows', { stationId: 7 }])
  })

  it('shows() with no id still carries an undefined stationId for cache stability', () => {
    expect(radioQueryKeys.shows()).toEqual(['radio-shows', { stationId: undefined }])
  })

  it('show(slug) scopes by slug', () => {
    expect(radioQueryKeys.show('drummer')).toEqual(['radio-shows', 'drummer'])
  })

  it('episodes(slug, params) carries the params object', () => {
    expect(radioQueryKeys.episodes('drummer', { limit: 20, offset: 0 })).toEqual([
      'radio-shows',
      'drummer',
      'episodes',
      { limit: 20, offset: 0 },
    ])
  })

  it('episode(slug, date) scopes by show + date', () => {
    expect(radioQueryKeys.episode('drummer', '2026-05-01')).toEqual([
      'radio-shows',
      'drummer',
      'episodes',
      '2026-05-01',
    ])
  })

  it('topArtists(slug, params) carries the params object', () => {
    expect(radioQueryKeys.topArtists('drummer', { period: 90, limit: 20 })).toEqual([
      'radio-shows',
      'drummer',
      'top-artists',
      { period: 90, limit: 20 },
    ])
  })

  it('topLabels(slug, params) carries the params object', () => {
    expect(radioQueryKeys.topLabels('drummer', { period: 90, limit: 20 })).toEqual([
      'radio-shows',
      'drummer',
      'top-labels',
      { period: 90, limit: 20 },
    ])
  })

  it('artistPlays(slug) scopes under the artist namespace', () => {
    expect(radioQueryKeys.artistPlays('gatecreeper')).toEqual([
      'artists',
      'gatecreeper',
      'radio-plays',
    ])
  })

  it('releasePlays(slug) scopes under the release namespace', () => {
    expect(radioQueryKeys.releasePlays('an-unkindness')).toEqual([
      'releases',
      'an-unkindness',
      'radio-plays',
    ])
  })

  it('newReleases(params) carries the params object', () => {
    expect(radioQueryKeys.newReleases({ stationId: 7, limit: 20 })).toEqual([
      'radio',
      'new-releases',
      { stationId: 7, limit: 20 },
    ])
  })

  it('stats() is a stable two-segment key', () => {
    expect(radioQueryKeys.stats()).toEqual(['radio', 'stats'])
  })
})
