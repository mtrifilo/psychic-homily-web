import { describe, it, expect } from 'vitest'
import {
  countDayShows,
  formatDayChip,
  formatDayCountLine,
  formatDayFull,
  formatPointerDay,
  formatShowPrice,
  formatShowStartTime,
  looksLikeCalendarDate,
  venueWebsiteHref,
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
    expect(formatPointerDay('2026-07-30', '2026-07-31')).toBe('Friday')
    expect(formatPointerDay('2026-07-30', '2026-08-05')).toBe('Wednesday')
  })

  // Past a week, a bare weekday names a day the reader cannot identify.
  it('adds the date once a weekday would be ambiguous', () => {
    expect(formatPointerDay('2026-07-30', '2026-08-06')).toBe('Thu, Aug 6')
    expect(formatPointerDay('2026-07-30', '2026-09-04')).toBe('Fri, Sep 4')
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
})

describe('formatShowPrice', () => {
  it('uses the site-wide price format', () => {
    expect(formatShowPrice(show({ price: 22 }))).toBe('$22.00')
    expect(formatShowPrice(show({ price: 0 }))).toBe('Free')
  })

  it('is absent when no price is recorded', () => {
    expect(formatShowPrice(show())).toBeNull()
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

describe('countDayShows', () => {
  const day = (over: Partial<SceneDayResponse> = {}): SceneDayResponse =>
    ({ date: '2026-07-31', shows: [show()], show_count: 1, ...over }) as SceneDayResponse

  it('prefers the server count', () => {
    expect(countDayShows(day({ show_count: 4 }))).toBe(4)
  })

  // The generator types `shows` nullable even though the API always emits an
  // array; a null must not take the page down.
  it('falls back to counting, and survives a null list', () => {
    expect(countDayShows(day({ show_count: undefined as never }))).toBe(1)
    expect(countDayShows(day({ show_count: undefined as never, shows: null }))).toBe(0)
  })
})

describe('venueWebsiteHref', () => {
  it('passes through an absolute http(s) URL', () => {
    expect(venueWebsiteHref('https://hotelcongress.com')).toBe('https://hotelcongress.com/')
  })

  // Operator-supplied data reaching an href. A stored `javascript:` value would
  // otherwise be stored XSS on a page that renders one link per room; the room
  // falls back to its page here instead, which is a worse link and never an
  // unsafe one.
  it.each([
    'javascript:alert(1)',
    'data:text/html,<script>alert(1)</script>',
    'hotelcongress.com',
    '',
    undefined,
  ])('refuses %s', raw => {
    expect(venueWebsiteHref(raw as string | undefined)).toBeNull()
  })
})
