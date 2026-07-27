import { describe, it, expect } from 'vitest'
import type { VenueWithShowCount } from '@/features/venues/types'
import type { PlaceableScene, VenuePin } from './components/globeTypes'
import {
  CITY_VIEW_MIN_ZOOM,
  VENUE_PIN_CAP_COUNT,
  labelledVenuePinIds,
  venuePinRadiusPx,
  cityDataUpdatedAt,
  cityGenreFamilies,
  cityRailStats,
  filterCityVenues,
  formatNextShowDate,
  formatPanelShowDate,
  nextShowBill,
  resolveCityScene,
  venuePanelIdentityLine,
  venuePanelShowCount,
  venuePinPosition,
} from './cityView'

function scene(overrides: Partial<PlaceableScene> = {}): PlaceableScene {
  return {
    city: 'Austin',
    state: 'TX',
    slug: 'austin-tx',
    venue_count: 6,
    upcoming_show_count: 52,
    total_show_count: 300,
    shows_this_week: 9,
    latitude: 30.2672,
    longitude: -97.7431,
    ...overrides,
  } as PlaceableScene
}

function venue(overrides: Partial<VenueWithShowCount> = {}): VenueWithShowCount {
  return {
    id: 1,
    slug: 'mohawk-austin-tx',
    name: 'Mohawk',
    address: null,
    city: 'Austin',
    state: 'TX',
    verified: true,
    latitude: 30.2672,
    longitude: -97.7431,
    upcoming_show_count: 14,
    shows_this_week: 3,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-07-25T00:00:00Z',
    ...overrides,
  } as VenueWithShowCount
}

describe('resolveCityScene', () => {
  it('claims no city above the street-zoom threshold', () => {
    expect(
      resolveCityScene([scene()], {
        lat: 30.2672,
        lng: -97.7431,
        zoom: CITY_VIEW_MIN_ZOOM - 0.1,
      }),
    ).toBeNull()
  })

  it('claims the scene the camera is centred on at street zoom', () => {
    const austin = scene()
    expect(
      resolveCityScene([austin], { lat: 30.27, lng: -97.74, zoom: 13.2 }),
    ).toBe(austin)
  })

  it('picks the NEAREST scene when several are in range', () => {
    const austin = scene()
    const sanMarcos = scene({
      city: 'San Marcos',
      slug: 'san-marcos-tx',
      latitude: 29.8833,
      longitude: -97.9414,
    })
    expect(
      resolveCityScene([austin, sanMarcos], {
        lat: 29.89,
        lng: -97.94,
        zoom: 13,
      }),
    ).toBe(sanMarcos)
  })

  it('claims nothing when the camera is out in open country', () => {
    // ~500 km west of Austin, well past the claim radius.
    expect(
      resolveCityScene([scene()], { lat: 30.27, lng: -102.9, zoom: 13 }),
    ).toBeNull()
  })

  it('claims nothing when there are no scenes at all', () => {
    expect(
      resolveCityScene([], { lat: 30.27, lng: -97.74, zoom: 13 }),
    ).toBeNull()
  })
})

describe('venuePinPosition (PSY-1536 privacy gate)', () => {
  it('uses street coordinates when the API served them', () => {
    expect(
      venuePinPosition(
        venue({ street_latitude: 30.2686, street_longitude: -97.7376 }),
      ),
    ).toEqual({ lat: 30.2686, lng: -97.7376, precision: 'street' })
  })

  it('falls back to the city centroid when street coords are ABSENT', () => {
    // The API omits street coords for unverified venues and stale geocodes.
    // The pin must land on the centroid, never be reconstructed some other way.
    expect(venuePinPosition(venue())).toEqual({
      lat: 30.2672,
      lng: -97.7431,
      precision: 'centroid',
    })
  })

  it('falls back to the centroid when street coords are explicitly null', () => {
    expect(
      venuePinPosition(
        venue({ street_latitude: null, street_longitude: null }),
      ),
    ).toEqual({ lat: 30.2672, lng: -97.7431, precision: 'centroid' })
  })

  it('does not pin on a half-present street geocode', () => {
    expect(
      venuePinPosition(
        venue({ street_latitude: 30.2686, street_longitude: null }),
      )?.precision,
    ).toBe('centroid')
  })

  it('returns null when the venue has no coordinates at all', () => {
    expect(
      venuePinPosition(venue({ latitude: null, longitude: null })),
    ).toBeNull()
  })
})

function pin(overrides: Partial<VenuePin> = {}): VenuePin {
  return {
    id: 1,
    name: 'Mohawk',
    lng: -97.7431,
    lat: 30.2672,
    upcomingShowCount: 14,
    nextShowLabel: '',
    ...overrides,
  }
}

describe('venuePinRadiusPx', () => {
  it('grows with the upcoming count', () => {
    expect(venuePinRadiusPx(10)).toBeGreaterThan(venuePinRadiusPx(1))
  })

  it('caps, so one huge venue cannot swallow the block', () => {
    expect(venuePinRadiusPx(VENUE_PIN_CAP_COUNT * 20)).toBe(
      venuePinRadiusPx(VENUE_PIN_CAP_COUNT),
    )
  })

  it('never returns NaN for malformed counts', () => {
    expect(Number.isFinite(venuePinRadiusPx(Number.NaN))).toBe(true)
    expect(Number.isFinite(venuePinRadiusPx(-5))).toBe(true)
  })
})

describe('labelledVenuePinIds', () => {
  it('labels every venue when none collide', () => {
    const ids = labelledVenuePinIds([
      pin({ id: 1, lat: 30.2672, lng: -97.7431 }),
      pin({ id: 2, lat: 30.29, lng: -97.72 }),
    ])
    expect([...ids].sort()).toEqual([1, 2])
  })

  it('labels only the busiest of venues stacked on the city centroid', () => {
    // The exact case the centroid fallback creates: several venues, one point.
    const ids = labelledVenuePinIds([
      pin({ id: 1, upcomingShowCount: 4 }),
      pin({ id: 2, upcomingShowCount: 11 }),
      pin({ id: 3, upcomingShowCount: 0 }),
    ])
    expect([...ids]).toEqual([2])
  })

  it('keeps both when a pair clears the declutter radius', () => {
    // ~1 km apart, well outside the 200 m radius.
    const ids = labelledVenuePinIds([
      pin({ id: 1, lat: 30.2672 }),
      pin({ id: 2, lat: 30.2762 }),
    ])
    expect([...ids].sort()).toEqual([1, 2])
  })

  it('is empty for no pins', () => {
    expect(labelledVenuePinIds([]).size).toBe(0)
  })
})

describe('filterCityVenues', () => {
  const mohawk = venue({ id: 1, name: 'Mohawk', shows_this_week: 3, dominant_genre: 'punk_hardcore' })
  const hotelVegas = venue({ id: 2, name: 'Hotel Vegas', shows_this_week: 0, dominant_genre: 'rock_indie' })
  const untinted = venue({ id: 3, name: 'Chess Club', shows_this_week: 1, dominant_genre: undefined })
  const all = [mohawk, hotelVegas, untinted]

  it('passes everything through with no filters', () => {
    expect(
      filterCityVenues(all, { thisWeekOnly: false, genreFamily: null }),
    ).toEqual(all)
  })

  it('drops venues with nothing booked this week', () => {
    expect(
      filterCityVenues(all, { thisWeekOnly: true, genreFamily: null }),
    ).toEqual([mohawk, untinted])
  })

  it('narrows to one genre family', () => {
    expect(
      filterCityVenues(all, { thisWeekOnly: false, genreFamily: 'rock_indie' }),
    ).toEqual([hotelVegas])
  })

  it('excludes untinted venues from any genre-family filter', () => {
    expect(
      filterCityVenues(all, { thisWeekOnly: false, genreFamily: 'punk_hardcore' }),
    ).toEqual([mohawk])
  })

  it('applies both filters together', () => {
    expect(
      filterCityVenues(all, { thisWeekOnly: true, genreFamily: 'rock_indie' }),
    ).toEqual([])
  })

  it('treats a missing shows_this_week as zero', () => {
    const noField = venue({ id: 4, shows_this_week: undefined })
    expect(
      filterCityVenues([noField], { thisWeekOnly: true, genreFamily: null }),
    ).toEqual([])
  })
})

describe('cityGenreFamilies', () => {
  it('offers only families present in this city, in legend order', () => {
    const families = cityGenreFamilies([
      venue({ id: 1, dominant_genre: 'electronic' }),
      venue({ id: 2, dominant_genre: 'punk_hardcore' }),
      venue({ id: 3, dominant_genre: 'punk_hardcore' }),
      venue({ id: 4, dominant_genre: undefined }),
    ])
    // GENRE_FAMILIES order is punk_hardcore, rock_indie, electronic, ...
    expect(families.map((f) => f.key)).toEqual(['punk_hardcore', 'electronic'])
  })

  it('is empty when nothing is tinted', () => {
    expect(cityGenreFamilies([venue({ dominant_genre: undefined })])).toEqual([])
  })
})

describe('cityRailStats', () => {
  it('sums the rows it is given, so the header cannot contradict the list', () => {
    expect(
      cityRailStats([
        venue({ id: 1, upcoming_show_count: 14, shows_this_week: 3 }),
        venue({ id: 2, upcoming_show_count: 11, shows_this_week: 0 }),
      ]),
    ).toEqual({ venueCount: 2, upcomingCount: 25, thisWeekCount: 3 })
  })

  it('zeroes out on an empty city', () => {
    expect(cityRailStats([])).toEqual({
      venueCount: 0,
      upcomingCount: 0,
      thisWeekCount: 0,
    })
  })
})

describe('cityDataUpdatedAt', () => {
  it('reports the most recently touched venue row', () => {
    expect(
      cityDataUpdatedAt([
        venue({ id: 1, updated_at: '2026-07-20T00:00:00Z' }),
        venue({ id: 2, updated_at: '2026-07-25T12:00:00Z' }),
        venue({ id: 3, updated_at: '2026-07-01T00:00:00Z' }),
      ]),
    ).toBe('2026-07-25T12:00:00Z')
  })

  it('is null with nothing to report', () => {
    expect(cityDataUpdatedAt([])).toBeNull()
  })

  it('ignores unparseable timestamps rather than reporting one', () => {
    expect(cityDataUpdatedAt([venue({ updated_at: 'not-a-date' })])).toBeNull()
  })
})

describe('nextShowBill', () => {
  it('prefers the show title when it has one', () => {
    expect(
      nextShowBill(
        venue({
          next_show_title: 'Levitation pre-party',
          next_show_artists: ['Some Band'],
        }),
      ),
    ).toBe('Levitation pre-party')
  })

  it('composes the bill when the show is titleless (the common case)', () => {
    expect(
      nextShowBill(
        venue({
          next_show_title: '',
          next_show_artists: ['Gouge Away', 'Militarie Gun'],
        }),
      ),
    ).toBe('Gouge Away / Militarie Gun')
  })

  it('is empty when the venue has neither', () => {
    expect(nextShowBill(venue())).toBe('')
  })
})

describe('formatNextShowDate', () => {
  it('renders the date the backend resolved, with no zone shift', () => {
    // The backend already resolved this in the VENUE's timezone. Parsing it as
    // UTC midnight would render Jul 27 anywhere west of Greenwich.
    expect(formatNextShowDate('2026-07-28')).toBe('Tue, Jul 28')
  })

  it('is empty for a missing date', () => {
    expect(formatNextShowDate(undefined)).toBe('')
    expect(formatNextShowDate(null)).toBe('')
    expect(formatNextShowDate('')).toBe('')
  })

  it('is empty for a malformed date rather than "Invalid Date"', () => {
    expect(formatNextShowDate('2026-07-28T21:00:00Z')).toBe('')
    expect(formatNextShowDate('nope')).toBe('')
  })
})

// ── Venue panel (PSY-1540) ────────────────────────────────────────────────

describe('formatPanelShowDate', () => {
  it('renders the date column in the VENUE timezone, not the viewer’s', () => {
    // 2026-08-01T02:00:00Z is 9pm on JUL 31 in Austin. A panel describing
    // Hotel Vegas's calendar must say FRI 7/31 whatever zone the reader is
    // in — the venue-timezone convention (PSY-985/986).
    expect(
      formatPanelShowDate('2026-08-01T02:00:00Z', 'TX', 'America/Chicago'),
    ).toBe('FRI 7/31')
  })

  it('applies the zone rather than deferring to the viewer', () => {
    // Same instant read in two venue zones lands on two different local days.
    // If the zone were being ignored these would agree.
    const instant = '2026-08-01T04:30:00Z'
    expect(formatPanelShowDate(instant, 'TX', 'America/Chicago')).toBe(
      'FRI 7/31',
    )
    expect(formatPanelShowDate(instant, 'NY', 'Europe/Berlin')).toBe('SAT 8/1')
  })

  it('falls back to the state map when the venue has no IANA zone yet', () => {
    // Pre-backfill rows carry a null timezone; resolveShowTimezone maps the
    // state rather than silently using the viewer's zone.
    expect(formatPanelShowDate('2026-08-01T02:00:00Z', 'TX', null)).toBe(
      'FRI 7/31',
    )
  })

  it('is empty for a missing or unparseable timestamp, never "Invalid Date"', () => {
    expect(formatPanelShowDate(undefined, 'TX', 'America/Chicago')).toBe('')
    expect(formatPanelShowDate(null, 'TX', 'America/Chicago')).toBe('')
    expect(formatPanelShowDate('nope', 'TX', 'America/Chicago')).toBe('')
  })
})

describe('venuePanelIdentityLine', () => {
  it('joins the address, capacity and place', () => {
    expect(
      venuePanelIdentityLine({
        address: '1502 E 6th St',
        capacity: 250,
        city: 'Austin',
        state: 'TX',
      }),
    ).toBe('1502 E 6th St · cap ~250 · Austin, TX')
  })

  it('omits the address an unverified venue’s API response withheld', () => {
    // PRIVACY GATE (PSY-1536): the backend nulls `address` for unverified
    // venues exactly as it withholds their street coordinates. The panel must
    // publish neither — a leak here would defeat the map-side gate.
    expect(
      venuePanelIdentityLine({
        address: null,
        capacity: null,
        city: 'Austin',
        state: 'TX',
      }),
    ).toBe('Austin, TX')
  })

  it('omits a blank address rather than emitting an empty segment', () => {
    expect(
      venuePanelIdentityLine({
        address: '   ',
        capacity: 250,
        city: 'Austin',
        state: 'TX',
      }),
    ).toBe('cap ~250 · Austin, TX')
  })

  it('omits a missing or nonsensical capacity', () => {
    const base = { address: '1502 E 6th St', city: 'Austin', state: 'TX' }
    expect(venuePanelIdentityLine({ ...base, capacity: null })).toBe(
      '1502 E 6th St · Austin, TX',
    )
    expect(venuePanelIdentityLine({ ...base, capacity: 0 })).toBe(
      '1502 E 6th St · Austin, TX',
    )
  })
})

describe('venuePanelShowCount', () => {
  it('counts what it actually listed when the page was complete', () => {
    // 12 rows came back under the 50 cap, two of them duplicates. Claiming
    // the API's 12 would double-count rows the reader can see are gone.
    expect(
      venuePanelShowCount({ total: 12, listed: 10, fetched: 12, limit: 50 }),
    ).toBe(10)
  })

  it('trusts the API total when the fetch limit truncated the page', () => {
    expect(
      venuePanelShowCount({ total: 137, listed: 50, fetched: 50, limit: 50 }),
    ).toBe(137)
  })

  it('falls back to the listed count when the API sent no total', () => {
    expect(
      venuePanelShowCount({
        total: undefined,
        listed: 50,
        fetched: 50,
        limit: 50,
      }),
    ).toBe(50)
  })

  it('is zero for a venue with nothing booked', () => {
    expect(
      venuePanelShowCount({ total: 0, listed: 0, fetched: 0, limit: 50 }),
    ).toBe(0)
  })
})
