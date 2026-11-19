import { describe, it, expect } from 'vitest'
import { buildSceneWindowJsonLd } from './sceneWindowJsonLd'
import type { SceneWindowData } from './sceneWindow'
import type { SceneWeekShow } from './sceneWeek'

const show = (over: Partial<SceneWeekShow> = {}): SceneWeekShow => ({
  id: 1,
  title: '',
  event_date: '2026-07-27',
  starts_at: '2026-07-28T03:00:00Z',
  is_sold_out: false,
  is_cancelled: false,
  slug: 'riff-wood-crescent-ballroom',
  venue_name: 'Crescent Ballroom',
  venue_slug: 'crescent-ballroom',
  venue_address: '308 N 2nd Ave',
  venue_city: 'Phoenix',
  venue_state: 'AZ',
  venue_country: 'US',
  venue_timezone: 'America/Phoenix',
  artist_names: ['Riff Wood'],
  ...over,
})

const windowData = (shows: SceneWeekShow[]): SceneWindowData => ({
  window: 'next-4-weeks',
  slug: 'phoenix-az',
  sceneName: 'Phoenix, AZ',
  city: 'Phoenix',
  state: 'AZ',
  timezone: 'America/Phoenix',
  days: [{ date: '2026-07-27', shows }],
  rendered: shows.length,
  truncated: false,
  trackedVenues: [],
  widerWindow: null,
})

const BEFORE = new Date('2026-07-20T00:00:00Z')
const AFTER = new Date('2026-09-01T00:00:00Z')

describe('buildSceneWindowJsonLd', () => {
  it('names no organizer on any event: upcoming, past, cancelled or sold out', () => {
    const shows = [show({ id: 1 }), show({ id: 2, is_cancelled: true }), show({ id: 3, is_sold_out: true })]
    for (const now of [BEFORE, AFTER]) {
      const { events } = buildSceneWindowJsonLd(windowData(shows), now)
      expect(events).toHaveLength(3)
      for (const event of events) expect(event).not.toHaveProperty('organizer')
    }
  })
})
