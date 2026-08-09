import { describe, it, expect } from 'vitest'
import { buildSceneDayJsonLd } from './sceneDayJsonLd'
import type { SceneDayResponse, SceneDayShow } from './sceneDay'

const show = (over: Partial<SceneDayShow> = {}): SceneDayShow =>
  ({
    id: 1,
    title: '',
    event_date: '2026-07-31',
    // 20:00 Phoenix on the 31st — the instant a naive date-only parse gets
    // wrong, since UTC midnight on the 31st is the 30th in Arizona.
    starts_at: '2026-08-01T03:00:00Z',
    is_sold_out: false,
    is_cancelled: false,
    slug: 'smooth-hands-valley-bar',
    venue_name: 'Valley Bar',
    venue_slug: 'valley-bar',
    venue_address: '130 N Central Ave',
    venue_city: 'Phoenix',
    venue_state: 'AZ',
    venue_country: 'US',
    venue_timezone: 'America/Phoenix',
    artist_names: ['Smooth Hands', 'Tournament'],
    ...over,
  }) as SceneDayShow

const day = (over: Partial<SceneDayResponse> = {}): SceneDayResponse =>
  ({
    slug: 'phoenix-az',
    scene_name: 'Phoenix, AZ',
    city: 'Phoenix',
    state: 'AZ',
    date: '2026-07-31',
    timezone: 'America/Phoenix',
    iso_week: '2026-W31',
    show_count: 1,
    prev_date: '2026-07-30',
    next_date: '2026-08-01',
    is_tonight: true,
    is_past_day: false,
    shows: [show()],
    tracked_venues: [],
    ...over,
  }) as SceneDayResponse

const NOW = new Date('2026-07-31T12:00:00Z')

describe('buildSceneDayJsonLd', () => {
  // The leaf names this NIGHT, from both day routes. /tonight would go stale
  // tomorrow, and the week permalink the rolling route canonicalizes to names
  // seven nights rather than this one.
  it('anchors the breadcrumb leaf on the dated permalink', () => {
    const { breadcrumb } = buildSceneDayJsonLd(day(), NOW)
    const items = breadcrumb.itemListElement
    expect(items[items.length - 1]).toMatchObject({
      name: 'Friday, July 31, 2026',
      item: 'https://psychichomily.com/scenes/phoenix-az/2026-07-31',
    })
  })

  it('emits one MusicEvent per describable show, with the venue-local start', () => {
    const { events } = buildSceneDayJsonLd(day(), NOW)
    expect(events).toHaveLength(1)
    // -07:00 is Phoenix; a UTC startDate would place this show on Aug 1.
    expect(events[0].startDate).toBe('2026-07-31T20:00:00-07:00')
    expect(events[0].location).toMatchObject({ name: 'Valley Bar' })
    expect(events[0].performer).toEqual([
      { '@type': 'MusicGroup', name: 'Smooth Hands' },
      { '@type': 'MusicGroup', name: 'Tournament' },
    ])
  })

  it('lists every show in the ItemList, through the same href the rows link', () => {
    const { itemList } = buildSceneDayJsonLd(day(), NOW)
    expect(itemList?.itemListElement[0]).toMatchObject({
      url: 'https://psychichomily.com/shows/smooth-hands-valley-bar',
      name: 'Smooth Hands, Tournament',
    })
  })

  // Google requires name, startDate and a located venue; publishing a
  // half-event is worse than publishing none. The show is still LISTED.
  it('lists but does not describe a show with no venue or no start instant', () => {
    const { itemList, events } = buildSceneDayJsonLd(
      day({
        show_count: 2,
        shows: [show({ id: 2, venue_name: '' }), show({ id: 3, starts_at: undefined as never })],
      }),
      NOW
    )
    expect(itemList?.itemListElement).toHaveLength(2)
    expect(events).toHaveLength(0)
  })

  // An ItemList of nothing says nothing, and an empty night carries noindex
  // anyway — but the breadcrumb still has to place the page.
  it('emits a breadcrumb and nothing else for a quiet night', () => {
    const result = buildSceneDayJsonLd(day({ show_count: 0, shows: [] }), NOW)
    expect(result.itemList).toBeUndefined()
    expect(result.events).toEqual([])
    expect(result.breadcrumb.itemListElement).not.toHaveLength(0)
  })

  it('survives a null show list', () => {
    const result = buildSceneDayJsonLd(day({ show_count: 0, shows: null }), NOW)
    expect(result.events).toEqual([])
  })
})
