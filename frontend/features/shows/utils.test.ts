import { describe, it, expect } from 'vitest'
import {
  splitBill,
  dedupArtistShows,
  dedupVenueShows,
  formatShowCountLabel,
  showTimingInput,
} from './utils'
import type { ShowResponse, VenueResponse } from './types'

// =============================================================================
// PSY-559: dedup helpers
// =============================================================================

describe('dedupArtistShows', () => {
  it('collapses two shows sharing (venue.id, event_date) to lowest id', () => {
    const eventDate = '2026-09-16T02:30:00Z'
    const shows = [
      {
        id: 64,
        event_date: eventDate,
        venue: { id: 1 },
        artists: [{ id: 10, is_headliner: true }],
      },
      {
        id: 786,
        event_date: eventDate,
        venue: { id: 1 },
        artists: [{ id: 10, is_headliner: true }],
      },
    ]
    const result = dedupArtistShows(shows)
    expect(result.map(s => s.id)).toEqual([64])
  })

  it('preserves matinee + evening at the same venue on the same day', () => {
    const matinee = '2026-05-17T20:00:00Z' // 1pm AZ
    const evening = '2026-05-18T03:00:00Z' // 8pm AZ
    const shows = [
      {
        id: 100,
        event_date: matinee,
        venue: { id: 7 },
        artists: [{ id: 22, is_headliner: true }],
      },
      {
        id: 101,
        event_date: evening,
        venue: { id: 7 },
        artists: [{ id: 22, is_headliner: true }],
      },
    ]
    const result = dedupArtistShows(shows)
    expect(result.map(s => s.id)).toEqual([100, 101])
  })

  it('does not collapse same artist on the same date at different venues', () => {
    const eventDate = '2026-04-11T02:30:00Z'
    const shows = [
      {
        id: 1,
        event_date: eventDate,
        venue: { id: 1 },
        artists: [{ id: 99, is_headliner: true }],
      },
      {
        id: 2,
        event_date: eventDate,
        venue: { id: 2 },
        artists: [{ id: 99, is_headliner: true }],
      },
    ]
    expect(dedupArtistShows(shows)).toHaveLength(2)
  })

  it('preserves API ordering when no duplicates', () => {
    const shows = [
      {
        id: 5,
        event_date: '2026-01-01T00:00:00Z',
        venue: { id: 1 },
        artists: [{ id: 1, is_headliner: true }],
      },
      {
        id: 3,
        event_date: '2026-02-01T00:00:00Z',
        venue: { id: 1 },
        artists: [{ id: 1, is_headliner: true }],
      },
    ]
    expect(dedupArtistShows(shows).map(s => s.id)).toEqual([5, 3])
  })

  it('treats missing venue as venue id 0 (still dedupes by event_date)', () => {
    const eventDate = '2026-06-01T00:00:00Z'
    const shows = [
      { id: 1, event_date: eventDate, artists: [{ id: 1, is_headliner: true }] },
      { id: 2, event_date: eventDate, artists: [{ id: 1, is_headliner: true }] },
    ]
    expect(dedupArtistShows(shows).map(s => s.id)).toEqual([1])
  })
})

describe('dedupVenueShows', () => {
  it('collapses two shows sharing (headliner_artist_id, event_date) to lowest id', () => {
    const eventDate = '2026-09-16T02:30:00Z'
    const shows = [
      {
        id: 64,
        event_date: eventDate,
        artists: [
          { id: 10, is_headliner: true, position: 0, set_type: 'headliner' },
        ],
      },
      {
        id: 786,
        event_date: eventDate,
        artists: [
          { id: 10, is_headliner: true, position: 0, set_type: 'headliner' },
        ],
      },
    ]
    expect(dedupVenueShows(shows).map(s => s.id)).toEqual([64])
  })

  it('preserves matinee + evening when artist is the same', () => {
    const matinee = '2026-05-17T20:00:00Z'
    const evening = '2026-05-18T03:00:00Z'
    const artists = [
      { id: 22, is_headliner: true, position: 0, set_type: 'headliner' },
    ]
    const shows = [
      { id: 100, event_date: matinee, artists },
      { id: 101, event_date: evening, artists },
    ]
    expect(dedupVenueShows(shows)).toHaveLength(2)
  })

  it('does not collapse different headliners on the same date', () => {
    const eventDate = '2026-06-01T00:00:00Z'
    const shows = [
      {
        id: 1,
        event_date: eventDate,
        artists: [
          { id: 1, is_headliner: true, position: 0, set_type: 'headliner' },
        ],
      },
      {
        id: 2,
        event_date: eventDate,
        artists: [
          { id: 2, is_headliner: true, position: 0, set_type: 'headliner' },
        ],
      },
    ]
    expect(dedupVenueShows(shows)).toHaveLength(2)
  })

  it('falls back to position 0 when set_type is unset', () => {
    const eventDate = '2026-06-01T00:00:00Z'
    const shows = [
      {
        id: 1,
        event_date: eventDate,
        artists: [{ id: 5, position: 0, set_type: 'performer' }],
      },
      {
        id: 2,
        event_date: eventDate,
        artists: [{ id: 5, position: 0, set_type: 'performer' }],
      },
    ]
    expect(dedupVenueShows(shows).map(s => s.id)).toEqual([1])
  })
})

describe('formatShowCountLabel', () => {
  it('uses the simple form when total equals loaded', () => {
    expect(formatShowCountLabel(12, 12)).toBe('12 shows')
  })

  it('uses singular when one show and complete', () => {
    expect(formatShowCountLabel(1, 1)).toBe('1 show')
  })

  it('shows loaded of total when truncated', () => {
    expect(formatShowCountLabel(150, 1088)).toBe('150 of 1,088 shows')
  })

  it('falls back to loaded-only when total is missing', () => {
    expect(formatShowCountLabel(50)).toBe('50 shows')
    expect(formatShowCountLabel(50, null)).toBe('50 shows')
  })
})

// =============================================================================
// showTimingInput: which calendar a show is judged on
// =============================================================================

describe('showTimingInput', () => {
  function makeShow(overrides: Partial<ShowResponse> = {}): ShowResponse {
    return {
      id: 1,
      slug: 'a-show',
      title: 'A Show',
      event_date: '2026-04-16T03:00:00Z',
      status: 'approved',
      state: 'AZ',
      venues: [],
      artists: [],
      created_at: '2024-01-01T00:00:00Z',
      updated_at: '2024-01-01T00:00:00Z',
      is_sold_out: false,
      is_cancelled: false,
      ...overrides,
    }
  }

  function makeVenue(overrides: Partial<VenueResponse> = {}): VenueResponse {
    return {
      id: 1,
      slug: 'the-venue',
      name: 'The Venue',
      city: 'Chicago',
      state: 'IL',
      verified: true,
      ...overrides,
    }
  }

  it('takes the zone from the venue the show happens at', () => {
    expect(
      showTimingInput(
        makeShow({
          venues: [makeVenue({ timezone: 'America/Chicago' })],
        })
      )
    ).toEqual({
      eventDate: '2026-04-16T03:00:00Z',
      state: 'IL',
      timezone: 'America/Chicago',
    })
  })

  // The show row's `state` is denormalized and can lag a venue edit, so it is
  // the fallback rather than the source.
  it('prefers the venue state over the show row when they disagree', () => {
    expect(
      showTimingInput(
        makeShow({ state: 'AZ', venues: [makeVenue({ state: 'IL' })] })
      ).state
    ).toBe('IL')
  })

  it('falls back to the show state when there is no venue', () => {
    expect(showTimingInput(makeShow())).toEqual({
      eventDate: '2026-04-16T03:00:00Z',
      state: 'AZ',
      timezone: undefined,
    })
  })
})

describe('splitBill', () => {
  const act = (
    name: string,
    overrides: { set_type?: string; is_headliner?: boolean | null } = {}
  ) => ({ name, set_type: 'performer', ...overrides })

  it('leads with the curated set_type', () => {
    const { headliners, support } = splitBill([
      act('Opener', { set_type: 'opener' }),
      act('Top', { set_type: 'headliner' }),
    ])
    expect(headliners.map(a => a.name)).toEqual(['Top'])
    expect(support.map(a => a.name)).toEqual(['Opener'])
  })

  it('honours the older is_headliner flag on shows written before the roles', () => {
    const { headliners, support } = splitBill([
      act('Support', { is_headliner: false }),
      act('Lead', { is_headliner: true }),
    ])
    expect(headliners.map(a => a.name)).toEqual(['Lead'])
    expect(support.map(a => a.name)).toEqual(['Support'])
  })

  it('keeps every co-headliner, in listed order', () => {
    const { headliners, support } = splitBill([
      act('A', { set_type: 'headliner' }),
      act('B', { is_headliner: true }),
      act('C', { set_type: 'opener' }),
    ])
    expect(headliners.map(a => a.name)).toEqual(['A', 'B'])
    expect(support.map(a => a.name)).toEqual(['C'])
  })

  it('reads an unclaimed bill in listed order, first act leading', () => {
    const { headliners, support } = splitBill([act('First'), act('Second')])
    expect(headliners.map(a => a.name)).toEqual(['First'])
    expect(support.map(a => a.name)).toEqual(['Second'])
  })

  it('returns nothing for a show with no bill at all', () => {
    expect(splitBill([])).toEqual({ headliners: [], support: [] })
  })
})
