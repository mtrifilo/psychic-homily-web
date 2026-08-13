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
  venue_country: 'US',
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
  is_past_week: false,
  tracked_venues: [{ name: 'Crescent Ballroom' }],
  days: [
    { date: '2026-07-27', shows },
    { date: '2026-07-28', shows: [] },
  ],
})

/** Before the fixture week, so its shows read as upcoming. */
const BEFORE = new Date('2026-07-20T00:00:00Z')
/** After it, so the same shows read as over. */
const AFTER = new Date('2026-09-01T00:00:00Z')

const build = (shows: SceneWeekShow[], now = BEFORE) => buildSceneWeekJsonLd(week(shows), now)

describe('buildSceneWeekJsonLd — MusicEvent', () => {
  it('emits one event per listed show, across days', () => {
    const data = week([])
    data.days = [
      { date: '2026-07-27', shows: [show({ id: 1 })] },
      { date: '2026-07-28', shows: [] },
      { date: '2026-07-29', shows: [show({ id: 2 }), show({ id: 3 })] },
    ]
    const { events } = buildSceneWeekJsonLd(data, BEFORE)
    expect(events).toHaveLength(3)
    expect(events.every(e => e['@type'] === 'MusicEvent')).toBe(true)
  })

  // The whole reason `starts_at` exists. `event_date` is a scene-local calendar
  // date; re-parsing it would place a Monday-evening Phoenix show on Sunday.
  it('renders startDate in the venue-local zone, with offset', () => {
    const [event] = build([show()]).events
    expect(event.startDate).toBe('2026-07-27T20:00:00-07:00')
  })

  // The backend bucketed the show into its day using the SCENE's zone, so that
  // is the fallback — not the state map, which can name a different zone and
  // put the event on a different date than the heading it sits under.
  it('falls back to the scene timezone, not the state map, when the venue has none', () => {
    const [event] = build([show({ venue_timezone: '' })]).events
    expect(event.startDate).toBe('2026-07-27T20:00:00-07:00')
  })

  it('carries the venue as a MusicVenue with a PostalAddress', () => {
    const [event] = build([show()]).events
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

  // Scenes are not US-only. Stamping `US` on a Toronto venue is a
  // machine-readable false statement, repeated once per show on the page.
  it('publishes the venue’s own country', () => {
    const [event] = build([show({ venue_country: 'CA', venue_city: 'Toronto', venue_state: 'ON' })])
      .events
    expect(event.location.address?.addressCountry).toBe('CA')
  })

  it('defaults the country to US when the venue has none recorded', () => {
    const [event] = build([show({ venue_country: '' })]).events
    expect(event.location.address?.addressCountry).toBe('US')
  })

  // Street addresses are withheld for unverified venues by the backend, which
  // sends "". The city-level address must survive that so the event still has
  // a location Google accepts.
  it('keeps a city-level address when the street address is withheld', () => {
    const [event] = build([show({ venue_address: '' })]).events
    expect(event.location.address?.streetAddress).toBeUndefined()
    expect(event.location.address?.addressLocality).toBe('Phoenix')
  })

  it('names the bill as performers', () => {
    const [event] = build([show({ artist_names: ['Riff Wood', 'Dogbreth'] })]).events
    expect(event.performer).toEqual([
      { '@type': 'MusicGroup', name: 'Riff Wood' },
      { '@type': 'MusicGroup', name: 'Dogbreth' },
    ])
  })

  it('omits performer rather than inventing one for a bill-less show', () => {
    const [event] = build([show({ artist_names: [], title: 'Open Decks' })]).events
    expect(event.performer).toBeUndefined()
    expect(event.name).toBe('Open Decks')
  })

  it('composes the event name from the bill and the venue when untitled', () => {
    const [event] = build([show()]).events
    expect(event.name).toBe('Riff Wood at Crescent Ballroom')
  })
})

// A show we cannot describe must not be published as a half-event: Google
// requires name + startDate + a located address, and a broken block is worse
// than none. The show stays in the ItemList either way.
describe('buildSceneWeekJsonLd — undescribable shows', () => {
  // `starts_at` is typed required, but the frontend and backend deploy
  // separately and Next's data cache outlives a deploy. Before this guard an
  // old payload threw a RangeError out of a server component — a 500 for the
  // whole page, not a missing field.
  it.each([
    ['missing', undefined],
    ['empty', ''],
    ['unparseable', 'not-a-date'],
  ])('drops a show whose starts_at is %s, without throwing', (_label, startsAt) => {
    const broken = show({ id: 9, starts_at: startsAt as unknown as string })
    const built = build([show(), broken])
    expect(built.events).toHaveLength(1)
    expect(built.itemList?.numberOfItems).toBe(2)
  })

  it('drops a show with no venue, keeping it in the list', () => {
    const built = build([show(), show({ id: 9, venue_name: undefined })])
    expect(built.events).toHaveLength(1)
    expect(built.itemList?.numberOfItems).toBe(2)
  })
})

describe('buildSceneWeekJsonLd — offers and status', () => {
  it('marks a cancelled show as EventCancelled, and makes no offer for it', () => {
    const [event] = build([show({ is_cancelled: true, price: 20 })]).events
    expect(event.eventStatus).toBe('https://schema.org/EventCancelled')
    expect(event.offers).toBeUndefined()
  })

  it('marks a sold-out show SoldOut and a live one InStock', () => {
    const soldOut = build([show({ is_sold_out: true, price: 20 })])
    expect(soldOut.events[0].offers?.availability).toBe('https://schema.org/SoldOut')

    const live = build([show({ price: 20 })])
    expect(live.events[0].offers).toMatchObject({
      price: 20,
      priceCurrency: 'USD',
      availability: 'https://schema.org/InStock',
    })
  })

  // Most ingested shows have no price. `availability` is schema.org's only
  // channel for sold-out, so gating it on a price would leave the SOLD OUT
  // badge the page renders with no machine-readable counterpart.
  it('still says SoldOut when no price is recorded', () => {
    const [event] = build([show({ is_sold_out: true })]).events
    expect(event.offers?.availability).toBe('https://schema.org/SoldOut')
    expect(event.offers?.price).toBeUndefined()
    expect(event.offers?.priceCurrency).toBeUndefined()
  })

  it('omits offers for an available show with no price', () => {
    const [event] = build([show()]).events
    expect(event.offers).toBeUndefined()
  })

  // The archive reaches back years. An offer is a claim about what a reader can
  // still buy, so a week that is over makes none.
  it('makes no offer for a show that already happened', () => {
    const priced = buildSceneWeekJsonLd(week([show({ price: 20 })]), AFTER)
    expect(priced.events[0].offers).toBeUndefined()

    const soldOut = buildSceneWeekJsonLd(week([show({ is_sold_out: true })]), AFTER)
    expect(soldOut.events[0].offers).toBeUndefined()

    // Still a real event that happened — only the offer goes away.
    expect(priced.events[0].eventStatus).toBe('https://schema.org/EventScheduled')
    expect(priced.events[0].startDate).toBe('2026-07-27T20:00:00-07:00')
  })

  // An `Offer` is a claim that a reader can still BUY a ticket, so the boundary
  // is the start instant, not the venue-local calendar day the share card is
  // cached against. Stretching it to local midnight would advertise tickets for
  // a show already in progress, and for nearly a full day on an after-midnight
  // one. The fixture starts 20:00 Phoenix on Jul 27.
  it('drops the offer the moment doors open, not at venue-local midnight', () => {
    const oneMinuteBefore = new Date('2026-07-28T02:59:00Z') // 19:59 Jul 27 Phoenix
    expect(
      buildSceneWeekJsonLd(week([show({ price: 20 })]), oneMinuteBefore).events[0].offers
    ).toBeDefined()

    const doorsOpen = new Date('2026-07-28T03:00:00Z') // 20:00 Jul 27 Phoenix
    expect(
      buildSceneWeekJsonLd(week([show({ price: 20 })]), doorsOpen).events[0].offers
    ).toBeUndefined()

    // Still the venue's Jul 27, so a venue-local-day rule would keep offering.
    const midSet = new Date('2026-07-28T04:30:00Z') // 21:30 Jul 27 Phoenix
    expect(
      buildSceneWeekJsonLd(week([show({ price: 20 })]), midSet).events[0].offers
    ).toBeUndefined()
  })
})

describe('buildSceneWeekJsonLd — BreadcrumbList', () => {
  it('walks Home → Scenes → scene → week, ending on the archived permalink', () => {
    const { breadcrumb } = build([show()])
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
    const { breadcrumb, itemList, events } = build([])
    expect(breadcrumb.itemListElement).toHaveLength(4)
    expect(itemList).toBeUndefined()
    expect(events).toEqual([])
  })
})

describe('buildSceneWeekJsonLd — ItemList', () => {
  it('keeps the existing position/url/name shape', () => {
    const { itemList } = build([show(), show({ id: 2, slug: undefined })])
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
    const { events } = build([
      show(),
      show({ id: 2, is_cancelled: true, artist_names: [] }),
      show({ id: 3, venue_address: '', slug: undefined, price: 0 }),
      show({ id: 4, venue_country: 'CA', venue_timezone: '' }),
      show({ id: 5, is_sold_out: true }),
    ])
    expect(events).toHaveLength(5)
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
