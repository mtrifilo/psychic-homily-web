import { describe, it, expect } from 'vitest'
import { buildSceneSliceJsonLd } from './sceneSliceJsonLd'
import { buildSceneSlice } from './sceneSlice'
import { countWindowShows } from './sceneWindow'
import type { SceneDayResponse, SceneDayShow } from './sceneDay'

const show = (over: Partial<SceneDayShow> = {}): SceneDayShow =>
  ({
    id: 1,
    title: '',
    event_date: '2026-07-31',
    // 20:00 Phoenix on the 31st. UTC midnight on the 31st is the 30th in
    // Arizona, which is the instant a naive date-only parse gets wrong.
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

/** Tonight plus the next full day, assembled the way the route assembles it. */
function twoDaySlice() {
  return buildSceneSlice(
    day({
      shows: [
        show({ id: 1, slug: 'smooth-hands-valley-bar' }),
        show({ id: 2, slug: 'tournament-rebel-lounge', venue_name: 'Rebel Lounge' }),
      ],
    }),
    day({
      date: '2026-08-01',
      is_tonight: false,
      prev_date: '2026-07-31',
      next_date: '2026-08-02',
      shows: [
        show({
          id: 3,
          slug: 'holy-fawn-crescent',
          event_date: '2026-08-01',
          starts_at: '2026-08-02T03:00:00Z',
          venue_name: 'Crescent Ballroom',
        }),
      ],
    })
  )
}

describe('buildSceneSliceJsonLd', () => {
  // The invariant this builder exists for: the root's structured data lists
  // exactly the shows the root renders. Both counts are pinned, because they
  // answer different questions: the ItemList lists every rendered row, and
  // `events` publishes the describable subset of them.
  it('describes exactly the shows the slice renders', () => {
    const slice = twoDaySlice()!
    const jsonLd = buildSceneSliceJsonLd(slice, NOW)!

    expect(countWindowShows(slice.days)).toBe(3)
    expect(jsonLd.itemList?.numberOfItems).toBe(3)
    expect(jsonLd.itemList?.itemListElement).toHaveLength(3)
    expect(jsonLd.events).toHaveLength(3)
  })

  it('crosses the day boundary rather than stopping at tonight', () => {
    const jsonLd = buildSceneSliceJsonLd(twoDaySlice()!, NOW)!

    expect(jsonLd.events.map(e => e.startDate)).toEqual([
      '2026-07-31T20:00:00-07:00',
      '2026-07-31T20:00:00-07:00',
      '2026-08-01T20:00:00-07:00',
    ])
  })

  it('lists shows through the same href the rows link', () => {
    const jsonLd = buildSceneSliceJsonLd(twoDaySlice()!, NOW)!

    expect(jsonLd.itemList?.itemListElement[0]).toMatchObject({
      url: 'https://psychichomily.com/shows/smooth-hands-valley-bar',
      name: 'Smooth Hands, Tournament',
    })
  })

  it('names the ItemList after the two dates it covers', () => {
    const jsonLd = buildSceneSliceJsonLd(twoDaySlice()!, NOW)!

    expect(jsonLd.itemList?.name).toBe('Phoenix, AZ shows, Fri, Jul 31 – Sat, Aug 1, 2026')
  })

  // The leaf is the root, which is this page's canonical. A week or a night
  // permalink there would name a page with different content.
  it('anchors the breadcrumb leaf on the scene root', () => {
    const jsonLd = buildSceneSliceJsonLd(twoDaySlice()!, NOW)!
    const items = jsonLd.breadcrumb.itemListElement

    expect(items).toHaveLength(3)
    expect(items[items.length - 1]).toMatchObject({
      name: 'Phoenix, AZ',
      item: 'https://psychichomily.com/scenes/phoenix-az',
    })
  })

  // `/scenes/mesa-az` renders the Phoenix scene. The payload's own slug is the
  // canonical one, and it is what the rendered links already use.
  it('describes the canonical scene the payload names', () => {
    const slice = buildSceneSlice(day({ slug: 'phoenix-az' }), null)!
    const jsonLd = buildSceneSliceJsonLd(slice, NOW)!
    const items = jsonLd.breadcrumb.itemListElement

    expect(items[items.length - 1]).toMatchObject({
      item: 'https://psychichomily.com/scenes/phoenix-az',
    })
  })

  // Honest zero: a quiet night renders no rows, so it publishes no list and no
  // events. Same shape the day, week and window builders return for an empty
  // payload. An ItemList of nothing says nothing.
  it('publishes no list and no events for a quiet slice', () => {
    const slice = buildSceneSlice(
      day({ shows: [] }),
      day({ date: '2026-08-01', is_tonight: false, shows: [] })
    )!
    const jsonLd = buildSceneSliceJsonLd(slice, NOW)!

    expect(countWindowShows(slice.days)).toBe(0)
    expect(jsonLd.itemList).toBeUndefined()
    expect(jsonLd.events).toEqual([])
    expect(jsonLd.breadcrumb.itemListElement).toHaveLength(3)
  })

  // A one-day slice is what the far edge of the servable window produces.
  it('describes a single day when the slice holds one', () => {
    const slice = buildSceneSlice(day({ next_date: '' }), null)!
    const jsonLd = buildSceneSliceJsonLd(slice, NOW)!

    expect(jsonLd.itemList?.name).toBe('Phoenix, AZ shows, Fri, Jul 31, 2026')
    expect(jsonLd.events).toHaveLength(1)
  })

  // A show with no venue cannot produce a valid MusicEvent, so it is listed and
  // linked but not published as an event. The same rule every scene surface
  // applies, through the shared `sceneShowEvents`.
  it('lists an undescribable show without publishing it as an event', () => {
    const slice = buildSceneSlice(
      day({ shows: [show(), show({ id: 2, slug: 'tba-show', venue_name: '' })] }),
      null
    )!
    const jsonLd = buildSceneSliceJsonLd(slice, NOW)!

    expect(jsonLd.itemList?.numberOfItems).toBe(2)
    expect(jsonLd.events).toHaveLength(1)
  })

  // Without a payload there is no scene name or canonical slug to describe.
  it('returns null when the slice named no day', () => {
    expect(buildSceneSliceJsonLd({ days: [] }, NOW)).toBeNull()
  })
})
