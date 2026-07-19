import { describe, expect, it } from 'vitest'
import { suggestAlternativeScenes } from './suggestScenes'
import type { ChartScene } from './types'

const scenes: ChartScene[] = [
  {
    metro: '38060',
    name: 'Phoenix-Mesa-Chandler, AZ',
    city: 'Phoenix',
    state: 'AZ',
    show_count: 42,
    artist_count: 41,
    venue_count: 12,
  },
  {
    metro: '46060',
    name: 'Tucson, AZ',
    city: 'Tucson',
    state: 'AZ',
    show_count: 17,
    artist_count: 19,
    venue_count: 8,
  },
  {
    metro: '31080',
    name: 'Los Angeles-Long Beach-Anaheim, CA',
    city: 'Los Angeles',
    state: 'CA',
    show_count: 11,
    artist_count: 30,
    venue_count: 20,
  },
]

describe('suggestAlternativeScenes', () => {
  it('excludes the current metro and keeps busiest-first order', () => {
    expect(suggestAlternativeScenes(scenes, '38060', 2)).toEqual([
      scenes[1],
      scenes[2],
    ])
  })

  it('returns an empty list when there is no current metro', () => {
    expect(suggestAlternativeScenes(scenes, '')).toEqual([])
  })

  it('returns an empty list when the current metro is the only scene', () => {
    expect(suggestAlternativeScenes([scenes[0]], '38060')).toEqual([])
  })

  it('honours the limit', () => {
    expect(suggestAlternativeScenes(scenes, '38060', 1)).toEqual([scenes[1]])
  })
})
