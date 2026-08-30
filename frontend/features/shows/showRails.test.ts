import { describe, expect, it } from 'vitest'
import {
  alsoTonightHasMore,
  alsoTonightQualifier,
  alsoTonightRailRows,
  alsoTonightRailTitle,
  alsoTonightSeeAllHref,
  moreAtVenueRailRows,
  SHOW_RAIL_ROW_CAP,
  VENUE_RAIL_FETCH_LIMIT,
  type AlsoTonightShow,
  type ShowAlsoTonightResponse,
} from './showRails'
import type { VenueShow } from '@/features/venues/types'

function alsoTonightShow(overrides: Partial<AlsoTonightShow> = {}): AlsoTonightShow {
  return {
    id: 1,
    title: 'A show',
    slug: 'a-show',
    event_date: '2026-08-12',
    starts_at: '2026-08-13T01:00:00Z',
    is_cancelled: false,
    is_sold_out: false,
    ...overrides,
  }
}

function rail(overrides: Partial<ShowAlsoTonightResponse> = {}): ShowAlsoTonightResponse {
  return {
    city: 'Chicago',
    state: 'IL',
    scene_name: 'Chicago, IL',
    scene_slug: 'chicago-il',
    date: '2026-08-12',
    timezone: 'America/Chicago',
    is_tonight: true,
    show_count: 0,
    has_more: false,
    shows: [],
    ...overrides,
  }
}

function venueShow(overrides: Partial<VenueShow> = {}): VenueShow {
  return {
    id: 1,
    slug: 'a-show',
    title: 'A show',
    event_date: '2026-08-15T01:00:00Z',
    city: 'Chicago',
    state: 'IL',
    price: null,
    age_requirement: null,
    is_cancelled: false,
    is_sold_out: false,
    artists: [],
    ...overrides,
  }
}

describe('alsoTonightRailRows', () => {
  it('draws nothing without a payload, so a pending rail is an absent rail', () => {
    expect(alsoTonightRailRows(undefined, 1)).toEqual([])
  })

  it('tolerates the generator-nullable shows array', () => {
    expect(alsoTonightRailRows(rail({ shows: null }), 1)).toEqual([])
  })

  it('drops the subject show even though the endpoint promises to', () => {
    const rows = alsoTonightRailRows(
      rail({
        shows: [
          alsoTonightShow({ id: 7 }),
          alsoTonightShow({ id: 8 }),
        ],
      }),
      7
    )
    expect(rows.map(row => row.id)).toEqual([8])
  })

  it('caps at the mock’s three rows, keeping the earliest', () => {
    const rows = alsoTonightRailRows(
      rail({
        shows: [1, 2, 3, 4, 5].map(id => alsoTonightShow({ id })),
      }),
      99
    )
    expect(rows.map(row => row.id)).toEqual([1, 2, 3])
    expect(SHOW_RAIL_ROW_CAP).toBe(3)
  })
})

describe('moreAtVenueRailRows', () => {
  it('excludes the show being read — a page must not recommend itself', () => {
    const rows = moreAtVenueRailRows(
      [venueShow({ id: 4 }), venueShow({ id: 5 }), venueShow({ id: 6 })],
      4
    )
    expect(rows.map(row => row.id)).toEqual([5, 6])
  })

  it('still fills the rail once the subject show is removed', () => {
    // The reason VENUE_RAIL_FETCH_LIMIT is cap + 1: a full page of rows minus
    // the subject show must still reach the cap.
    const fetched = Array.from({ length: VENUE_RAIL_FETCH_LIMIT }, (_, i) =>
      venueShow({ id: i + 1 })
    )
    expect(moreAtVenueRailRows(fetched, 1)).toHaveLength(SHOW_RAIL_ROW_CAP)
  })

  it('draws nothing without rows', () => {
    expect(moreAtVenueRailRows(undefined, 1)).toEqual([])
    expect(moreAtVenueRailRows([venueShow({ id: 1 })], 1)).toEqual([])
  })
})

describe('alsoTonightQualifier', () => {
  it('says Tonight only when the SCENE says so, never the viewer’s clock', () => {
    expect(alsoTonightQualifier(rail({ is_tonight: true }))).toBe('Tonight')
  })

  it('names the night by its own date otherwise', () => {
    // A show page is read months early and years late; the same rail must not
    // claim "tonight" on either.
    expect(alsoTonightQualifier(rail({ is_tonight: false, date: '2026-08-12' })))
      .toBe('Wed Aug 12')
  })
})

describe('alsoTonightRailTitle', () => {
  it('composes the SECTION / QUALIFIER register with the metro’s city', () => {
    expect(alsoTonightRailTitle(rail())).toBe('Also / Tonight · Chicago')
  })

  it('omits a city it does not have rather than guessing one', () => {
    expect(alsoTonightRailTitle(rail({ city: undefined }))).toBe('Also / Tonight')
  })
})

describe('alsoTonightSeeAllHref', () => {
  it('points at the scene’s own page for that night', () => {
    expect(alsoTonightSeeAllHref(rail())).toBe('/scenes/chicago-il/2026-08-12')
  })

  it('withholds the link when the backend withheld the slug', () => {
    // The backend drops scene_slug precisely when following it would land on a
    // page that does not list the show it came from.
    expect(alsoTonightSeeAllHref(rail({ scene_slug: undefined }))).toBeNull()
    expect(alsoTonightSeeAllHref(rail({ scene_slug: '' }))).toBeNull()
  })

  it('refuses to build a link from a date the scene route would not route', () => {
    expect(alsoTonightSeeAllHref(rail({ date: 'tonight' }))).toBeNull()
    expect(alsoTonightSeeAllHref(rail({ date: '' }))).toBeNull()
  })
})

describe('alsoTonightHasMore', () => {
  it('is true when the backend already truncated the night', () => {
    expect(alsoTonightHasMore(rail({ has_more: true }), 3, 99)).toBe(true)
  })

  it('is true when the rail’s own cap hid a row the payload carried', () => {
    const payload = rail({
      shows: [1, 2, 3, 4].map(id => alsoTonightShow({ id })),
    })
    expect(alsoTonightHasMore(payload, 3, 99)).toBe(true)
  })

  it('is false when every listable row is on screen', () => {
    const payload = rail({
      shows: [1, 2, 3].map(id => alsoTonightShow({ id })),
    })
    expect(alsoTonightHasMore(payload, 3, 99)).toBe(false)
  })

  it('does not count the subject show as something hidden', () => {
    // Four rows, one of them this show: three are listable and three are drawn,
    // so there is nothing behind a "see all".
    const payload = rail({
      shows: [1, 2, 3, 7].map(id => alsoTonightShow({ id })),
    })
    expect(alsoTonightHasMore(payload, 3, 7)).toBe(false)
  })
})
