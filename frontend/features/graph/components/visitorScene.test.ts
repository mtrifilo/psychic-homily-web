import { describe, it, expect } from 'vitest'
import { pickVisitorScene } from './visitorScene'
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

const chicago = scene({
  city: 'Chicago',
  state: 'IL',
  slug: 'chicago-il',
  latitude: 41.88,
  longitude: -87.63,
})

describe('pickVisitorScene', () => {
  it('returns the scene whose city and state the visitor is in', () => {
    expect(
      pickVisitorScene([chicago, scene()], { city: 'Phoenix', state: 'AZ' })?.slug
    ).toBe('phoenix-az')
  })

  it('matches across casing and stray whitespace on either side', () => {
    const scenes = [scene({ city: ' Phoenix ', state: 'az' })]
    expect(pickVisitorScene(scenes, { city: 'PHOENIX', state: 'Az' })?.slug).toBe(
      'phoenix-az'
    )
  })

  // The whole point of the guard: a visitor we cannot name a scene for keeps
  // the global listing rather than being sent to whichever scene exists.
  it('returns null with no geo suggestion', () => {
    expect(pickVisitorScene([scene()], null)).toBeNull()
    expect(pickVisitorScene([scene()], undefined)).toBeNull()
  })

  it('returns null before the scenes list has loaded', () => {
    expect(pickVisitorScene([], { city: 'Phoenix', state: 'AZ' })).toBeNull()
  })

  // The deliberate divergence from the city filter and the homepage graph,
  // which would both place this visitor on Phoenix by distance. This link
  // names no city, so a silent neighbouring-metro answer is worse than none.
  it('does NOT fall back to the nearest scene for a suburb', () => {
    const nearPhoenix = { city: 'Tempe', state: 'AZ', latitude: 33.42, longitude: -111.94 }
    expect(pickVisitorScene([scene()], nearPhoenix)).toBeNull()
  })

  it('does NOT fall back to the nearest scene for a far-away visitor', () => {
    const inHonolulu = { city: 'Honolulu', state: 'HI', latitude: 21.31, longitude: -157.86 }
    expect(pickVisitorScene([scene()], inHonolulu)).toBeNull()
  })

  // A scene dark all week has a nightly page that is correct and empty, which
  // is a worse destination than the listing.
  it('returns null when the matched scene has been quiet all week', () => {
    expect(
      pickVisitorScene([scene({ shows_this_week: 0 })], { city: 'Phoenix', state: 'AZ' })
    ).toBeNull()
  })

  // The gate is near-term activity, not the scene's whole upcoming calendar:
  // a scene with a full month ahead and nothing this week is not somewhere to
  // send a reader asking what is on tonight.
  it('reads this week, not the whole upcoming calendar', () => {
    const scenes = [scene({ upcoming_show_count: 40, shows_this_week: 0 })]
    expect(pickVisitorScene(scenes, { city: 'Phoenix', state: 'AZ' })).toBeNull()
  })

  // A city name alone is not an identity: Portland OR and Portland ME are
  // different scenes, so a half-answer must not resolve to either.
  it('returns null when the suggestion is missing half of its place', () => {
    expect(pickVisitorScene([scene()], { city: 'Phoenix', state: '' })).toBeNull()
    expect(pickVisitorScene([scene()], { city: '', state: 'AZ' })).toBeNull()
  })
})
