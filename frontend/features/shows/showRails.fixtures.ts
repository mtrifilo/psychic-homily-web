import type { VenueShow, VenueShowsResponse } from '@/features/venues/types'
import type { AlsoTonightShow, ShowAlsoTonightResponse } from './showRails'
import type { ShowResponse, VenueResponse } from './types'

/**
 * Rail fixtures, shared by the policy test and the component test.
 *
 * One set of builders on purpose: the two suites cover the same two payloads
 * from opposite sides, and when each kept its own builders they drifted — the
 * component copy carried `venue_name`/`price` the policy copy omitted, so a
 * field could be exercised on one side and silently absent on the other. These
 * defaults are the FULLER ones, and every field a rail reads is populated, so
 * a test that cares about an absence has to ask for it explicitly.
 *
 * A `.fixtures.ts` rather than an export from one of the test files: importing
 * across `.test` files makes the two suites' setup order matter, and vitest
 * would collect this file as a suite with no tests in it.
 */

/** 8PM Aug 12 in Chicago, the instant the mock's first also-tonight row sits on. */
export const CHICAGO_8PM_AUG_12 = '2026-08-13T01:00:00Z'

/** 8PM Aug 15 in Chicago, the instant the mock's first venue row sits on. */
export const CHICAGO_8PM_AUG_15 = '2026-08-16T01:00:00Z'

export function makeAlsoTonightShow(
  overrides: Partial<AlsoTonightShow> = {}
): AlsoTonightShow {
  return {
    id: 100,
    title: 'Dehd',
    slug: 'dehd-lifeguard',
    event_date: '2026-08-12',
    starts_at: CHICAGO_8PM_AUG_12,
    artist_names: ['Dehd', 'Lifeguard'],
    venue_name: 'Empty Bottle',
    venue_slug: 'empty-bottle',
    venue_city: 'Chicago',
    venue_state: 'IL',
    venue_timezone: 'America/Chicago',
    price: 15,
    is_cancelled: false,
    is_sold_out: false,
    ...overrides,
  }
}

export function makeAlsoTonightPayload(
  overrides: Partial<ShowAlsoTonightResponse> = {}
): ShowAlsoTonightResponse {
  return {
    city: 'Chicago',
    state: 'IL',
    scene_name: 'Chicago, IL',
    scene_slug: 'chicago-il',
    date: '2026-08-12',
    timezone: 'America/Chicago',
    is_tonight: true,
    show_count: 1,
    has_more: false,
    shows: [makeAlsoTonightShow()],
    ...overrides,
  }
}

export function makeRailVenue(
  overrides: Partial<VenueResponse> = {}
): VenueResponse {
  return {
    id: 10,
    slug: 'salt-shed',
    name: 'Salt Shed',
    city: 'Chicago',
    state: 'IL',
    timezone: 'America/Chicago',
    verified: true,
    ...overrides,
  }
}

/** Sold out by default, so the status treatment is exercised without setup. */
export function makeVenueShow(overrides: Partial<VenueShow> = {}): VenueShow {
  return {
    id: 200,
    slug: 'waxahatchee',
    title: 'Waxahatchee',
    event_date: CHICAGO_8PM_AUG_15,
    city: 'Chicago',
    state: 'IL',
    price: null,
    age_requirement: null,
    is_cancelled: false,
    is_sold_out: true,
    artists: [
      {
        id: 1,
        name: 'Waxahatchee',
        slug: 'waxahatchee',
      } as VenueShow['artists'][number],
    ],
    ...overrides,
  }
}

export function makeVenueShowsResponse(
  shows: VenueShow[],
  total = shows.length
): VenueShowsResponse {
  return { shows, venue_id: 10, total, limit: 4, offset: 0, year: 0 }
}

export function makeRailShow(
  overrides: Partial<ShowResponse> = {}
): ShowResponse {
  return {
    id: 1,
    slug: 'modest-mouse-califone',
    title: 'Modest Mouse',
    event_date: CHICAGO_8PM_AUG_12,
    status: 'approved',
    is_sold_out: false,
    is_cancelled: false,
    venues: [makeRailVenue()],
    artists: [],
    created_at: '2026-07-12T12:00:00Z',
    updated_at: '2026-07-12T12:00:00Z',
    ...overrides,
  }
}
