import { describe, it, expect } from 'vitest'
import { buildSceneWeekJsonLd } from './sceneWeekJsonLd'
import type { SceneWeekResponse, SceneWeekShow } from './sceneWeek'

const show = (over: Partial<SceneWeekShow> = {}): SceneWeekShow => ({
  id: 1,
  title: '',
  event_date: '2026-07-27',
  // 20:00 Phoenix time on the 27th — the instant a naive date-only parse gets
  // wrong, since UTC midnight on the 27th is the 26th in Arizona.
  starts_at: '2026-07-28T03:00:00Z',
  is_sold_out: false,
  is_cancelled: false,
  slug: 'riff-wood-crescent-ballroom',
  venue_name: 'Crescent Ballroom',
  venue_slug: 'crescent-ballroom',
  venue_address: '308 N 2nd Ave',
  venue_city: 'Phoenix',
  venue_state: 'AZ',
  venue_timezone: 'America/Phoenix',
  artist_names: ['Riff Wood'],
  ...over,
})

const week = (shows: SceneWeekShow[]): SceneWeekResponse => ({
  slug: 'phoenix-az',
  scene_name: 'Phoenix, AZ',
  city: 'Phoenix',
  state: 'AZ',
  iso_week: '2026-W31',
  start_date: '2026-07-27',
  end_date: '2026-08-02',
  timezone: 'America/Phoenix',
  show_count: shows.length,
  prev_week: '2026-W30',
  next_week: '2026-W32',
  is_current_week: true,
  tracked_venues: ['Crescent Ballroom'],
  days: [
    { date: '2026-07-27', shows },
    { date: '2026-07-28', shows: [] },
  ],
})

describe('buildSceneWeekJsonLd — MusicEvent', () => {
  it('emits one event per listed show, across days', () => {
    const data = week([])
    data.days = [
      { date: '2026-07-27', shows: [show({ id: 1 })] },
      { date: '2026-07-28', shows: [] },
      { date: '2026-07-29', shows: [show({ id: 2 }), show({ id: 3 })] },
    ]
    const { events } = buildSceneWeekJsonLd(data)
    expect(events).toHaveLength(3)
    expect(events.every(e => e['@type'] === 'MusicEvent')).toBe(true)
  })

  // The whole reason `starts_at` exists. `event_date` is a scene-local calendar
  // date; re-parsing it would place a Monday-evening Phoenix show on Sunday.
  it('renders startDate in the venue-local zone, with offset', () => {
    const [event] = buildSceneWeekJsonLd(week([show()])).events
    expect(event.startDate).toBe('2026-07-27T20:00:00-07:00')
  })

  it('falls back to the state timezone map when the venue has no zone', () => {
    const [event] = buildSceneWeekJsonLd(week([show({ venue_timezone: '' })])).events
    expect(event.startDate).toBe('2026-07-27T20:00:00-07:00')
  })

  it('carries the venue as a MusicVenue with a PostalAddress', () => {
    const [event] = buildSceneWeekJsonLd(week([show()])).events
    expect(event.location).toMatchObject({
      '@type': 'MusicVenue',
      name: 'Crescent Ballroom',
      url: 'https://psychichomily.com/venues/crescent-ballroom',
      address: {
        '@type': 'PostalAddress',
        streetAddress: '308 N 2nd Ave',
        addressLocality: 'Phoenix',
        addressRegion: 'AZ',
        addressCountry: 'US',
      },
    })
  })

  // Street addresses are withheld for unverified venues by the backend, which
  // sends "". The city-level address must survive that so the event still has
  // a location Google accepts.
  it('keeps a city-level address when the street address is withheld', () => {
    const [event] = buildSceneWeekJsonLd(week([show({ venue_address: '' })])).events
    expect(event.location.address?.streetAddress).toBeUndefined()
    expect(event.location.address?.addressLocality).toBe('Phoenix')
  })

  it('names the bill as performers', () => {
    const [event] = buildSceneWeekJsonLd(
      week([show({ artist_names: ['Riff Wood', 'Dogbreth'] })])
    ).events
    expect(event.performer).toEqual([
      { '@type': 'MusicGroup', name: 'Riff Wood' },
      { '@type': 'MusicGroup', name: 'Dogbreth' },
    ])
  })

  it('omits performer rather than inventing one for a bill-less show', () => {
    const [event] = buildSceneWeekJsonLd(
      week([show({ artist_names: [], title: 'Open Decks' })])
    ).events
    expect(event.performer).toBeUndefined()
    expect(event.name).toBe('Open Decks')
  })

  // The same show is named identically here and on /shows/{slug}: both forward
  // an empty title and let the shared generator compose the name.
  it('composes the event name from the bill and the venue when untitled', () => {
    const [event] = buildSceneWeekJsonLd(week([show()])).events
    expect(event.name).toBe('Riff Wood at Crescent Ballroom')
  })

  it('marks a cancelled show as EventCancelled, and makes no offer for it', () => {
    const [event] = buildSceneWeekJsonLd(
      week([show({ is_cancelled: true, price: 20 })])
    ).events
    expect(event.eventStatus).toBe('https://schema.org/EventCancelled')
    expect(event.offers).toBeUndefined()
  })

  it('marks a sold-out show SoldOut and a live one InStock', () => {
    const soldOut = buildSceneWeekJsonLd(week([show({ is_sold_out: true, price: 20 })]))
    expect(soldOut.events[0].offers?.availability).toBe('https://schema.org/SoldOut')

    const live = buildSceneWeekJsonLd(week([show({ price: 20 })]))
    expect(live.events[0].offers).toMatchObject({
      price: 20,
      priceCurrency: 'USD',
      availability: 'https://schema.org/InStock',
    })
  })

  it('omits offers when no price is recorded', () => {
    const [event] = buildSceneWeekJsonLd(week([show()])).events
    expect(event.offers).toBeUndefined()
  })
})

describe('buildSceneWeekJsonLd — BreadcrumbList', () => {
  it('walks Home → Scenes → scene → week, ending on the archived permalink', () => {
    const { breadcrumb } = buildSceneWeekJsonLd(week([show()]))
    expect(breadcrumb.itemListElement).toEqual([
      { '@type': 'ListItem', position: 1, name: 'Home', item: 'https://psychichomily.com' },
      { '@type': 'ListItem', position: 2, name: 'Scenes', item: 'https://psychichomily.com/scenes' },
      {
        '@type': 'ListItem',
        position: 3,
        name: 'Phoenix, AZ',
        item: 'https://psychichomily.com/scenes/phoenix-az',
      },
      {
        '@type': 'ListItem',
        position: 4,
        name: 'Mon, Jul 27 – Sun, Aug 2, 2026',
        item: 'https://psychichomily.com/scenes/phoenix-az/2026-W31',
      },
    ])
  })

  // A quiet week is still a real page with a real place in the hierarchy.
  it('is present on an empty week', () => {
    const empty = week([])
    const { breadcrumb, itemList, events } = buildSceneWeekJsonLd(empty)
    expect(breadcrumb.itemListElement).toHaveLength(4)
    expect(itemList).toBeUndefined()
    expect(events).toEqual([])
  })
})

describe('buildSceneWeekJsonLd — ItemList', () => {
  it('keeps the existing position/url/name shape', () => {
    const { itemList } = buildSceneWeekJsonLd(week([show(), show({ id: 2, slug: undefined })]))
    expect(itemList).toEqual({
      '@context': 'https://schema.org',
      '@type': 'ItemList',
      name: 'Phoenix, AZ shows, Mon, Jul 27 – Sun, Aug 2, 2026',
      numberOfItems: 2,
      itemListElement: [
        {
          '@type': 'ListItem',
          position: 1,
          url: 'https://psychichomily.com/shows/riff-wood-crescent-ballroom',
          name: 'Riff Wood',
        },
        {
          '@type': 'ListItem',
          position: 2,
          url: 'https://psychichomily.com/shows/2',
          name: 'Riff Wood',
        },
      ],
    })
  })
})

// Google's Event rich result errors on a missing name, startDate, or a location
// without name+address. Everything else is a warning at most, so this asserts
// exactly the set that would turn the Rich Results Test red.
describe('buildSceneWeekJsonLd — required structured-data fields', () => {
  it('every event carries the fields Google requires', () => {
    const { events } = buildSceneWeekJsonLd(
      week([
        show(),
        show({ id: 2, is_cancelled: true, artist_names: [] }),
        show({ id: 3, venue_address: '', slug: undefined, price: 0 }),
      ])
    )
    expect(events).toHaveLength(3)
    for (const event of events) {
      expect(event['@context']).toBe('https://schema.org')
      expect(event.name).toBeTruthy()
      expect(event.startDate).toMatch(/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}[+-]\d{2}:\d{2}$/)
      expect(event.location.name).toBeTruthy()
      expect(event.location.address?.addressLocality).toBeTruthy()
      expect(event.eventStatus).toMatch(/^https:\/\/schema\.org\/Event/)
    }
  })
})
