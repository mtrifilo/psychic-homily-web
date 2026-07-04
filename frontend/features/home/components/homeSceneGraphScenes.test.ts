import { describe, it, expect } from 'vitest'
import { pickDefaultScene, pickSurpriseScene } from './homeSceneGraphScenes'
import type { SceneListItem } from '@/features/scenes/types'

function scene(overrides: Partial<SceneListItem> & { slug: string }): SceneListItem {
  return {
    city: 'Phoenix',
    state: 'AZ',
    venue_count: 3,
    upcoming_show_count: 0,
    total_show_count: 10,
    shows_this_week: 0,
    ...overrides,
  }
}

describe('pickDefaultScene', () => {
  it('returns null for an empty list', () => {
    expect(pickDefaultScene([])).toBeNull()
  })

  it('picks the liveliest scene by upcoming_show_count without mutating the input', () => {
    const scenes = [
      scene({ slug: 'phoenix-az', upcoming_show_count: 4 }),
      scene({ slug: 'chicago-il', city: 'Chicago', state: 'IL', upcoming_show_count: 17 }),
      scene({ slug: 'minneapolis-mn', city: 'Minneapolis', state: 'MN', upcoming_show_count: 9 }),
    ]
    const original = scenes.map(s => s.slug)
    expect(pickDefaultScene(scenes)?.slug).toBe('chicago-il')
    expect(scenes.map(s => s.slug)).toEqual(original)
  })

  it('tolerates non-finite counts (they sort last)', () => {
    const scenes = [
      scene({ slug: 'bad', upcoming_show_count: NaN }),
      scene({ slug: 'good', upcoming_show_count: 1 }),
    ]
    expect(pickDefaultScene(scenes)?.slug).toBe('good')
  })

  describe('geo personalization (PSY-1346)', () => {
    const scenes = [
      scene({
        slug: 'chicago-il',
        city: 'Chicago',
        state: 'IL',
        upcoming_show_count: 17,
        latitude: 41.88,
        longitude: -87.63,
      }),
      scene({
        slug: 'phoenix-az',
        city: 'Phoenix',
        state: 'AZ',
        upcoming_show_count: 4,
        latitude: 33.45,
        longitude: -112.07,
      }),
    ]

    it('falls back to the liveliest scene when geo is absent', () => {
      expect(pickDefaultScene(scenes, null)?.slug).toBe('chicago-il')
      expect(pickDefaultScene(scenes, undefined)?.slug).toBe('chicago-il')
    })

    it('picks the visitor’s scene on an exact city/state match (over the liveliest)', () => {
      expect(
        pickDefaultScene(scenes, { city: 'Phoenix', state: 'AZ' })?.slug,
      ).toBe('phoenix-az')
    })

    it('matches case- and whitespace-insensitively (Vercel spelling vs stored casing)', () => {
      expect(
        pickDefaultScene(scenes, { city: '  phoenix ', state: 'az' })?.slug,
      ).toBe('phoenix-az')
    })

    it('picks the nearest scene by haversine when the exact city has no scene', () => {
      // Tucson, AZ has no scene here; its coords are far closer to Phoenix
      // than to Chicago.
      expect(
        pickDefaultScene(scenes, {
          city: 'Tucson',
          state: 'AZ',
          latitude: 32.22,
          longitude: -110.97,
        })?.slug,
      ).toBe('phoenix-az')
    })

    it('falls back to the liveliest scene when the exact city misses and no coords are supplied', () => {
      expect(
        pickDefaultScene(scenes, { city: 'Nowhere', state: 'XX' })?.slug,
      ).toBe('chicago-il')
    })

    it('ignores scenes the geocoder could not place when computing the nearest', () => {
      const withUnplaced = [
        scene({
          slug: 'chicago-il',
          city: 'Chicago',
          state: 'IL',
          upcoming_show_count: 17,
          latitude: null,
          longitude: null,
        }),
        scene({
          slug: 'phoenix-az',
          city: 'Phoenix',
          state: 'AZ',
          upcoming_show_count: 4,
          latitude: 33.45,
          longitude: -112.07,
        }),
      ]
      // Visitor near Chicago, but Chicago has no centroid → nearest placeable
      // is Phoenix (rather than "no default").
      expect(
        pickDefaultScene(withUnplaced, {
          city: 'Evanston',
          state: 'IL',
          latitude: 42.05,
          longitude: -87.69,
        })?.slug,
      ).toBe('phoenix-az')
    })
  })
})

describe('pickSurpriseScene', () => {
  const scenes = [
    scene({ slug: 'phoenix-az', upcoming_show_count: 4 }),
    scene({ slug: 'chicago-il', upcoming_show_count: 17 }),
    scene({ slug: 'ghost-town', upcoming_show_count: 0 }),
  ]

  it('never returns the current scene', () => {
    for (let i = 0; i < 10; i++) {
      expect(pickSurpriseScene(scenes, 'chicago-il')?.slug).not.toBe('chicago-il')
    }
  })

  it('prefers scenes with upcoming shows', () => {
    // From phoenix, the active pool is just chicago — ghost-town is excluded.
    expect(pickSurpriseScene(scenes, 'phoenix-az', () => 0.99)?.slug).toBe('chicago-il')
  })

  it('falls back to inactive scenes when no other scene has shows', () => {
    const sparse = [
      scene({ slug: 'phoenix-az', upcoming_show_count: 4 }),
      scene({ slug: 'ghost-town', upcoming_show_count: 0 }),
    ]
    expect(pickSurpriseScene(sparse, 'phoenix-az')?.slug).toBe('ghost-town')
  })

  it('returns null when there is nothing to rotate to', () => {
    expect(pickSurpriseScene([], null)).toBeNull()
    expect(pickSurpriseScene([scene({ slug: 'only-one' })], 'only-one')).toBeNull()
  })

  it('clamps an inclusive random() === 1 into range', () => {
    expect(pickSurpriseScene(scenes, 'ghost-town', () => 1)?.slug).toBeDefined()
  })
})
