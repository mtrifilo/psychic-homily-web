import { describe, it, expect } from 'vitest'
import { pickViewerScene } from './viewerScene'
import type { SceneListItem } from '@/features/scenes/types'

const scene = (over: Partial<SceneListItem> = {}): SceneListItem => ({
  city: 'Phoenix',
  state: 'AZ',
  slug: 'phoenix-az',
  venue_count: 10,
  upcoming_show_count: 12,
  total_show_count: 400,
  shows_this_week: 4,
  latitude: 33.45,
  longitude: -112.07,
  ...over,
})

describe('pickViewerScene', () => {
  it('returns the scene whose city and state the visitor is in', () => {
    const scenes = [
      scene({ city: 'Chicago', state: 'IL', slug: 'chicago-il', latitude: 41.88, longitude: -87.63 }),
      scene(),
    ]
    expect(pickViewerScene(scenes, { city: 'Phoenix', state: 'AZ' })?.slug).toBe('phoenix-az')
  })

  // A suburb is not its own scene, so an exact match is not available; the
  // shared nearest-by-centroid tier is what places the visitor anyway.
  it('falls back to the nearest scene when the visitor city is not one', () => {
    const scenes = [
      scene({ city: 'Chicago', state: 'IL', slug: 'chicago-il', latitude: 41.88, longitude: -87.63 }),
      scene(),
    ]
    const nearPhoenix = { city: 'Tempe', state: 'AZ', latitude: 33.42, longitude: -111.94 }
    expect(pickViewerScene(scenes, nearPhoenix)?.slug).toBe('phoenix-az')
  })

  // The whole point of the guard: a visitor we cannot place must keep the
  // global listing rather than be sent to whichever scene happens to exist.
  it('returns null with no geo suggestion', () => {
    expect(pickViewerScene([scene()], null)).toBeNull()
    expect(pickViewerScene([scene()], undefined)).toBeNull()
  })

  it('returns null before the scenes list has loaded', () => {
    expect(pickViewerScene([], { city: 'Phoenix', state: 'AZ' })).toBeNull()
  })

  // Unlike the homepage graph's default, there is no liveliest-scene fallback:
  // the caller is a navigation target, and another region's night is a worse
  // answer than the listing the visitor already had.
  it('returns null when the visitor cannot be placed against any scene', () => {
    const scenes = [scene({ latitude: null, longitude: null })]
    expect(pickViewerScene(scenes, { city: 'Berlin', state: 'BE' })).toBeNull()
  })

  // A correct-but-empty nightly page is a worse destination than the listing.
  it('returns null when the matched scene has nothing upcoming', () => {
    const scenes = [scene({ upcoming_show_count: 0 })]
    expect(pickViewerScene(scenes, { city: 'Phoenix', state: 'AZ' })).toBeNull()
  })
})
