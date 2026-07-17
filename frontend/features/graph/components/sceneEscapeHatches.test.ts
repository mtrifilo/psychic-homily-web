import { describe, expect, it } from 'vitest'

import type { SceneListItem } from '@/features/scenes/types'
import { pickSceneEscapeHatches } from './sceneEscapeHatches'

const SCENE = (
  city: string,
  state: string,
  upcoming: number,
): SceneListItem => ({
  city,
  state,
  slug: `${city.toLowerCase().replace(/\s+/g, '-')}-${state.toLowerCase()}`,
  venue_count: 3,
  upcoming_show_count: upcoming,
  total_show_count: upcoming * 10,
  shows_this_week: 1,
})

const SCENES = [
  SCENE('Phoenix', 'AZ', 12),
  SCENE('Tucson', 'AZ', 4),
  SCENE('Los Angeles', 'CA', 30),
  SCENE('Portland', 'OR', 8),
]

describe('pickSceneEscapeHatches (PSY-1474 F4)', () => {
  it('puts the artist home-metro scene first, then the liveliest same-state scene', () => {
    const picks = pickSceneEscapeHatches(SCENES, 'Tucson', 'AZ')
    expect(picks.map(s => s.city)).toEqual(['Tucson', 'Phoenix'])
  })

  it('matches city case-insensitively', () => {
    const picks = pickSceneEscapeHatches(SCENES, 'phoenix', 'az')
    expect(picks[0].city).toBe('Phoenix')
  })

  it('falls back to the liveliest scenes when the artist has no location', () => {
    const picks = pickSceneEscapeHatches(SCENES, undefined, undefined)
    expect(picks.map(s => s.city)).toEqual(['Los Angeles', 'Phoenix'])
  })

  it('falls back past the state when it has no other scenes', () => {
    const picks = pickSceneEscapeHatches(SCENES, 'Portland', 'OR')
    expect(picks.map(s => s.city)).toEqual(['Portland', 'Los Angeles'])
  })

  it('does not treat a same-named city in another state as home', () => {
    const scenes = [...SCENES, SCENE('Portland', 'ME', 2)]
    const picks = pickSceneEscapeHatches(scenes, 'Portland', 'ME')
    expect(picks.map(s => s.state)).toEqual(['ME', 'CA'])
  })

  it('returns at most two scenes and tolerates an empty list', () => {
    expect(pickSceneEscapeHatches(SCENES, 'Phoenix', 'AZ')).toHaveLength(2)
    expect(pickSceneEscapeHatches([], 'Phoenix', 'AZ')).toEqual([])
  })
})
