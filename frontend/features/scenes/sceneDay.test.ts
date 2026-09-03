import { describe, it, expect } from 'vitest'
import { looksLikeISOWeek } from './sceneWeek'
import {
  dayShows,
  formatDayChip,
  formatDayCountLine,
  formatDayFull,
  formatPointerDay,
  formatShowStartTime,
  formatShowStartTimeCompact,
  looksLikeCalendarDate,
  orderNightShows,
  type SceneDayResponse,
  type SceneDayShow,
} from './sceneDay'

const show = (over: Partial<SceneDayShow> = {}): SceneDayShow =>
  ({
    id: 1,
    title: '',
    event_date: '2026-07-31',
    // 20:00 Phoenix on the 31st. UTC midnight on the 31st is the 30th in
    // Arizona, so anything that renders this without a zone gets it wrong.
    starts_at: '2026-08-01T03:00:00Z',
    is_sold_out: false,
    is_cancelled: false,
    venue_name: 'Valley Bar',
    venue_state: 'AZ',
    venue_timezone: 'America/Phoenix',
    ...over,
  }) as SceneDayShow

describe('looksLikeCalendarDate', () => {
  it('accepts a padded ISO date in range', () => {
    expect(looksLikeCalendarDate('2026-07-31')).toBe(true)
  })

  it.each([
    ['2026-7-31', 'unpadded month'],
    ['26-07-31', 'two-digit year'],
    ['2026-W31', 'an ISO week key'],
    ['2026-07-31T00:00:00Z', 'an instant'],
    ['tonight', 'a word'],
    [' 2026-07-31', 'a leading space'],
    ['', 'empty'],
  ])('rejects %s (%s)', segment => {
    expect(looksLikeCalendarDate(segment)).toBe(false)
  })

  // The segment is dynamic, so an unbounded key space is an unbounded set of
  // distinct URLs. Same bound, same reason, as the week key.
  it('rejects years outside the tracked window', () => {
    expect(looksLikeCalendarDate('1998-07-31')).toBe(false)
    expect(looksLikeCalendarDate('2400-07-31')).toBe(false)
  })

  // Shape only — the backend decides whether the date is real, and a February
  // 30th that reaches it comes back 404.
  it('does not judge whether the date exists', () => {
    expect(looksLikeCalendarDate('2026-02-30')).toBe(true)
  })

  /**
   * `/scenes/{slug}/{period}` dispatches on these two predicates alone: Next
   * allows one dynamic segment per level, so the week view and the day view
   * share a route and this is what tells them apart. If both ever matched the
   * same key, whichever branch is written first would silently swallow the
   * other's URLs — a week key rendering the day view, or vice versa, with the
   * typecheck and every other test still green.
   */
  it('can never both match — the shared route dispatches on this', () => {
    const keys = [
      '2026-W31',
      '2026-W01',
      '2026-07-31',
      '2026-02-30',
      '2015-01-01',
      'garbage',
      'tonight',
      'week',
    ]
    for (const key of keys) {
      expect(
        looksLikeISOWeek(key) && looksLikeCalendarDate(key),
        `${key} matched BOTH period shapes`
      ).toBe(false)
    }
    // And each shape claims the keys it should.
    expect(looksLikeISOWeek('2026-W31')).toBe(true)
    expect(looksLikeCalendarDate('2026-07-31')).toBe(true)
  })
})

describe('date formatting', () => {
  // The killer bug this guards: `new Date('2026-07-31')` is UTC midnight, which
  // renders as July 30 in every US zone.
  it('formats the full date from a calendar date, not a UTC instant', () => {
    expect(formatDayFull('2026-07-31')).toBe('Friday, July 31, 2026')
  })

  it('formats the adjacent-day chips', () => {
    expect(formatDayChip('2026-07-30')).toBe('Thu Jul 30')
    expect(formatDayChip('2026-08-01')).toBe('Sat Aug 1')
  })
})

describe('formatPointerDay', () => {
  it('names a nearby night by weekday alone, the way anyone would say it', () => {
    expect(formatPointerDay('2026-07-30', '2026-07-31', true)).toBe('Friday')
    expect(formatPointerDay('2026-07-30', '2026-08-05', true)).toBe('Wednesday')
  })

  // Past a week, a bare weekday names a day the reader cannot identify.
  it('adds the date once a weekday would be ambiguous', () => {
    expect(formatPointerDay('2026-07-30', '2026-08-06', true)).toBe('Thu, Aug 6')
    expect(formatPointerDay('2026-07-30', '2026-09-04', true)).toBe('Fri, Sep 4')
  })

  // "Friday" means THIS Friday to a reader, whatever date the page is about. A
  // visitor landing on a 2020 permalink from search must not be told the next
  // show is "Friday".
  it('always spells out the date away from the live night', () => {
    expect(formatPointerDay('2020-01-15', '2020-01-17', false)).toBe('Fri, Jan 17')
    expect(formatPointerDay('2026-07-30', '2026-07-31', false)).toBe('Fri, Jul 31')
  })

  // And a bare month/day is just as relative as a bare weekday: "Sat, Aug 8" on
  // a page headed January 2020 reads as August 2020. This is the shape a future
  // dated permalink actually produces, so it is the shape that must be pinned.
  it('carries the year whenever it differs from the page it sits on', () => {
    expect(formatPointerDay('2020-01-15', '2026-08-08', false)).toBe('Sat, Aug 8, 2026')
    expect(formatPointerDay('2026-12-30', '2027-01-02', true)).toBe('Saturday')
    expect(formatPointerDay('2026-12-30', '2027-03-05', false)).toBe('Fri, Mar 5, 2027')
  })
})

describe('formatShowStartTime', () => {
  it('renders the instant in the venue zone', () => {
    expect(formatShowStartTime(show())).toBe('8:00 PM')
  })

  // The backend bucketed the show into this day using the SCENE's zone, so a
  // venue with no zone of its own must be rendered against that, not the
  // viewer's and not a different city's.
  it('falls back to the scene zone when the venue has none', () => {
    expect(
      formatShowStartTime(show({ venue_timezone: '', venue_state: '' }), 'America/Phoenix')
    ).toBe('8:00 PM')
  })

  // Chicago is the check that the SCENE zone is actually reaching the
  // formatter: the state fallback inside the shared helper defaults to Arizona,
  // so a scene zone that was being dropped would still say 8:00 PM above.
  it('uses the scene zone, not the helper default, when neither is on the show', () => {
    expect(
      formatShowStartTime(show({ venue_timezone: '', venue_state: '' }), 'America/Chicago')
    ).toBe('10:00 PM')
  })

  // A type is not a runtime guarantee: the backend can deploy ahead of the
  // frontend, and Intl THROWS on an invalid date — from a server component,
  // which would 500 the whole page.
  it.each([undefined, 'not-a-date'])('returns null for an unusable instant (%s)', raw => {
    expect(formatShowStartTime(show({ starts_at: raw as never }))).toBeNull()
  })

  // The scene's zone is what normally rescues a zone-less row here, so the
  // refusal only bites when the SCENE has none either: a day page assembled
  // before any of its venues were geocoded.
  it('returns null when neither the row nor the scene supplies a zone', () => {
    expect(
      formatShowStartTime(show({ venue_timezone: '', venue_state: '' }))
    ).toBeNull()
  })

  it('returns null for a venue state outside the US map with no scene zone', () => {
    expect(
      formatShowStartTime(
        show({ venue_timezone: '', venue_state: 'England' })
      )
    ).toBeNull()
  })

  // The day payload now sends null rather than its own fallback literal for a
  // scene whose zone it cannot name, which is the case this refusal could not
  // previously reach.
  it('returns null on an explicitly null scene zone', () => {
    expect(
      formatShowStartTime(show({ venue_timezone: '', venue_state: '' }), null)
    ).toBeNull()
    expect(
      formatShowStartTimeCompact(
        show({ venue_timezone: '', venue_state: '' }),
        null
      )
    ).toBeNull()
  })
})


describe('formatDayCountLine', () => {
  it('counts and pluralises listed shows', () => {
    expect(formatDayCountLine(4)).toBe('4 shows')
    expect(formatDayCountLine(1)).toBe('1 show')
  })

  // "0 shows" would assert nothing is happening in the city. We only know our
  // own calendar, and the page must not claim more than that.
  it('says LISTED when there are none, never that there are none', () => {
    expect(formatDayCountLine(0)).toBe('0 shows listed')
  })
})

describe('dayShows', () => {
  const day = (over: Partial<SceneDayResponse> = {}): SceneDayResponse =>
    ({ date: '2026-07-31', shows: [show()], show_count: 1, ...over }) as SceneDayResponse

  // The generator types `shows` nullable even though the API always emits an
  // array; a null must not take the page down.
  it('survives a null list', () => {
    expect(dayShows(day({ shows: null }))).toEqual([])
  })

  // The rows are the count. `show_count` is `len(shows)` computed by the same
  // handler that serialized the slice, so a header sourced from it could only
  // ever agree — or state a number the page cannot show.
  it('ignores a show_count that disagrees with the rows', () => {
    expect(dayShows(day({ show_count: 99 }))).toHaveLength(1)
  })
})

describe('formatShowStartTimeCompact', () => {
  it('renders the same instant in the compact register', () => {
    // 03:00Z is 20:00 in Phoenix; the full register says 8:00 PM for the row.
    expect(formatShowStartTimeCompact(show())).toBe('8PM')
    expect(formatShowStartTime(show())).toBe('8:00 PM')
  })

  it('keeps the minutes off the hour', () => {
    expect(
      formatShowStartTimeCompact(show({ starts_at: '2026-08-01T02:30:00Z' }))
    ).toBe('7:30PM')
  })

  it('falls back to the scene clock exactly as its sibling does', () => {
    const roomless = show({ venue_timezone: undefined, venue_state: undefined })
    expect(formatShowStartTimeCompact(roomless, 'America/New_York')).toBe('11PM')
  })

  it('returns null when the payload has no usable instant', () => {
    expect(formatShowStartTimeCompact(show({ starts_at: 'not-a-date' }))).toBeNull()
  })
})

describe('orderNightShows', () => {
  // 20:00, 21:00 and 22:00 Phoenix on the same night.
  const doors8 = show({ id: 8, starts_at: '2026-08-01T03:00:00Z' })
  const doors9 = show({ id: 9, starts_at: '2026-08-01T04:00:00Z' })
  const doors10 = show({ id: 10, starts_at: '2026-08-01T05:00:00Z' })
  const night = [doors8, doors9, doors10]
  const ids = (rows: SceneDayShow[]) => rows.map(row => row.id)

  it('sinks started shows below the ones still to come, on the live night', () => {
    // 21:30 Phoenix: the 20:00 and 21:00 sets are on, the 22:00 is not.
    const at2130 = new Date('2026-08-01T04:30:00Z')
    expect(ids(orderNightShows(night, true, at2130))).toEqual([10, 8, 9])
  })

  it('keeps started shows in the order they began', () => {
    const at2230 = new Date('2026-08-01T05:30:00Z')
    expect(ids(orderNightShows(night, true, at2230))).toEqual([8, 9, 10])
  })

  it('sinks a show the INSTANT it starts, and not before', () => {
    // The boundary both ways, against the same predicate the offer gate uses:
    // at the start instant the show has begun; a millisecond earlier it has not.
    const atDoors = new Date('2026-08-01T04:00:00Z')
    expect(ids(orderNightShows(night, true, atDoors))).toEqual([10, 8, 9])

    const oneMsBefore = new Date('2026-08-01T03:59:59.999Z')
    expect(ids(orderNightShows(night, true, oneMsBefore))).toEqual([9, 10, 8])
  })

  it('leaves an archive or future night in clock order', () => {
    const wellAfter = new Date('2026-08-02T03:00:00Z')
    expect(orderNightShows(night, false, wellAfter)).toBe(night)
  })

  it('returns the input untouched when nothing has started yet', () => {
    const beforeDoors = new Date('2026-08-01T02:00:00Z')
    expect(orderNightShows(night, true, beforeDoors)).toBe(night)
  })

  it('sinks a row whose instant cannot be read', () => {
    // `hasShowStarted` counts an unreadable date as started, so such a row must
    // not head a list of shows a reader can still get to.
    const undated = show({ id: 99, starts_at: 'not-a-date' })
    const beforeDoors = new Date('2026-08-01T02:00:00Z')
    expect(ids(orderNightShows([undated, doors8], true, beforeDoors))).toEqual([
      8, 99,
    ])
  })
})
