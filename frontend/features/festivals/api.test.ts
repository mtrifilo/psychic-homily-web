import { describe, it, expect } from 'vitest'
import { API_BASE_URL } from '@/lib/api-base'
import { festivalEndpoints, festivalQueryKeys } from './api'

describe('festivalEndpoints', () => {
  it('exposes static collection endpoints', () => {
    expect(festivalEndpoints.LIST).toBe(`${API_BASE_URL}/festivals`)
    expect(festivalEndpoints.SEARCH).toBe(`${API_BASE_URL}/festivals/search`)
    expect(festivalEndpoints.CREATE).toBe(`${API_BASE_URL}/festivals`)
  })

  it('builds detail + mutation endpoints from an id or slug', () => {
    expect(festivalEndpoints.GET('form-arcosanti')).toBe(
      `${API_BASE_URL}/festivals/form-arcosanti`
    )
    expect(festivalEndpoints.GET(42)).toBe(`${API_BASE_URL}/festivals/42`)
    expect(festivalEndpoints.UPDATE(42)).toBe(`${API_BASE_URL}/festivals/42`)
    expect(festivalEndpoints.DELETE(42)).toBe(`${API_BASE_URL}/festivals/42`)
  })

  it('builds lineup (artist) endpoints', () => {
    expect(festivalEndpoints.ARTISTS(1)).toBe(`${API_BASE_URL}/festivals/1/artists`)
    expect(festivalEndpoints.ADD_ARTIST(1)).toBe(`${API_BASE_URL}/festivals/1/artists`)
    expect(festivalEndpoints.UPDATE_ARTIST(1, 7)).toBe(
      `${API_BASE_URL}/festivals/1/artists/7`
    )
    expect(festivalEndpoints.REMOVE_ARTIST(1, 7)).toBe(
      `${API_BASE_URL}/festivals/1/artists/7`
    )
  })

  it('builds venue endpoints', () => {
    expect(festivalEndpoints.VENUES(1)).toBe(`${API_BASE_URL}/festivals/1/venues`)
    expect(festivalEndpoints.ADD_VENUE(1)).toBe(`${API_BASE_URL}/festivals/1/venues`)
    expect(festivalEndpoints.REMOVE_VENUE(1, 9)).toBe(
      `${API_BASE_URL}/festivals/1/venues/9`
    )
  })

  it('builds artist-scoped festival endpoints', () => {
    expect(festivalEndpoints.ARTIST_FESTIVALS('radiohead')).toBe(
      `${API_BASE_URL}/artists/radiohead/festivals`
    )
    expect(festivalEndpoints.ARTIST_TRAJECTORY('radiohead')).toBe(
      `${API_BASE_URL}/artists/radiohead/festival-trajectory`
    )
  })

  it('builds festival-intelligence endpoints', () => {
    expect(festivalEndpoints.SIMILAR(1)).toBe(`${API_BASE_URL}/festivals/1/similar`)
    expect(festivalEndpoints.OVERLAP(1, 2)).toBe(
      `${API_BASE_URL}/festivals/1/overlap/2`
    )
    expect(festivalEndpoints.BREAKOUTS(1)).toBe(
      `${API_BASE_URL}/festivals/1/breakouts`
    )
    expect(festivalEndpoints.SERIES_COMPARE('coachella')).toBe(
      `${API_BASE_URL}/festivals/series/coachella/compare`
    )
  })
})

describe('festivalQueryKeys', () => {
  it('exposes the root key used for invalidation', () => {
    expect(festivalQueryKeys.all).toEqual(['festivals'])
  })

  it('namespaces list keys under the filter object', () => {
    expect(festivalQueryKeys.list()).toEqual(['festivals', 'list', undefined])
    expect(festivalQueryKeys.list({ year: 2025 })).toEqual([
      'festivals',
      'list',
      { year: 2025 },
    ])
  })

  it('lower-cases the search key for case-insensitive caching', () => {
    expect(festivalQueryKeys.search('Coachella')).toEqual([
      'festivals',
      'search',
      'coachella',
    ])
  })

  it('stringifies ids in detail/artist/venue keys for stable equality', () => {
    expect(festivalQueryKeys.detail(42)).toEqual(['festivals', 'detail', '42'])
    expect(festivalQueryKeys.detail('form')).toEqual([
      'festivals',
      'detail',
      'form',
    ])
    expect(festivalQueryKeys.venues(3)).toEqual(['festivals', 'venues', '3'])
    expect(festivalQueryKeys.artistFestivals(5)).toEqual([
      'festivals',
      'artist',
      '5',
    ])
  })

  it('includes the optional dayDate segment in the artists key', () => {
    expect(festivalQueryKeys.artists(1)).toEqual([
      'festivals',
      'artists',
      '1',
      undefined,
    ])
    expect(festivalQueryKeys.artists(1, '2025-05-09')).toEqual([
      'festivals',
      'artists',
      '1',
      '2025-05-09',
    ])
  })

  it('builds intelligence keys', () => {
    expect(festivalQueryKeys.similar(1)).toEqual(['festivals', 'similar', '1'])
    expect(festivalQueryKeys.overlap(1, 2)).toEqual([
      'festivals',
      'overlap',
      '1',
      '2',
    ])
    expect(festivalQueryKeys.breakouts(1)).toEqual([
      'festivals',
      'breakouts',
      '1',
    ])
    expect(festivalQueryKeys.artistTrajectory(7)).toEqual([
      'festivals',
      'trajectory',
      '7',
    ])
  })

  it('joins the years array into the seriesCompare key', () => {
    expect(festivalQueryKeys.seriesCompare('coachella', [2024, 2025])).toEqual([
      'festivals',
      'series',
      'coachella',
      '2024,2025',
    ])
  })
})
