import { describe, it, expect } from 'vitest'
import {
  dayAnchorId,
  dayGroupHeading,
  groupShowsByVenueLocalDay,
} from './dayGroups'
import type { ShowResponse } from './types'

function makeShow(
  id: number,
  eventDate: string,
  venueTimezone?: string,
  state = 'AZ'
): ShowResponse {
  return {
    id,
    slug: `show-${id}`,
    title: `Show ${id}`,
    event_date: eventDate,
    status: 'approved',
    city: 'Phoenix',
    state,
    venues: venueTimezone
      ? [{ id, name: 'Room', timezone: venueTimezone } as never]
      : [],
    artists: [],
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    is_sold_out: false,
    is_cancelled: false,
  } as ShowResponse
}

describe('groupShowsByVenueLocalDay', () => {
  it('groups consecutive rows that share a venue-local date', () => {
    const groups = groupShowsByVenueLocalDay(
      [
        makeShow(1, '2026-09-12T02:00:00Z', 'America/Phoenix'),
        makeShow(2, '2026-09-12T03:00:00Z', 'America/Phoenix'),
        makeShow(3, '2026-09-13T02:00:00Z', 'America/Phoenix'),
      ],
      null
    )

    expect(groups.map(group => group.dateKey)).toEqual([
      '2026-09-11',
      '2026-09-12',
    ])
    expect(groups[0].rows.map(row => row.id)).toEqual([1, 2])
    expect(groups[1].rows.map(row => row.id)).toEqual([3])
  })

  // The whole point of grouping venue-locally: an All Cities list has to place
  // each show on the day it happens where it happens, not on the reader's day.
  it('reads the date in each row s own venue zone, not one shared zone', () => {
    // The list sorts on the absolute instant, and these two are in that order.
    // Read venue-locally they invert: 01:00 in New York is already the 12th
    // while 23:00 in Los Angeles, an hour LATER in absolute terms, is still the
    // 11th. A single shared zone would put both on one date.
    const groups = groupShowsByVenueLocalDay(
      [
        makeShow(1, '2026-09-12T05:00:00Z', 'America/New_York', 'NY'),
        makeShow(2, '2026-09-12T06:00:00Z', 'America/Los_Angeles', 'CA'),
      ],
      null
    )

    expect(groups.map(group => group.dateKey)).toEqual([
      '2026-09-12',
      '2026-09-11',
    ])
  })

  it('preserves list order rather than merging rows across a gap', () => {
    // The list sorts on the absolute instant while a date is venue-local, so
    // two rows on one local date can arrive either side of a row on another.
    // Collecting by date would silently reorder the list.
    const groups = groupShowsByVenueLocalDay(
      [
        makeShow(1, '2026-09-12T02:00:00Z', 'America/Phoenix'),
        makeShow(2, '2026-09-13T02:00:00Z', 'America/Phoenix'),
        makeShow(3, '2026-09-12T04:00:00Z', 'America/Phoenix'),
      ],
      null
    )

    expect(groups.map(group => group.rows.map(row => row.id))).toEqual([
      [1],
      [2],
      [3],
    ])
  })

  // An id has to be unique in a document, and the first occurrence is where a
  // `#d-2026-09-11` link should land.
  it('anchors only the first group of a repeated date', () => {
    const groups = groupShowsByVenueLocalDay(
      [
        makeShow(1, '2026-09-12T02:00:00Z', 'America/Phoenix'),
        makeShow(2, '2026-09-13T02:00:00Z', 'America/Phoenix'),
        makeShow(3, '2026-09-12T04:00:00Z', 'America/Phoenix'),
      ],
      null
    )

    expect(groups[0].anchorId).toBe('d-2026-09-11')
    expect(groups[1].anchorId).toBe('d-2026-09-12')
    expect(groups[2].anchorId).toBeNull()
  })

  it('drops no rows', () => {
    const shows = [
      makeShow(1, '2026-09-12T02:00:00Z', 'America/Phoenix'),
      makeShow(2, 'not-a-date', 'America/Phoenix'),
      makeShow(3, '2026-09-13T02:00:00Z', 'America/Phoenix'),
    ]

    const grouped = groupShowsByVenueLocalDay(shows, null).flatMap(
      group => group.rows
    )
    expect(grouped.map(row => row.id)).toEqual([1, 2, 3])
  })

  describe('the tonight test', () => {
    it('marks the group whose date is today where its shows happen', () => {
      const now = new Date('2026-09-12T19:00:00Z') // 12:00 in Phoenix
      const groups = groupShowsByVenueLocalDay(
        [
          makeShow(1, '2026-09-13T02:00:00Z', 'America/Phoenix'), // Sep 12 local
          makeShow(2, '2026-09-14T02:00:00Z', 'America/Phoenix'), // Sep 13 local
        ],
        now
      )

      expect(groups[0].isToday).toBe(true)
      expect(groups[1].isToday).toBe(false)
    })

    // TONIGHT is a same-day claim, and a venue with no zone of its own outside
    // the US state map is dated on a FALLBACK zone that can be a calendar day
    // out. The date still prints; the word does not.
    it('makes no TONIGHT claim for a venue whose zone is only guessed', () => {
      const now = new Date('2026-09-12T19:00:00Z')
      const guessed = makeShow(1, '2026-09-13T02:00:00Z', undefined, 'XX')

      const groups = groupShowsByVenueLocalDay([guessed], now)

      expect(groups[0].isToday).toBe(false)
      // The heading is still rendered, on the guessed zone.
      expect(groups[0].dateKey).not.toBeNull()
    })

    it('still marks today when the venue carries its own zone', () => {
      const now = new Date('2026-09-12T19:00:00Z')
      const resolved = makeShow(1, '2026-09-13T02:00:00Z', 'America/Phoenix', 'XX')

      expect(groupShowsByVenueLocalDay([resolved], now)[0].isToday).toBe(true)
    })

    // The server render passes no clock: reading one in a prerenderable scope
    // would move the whole list into the dynamic resume.
    it('marks nothing when no clock is supplied', () => {
      const groups = groupShowsByVenueLocalDay(
        [makeShow(1, '2026-09-13T02:00:00Z', 'America/Phoenix')],
        null
      )

      expect(groups[0].isToday).toBe(false)
    })
  })
})

describe('dayGroupHeading', () => {
  const base = {
    dateKey: '2026-09-11',
    anchorId: 'd-2026-09-11',
    dayOfWeek: 'FRI',
    monthDay: 'SEP 11',
    rows: [],
  }

  it('prints the weekday and the date', () => {
    expect(dayGroupHeading({ ...base, isToday: false })).toBe('FRI · SEP 11')
  })

  it('leads with TONIGHT on today s group', () => {
    expect(
      dayGroupHeading({
        ...base,
        dayOfWeek: 'SAT',
        monthDay: 'SEP 12',
        isToday: true,
      })
    ).toBe('TONIGHT · SAT SEP 12')
  })
})

describe('dayAnchorId', () => {
  it('addresses a day by its venue-local date', () => {
    expect(dayAnchorId('2026-09-12')).toBe('d-2026-09-12')
  })
})
