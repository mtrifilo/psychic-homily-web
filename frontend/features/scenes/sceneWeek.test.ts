import { describe, it, expect } from 'vitest'
import {
  countShows,
  currentWeekBounds,
  formatDayHeading,
  formatShowCountLine,
  formatWeekRange,
  formatWeekRangeCompact,
  resolveRequestedWeek,
  looksLikeISOWeek,
  showDisplayTitle,
  showHref,
  type SceneWeekResponse,
  type SceneWeekShow,
} from './sceneWeek'

const show = (over: Partial<SceneWeekShow> = {}): SceneWeekShow => ({
  id: 1,
  title: '',
  event_date: '2026-07-27',
  starts_at: '2026-07-28T03:00:00Z',
  is_sold_out: false,
  is_cancelled: false,
  ...over,
})

describe('formatDayHeading', () => {
  // The load-bearing case. `new Date('2026-07-27')` parses as UTC midnight,
  // which renders as Jul 26 in any negative-offset timezone — so a US reader
  // would see every day of the week shifted back by one. These are calendar
  // dates the backend already resolved in the scene's timezone, not instants.
  it('renders the calendar date, not a timezone-shifted one', () => {
    expect(formatDayHeading('2026-07-27')).toBe('MON JUL 27')
    expect(formatDayHeading('2026-08-02')).toBe('SUN AUG 2')
    expect(formatDayHeading('2026-01-01')).toBe('THU JAN 1')
  })

  it('does not drift across a month boundary', () => {
    expect(formatDayHeading('2026-08-01')).toBe('SAT AUG 1')
    expect(formatDayHeading('2026-07-31')).toBe('FRI JUL 31')
  })
})

describe('currentWeekBounds', () => {
  // 2026-08-02T05:00Z is Sunday in UTC but still SATURDAY the 1st in Phoenix
  // (UTC-7). The two zones therefore disagree about which day it is, and on
  // this instant they still agree about the WEEK — the interesting case is the
  // one below, where they do not.
  it('reads the calendar date in the zone it is given', () => {
    const instant = new Date('2026-08-02T05:00:00Z')

    expect(currentWeekBounds(instant, 'UTC')).toEqual({
      start: '2026-07-27',
      end: '2026-08-02',
    })
    expect(currentWeekBounds(instant, 'America/Phoenix')).toEqual({
      start: '2026-07-27',
      end: '2026-08-02',
    })
  })

  // Monday 00:30 UTC is still Sunday evening on the US west coast, so the two
  // zones are in different weeks. Deriving the week without naming a zone would
  // pick one of these by accident.
  it('puts zones on opposite sides of the Monday boundary in different weeks', () => {
    const instant = new Date('2026-08-03T00:30:00Z')

    expect(currentWeekBounds(instant, 'UTC')).toEqual({
      start: '2026-08-03',
      end: '2026-08-09',
    })
    expect(currentWeekBounds(instant, 'America/Los_Angeles')).toEqual({
      start: '2026-07-27',
      end: '2026-08-02',
    })
  })

  it('treats Monday as the first day and Sunday as the last', () => {
    expect(currentWeekBounds(new Date('2026-07-27T12:00:00Z'), 'UTC')).toEqual({
      start: '2026-07-27',
      end: '2026-08-02',
    })
    expect(currentWeekBounds(new Date('2026-08-02T12:00:00Z'), 'UTC')).toEqual({
      start: '2026-07-27',
      end: '2026-08-02',
    })
  })

  it('does not drift across a year boundary', () => {
    expect(currentWeekBounds(new Date('2027-01-01T12:00:00Z'), 'UTC')).toEqual({
      start: '2026-12-28',
      end: '2027-01-03',
    })
  })
})

describe('formatWeekRange', () => {
  it('spans a month boundary with the end year', () => {
    expect(formatWeekRange('2026-07-27', '2026-08-02')).toBe(
      'Mon, Jul 27 – Sun, Aug 2, 2026'
    )
  })

  // ISO week 1 of 2026 starts 2025-12-29 — in the PREVIOUS calendar year. The
  // range must show the year the week ENDS in.
  it('handles a week that starts in the previous calendar year', () => {
    expect(formatWeekRange('2025-12-29', '2026-01-04')).toBe(
      'Mon, Dec 29 – Sun, Jan 4, 2026'
    )
  })
})

describe('formatWeekRangeCompact', () => {
  it('drops the weekday and year the share card has no room for', () => {
    expect(formatWeekRangeCompact('2026-07-27', '2026-08-02')).toBe('JUL 27 – AUG 2')
  })

  // The separator is an EN DASH. A hyphen would be an invisible regression:
  // it renders, it just looks wrong on a card nobody re-reads.
  it('separates the two dates with an en dash', () => {
    expect(formatWeekRangeCompact('2026-07-27', '2026-08-02')).toContain(' – ')
    expect(formatWeekRangeCompact('2026-07-27', '2026-08-02')).not.toContain('-')
  })

  // Same calendar-date hazard as its siblings: parsed component-wise, so a
  // negative-offset timezone must not shift either end back a day.
  it('renders the calendar dates, not timezone-shifted ones', () => {
    expect(formatWeekRangeCompact('2025-12-29', '2026-01-04')).toBe('DEC 29 – JAN 4')
  })
})

describe('resolveRequestedWeek', () => {
  it('passes an absent segment through as "the current week"', () => {
    expect(resolveRequestedWeek(undefined)).toBeUndefined()
  })

  it('returns a well-formed key exactly as it was validated', () => {
    expect(resolveRequestedWeek('2026-W31')).toBe('2026-W31')
    expect(resolveRequestedWeek('2026-w31')).toBe('2026-w31')
  })

  // The distinction that matters: junk must be DISTINGUISHABLE from "no segment
  // supplied". Collapsing the two would serve the current week's card for a URL
  // whose page 404s — a confident answer to a question nobody asked.
  it('rejects junk rather than falling back to the current week', () => {
    for (const junk of ['garbage', '2026-31', 'W31', '', 'opengraph-image']) {
      expect(resolveRequestedWeek(junk)).toBeNull()
    }
    expect(resolveRequestedWeek('garbage')).not.toBeUndefined()
  })

  // Each accepted spelling of the same week is another distinct URL that
  // renders and caches its own card, so whitespace variants are rejected rather
  // than normalized — and the proxy, which guards the page, rejects them too.
  it('rejects whitespace-padded spellings instead of normalizing them', () => {
    for (const padded of [' 2026-W31', '2026-W31 ', '\t2026-W31', ' 2026-W31']) {
      expect(resolveRequestedWeek(padded)).toBeNull()
    }
  })

  // An unbounded year range is an unbounded set of renderable URLs.
  it('rejects years outside the range the site could possibly cover', () => {
    expect(resolveRequestedWeek('1998-W07')).toBeNull()
    expect(resolveRequestedWeek('9999-W01')).toBeNull()
    const nextYear = new Date().getUTCFullYear() + 1
    expect(resolveRequestedWeek(`${nextYear}-W01`)).toBe(`${nextYear}-W01`)
    expect(resolveRequestedWeek(`${nextYear + 1}-W01`)).toBeNull()
  })
})

describe('formatShowCountLine', () => {
  it('says "this week" only for the current week', () => {
    expect(formatShowCountLine(32, true)).toBe('32 shows this week')
    expect(formatShowCountLine(32, false)).toBe('32 shows')
  })

  // A card for a week that ended must not claim to be current — it is the one
  // thing separating a truthful archived card from a lying one.
  it('never claims an archived week is current', () => {
    expect(formatShowCountLine(24, false)).not.toContain('this week')
    expect(formatShowCountLine(0, false)).not.toContain('this week')
  })

  it('says a quiet week in words rather than posting a zero', () => {
    expect(formatShowCountLine(0, true)).toBe('No shows this week')
    expect(formatShowCountLine(0, false)).toBe('No shows')
  })

  it('uses the singular for one show', () => {
    expect(formatShowCountLine(1, true)).toBe('1 show this week')
  })
})

describe('looksLikeISOWeek', () => {
  it('accepts well-formed keys regardless of case', () => {
    expect(looksLikeISOWeek('2026-W31')).toBe(true)
    expect(looksLikeISOWeek('2026-w31')).toBe(true)
    // Shape only — 2025 has 52 weeks, but deciding that is the backend's job.
    expect(looksLikeISOWeek('2025-W53')).toBe(true)
  })

  // Deliberately NOT trimmed. The proxy that guards the page tests the raw
  // segment, so trimming here would let the card accept URLs the page rejects —
  // and every accepted spelling is another distinct, separately-rendered URL.
  it('rejects padded spellings so one week has one URL', () => {
    for (const padded of ['  2026-W31  ', ' 2026-W31', '2026-W31 ', '\t2026-W31']) {
      expect(looksLikeISOWeek(padded)).toBe(false)
    }
  })

  it('rejects anything that is not week-shaped', () => {
    for (const bad of ['week', 'garbage', '2026', '2026-31', '26-W31', '2026-W3', '']) {
      expect(looksLikeISOWeek(bad)).toBe(false)
    }
  })

  it('rejects years the site could not possibly have data for', () => {
    expect(looksLikeISOWeek('1998-W07')).toBe(false)
    expect(looksLikeISOWeek('9999-W01')).toBe(false)
  })
})

describe('showDisplayTitle', () => {
  // Most shows carry an empty title — display names are composed from the bill
  // everywhere else in the app — so artists lead and title is the fallback.
  it('prefers the bill over the title', () => {
    expect(showDisplayTitle(show({ artist_names: ['Ovlov', 'Cusp'], title: 'Ignored' }))).toBe(
      'Ovlov, Cusp'
    )
  })

  it('falls back to the title when there is no bill', () => {
    expect(showDisplayTitle(show({ title: 'Some Festival' }))).toBe('Some Festival')
    expect(showDisplayTitle(show({ artist_names: [], title: 'Some Festival' }))).toBe(
      'Some Festival'
    )
  })

  it('never renders empty', () => {
    expect(showDisplayTitle(show())).toBe('Live music')
    expect(showDisplayTitle(show({ artist_names: null }))).toBe('Live music')
  })
})

describe('showHref', () => {
  it('prefers the slug and falls back to the id', () => {
    expect(showHref(show({ slug: 'ovlov-2026-07-27' }))).toBe('/shows/ovlov-2026-07-27')
    expect(showHref(show({ id: 42, slug: '' }))).toBe('/shows/42')
  })
})

describe('countShows', () => {
  const week = (over: Partial<SceneWeekResponse>): SceneWeekResponse =>
    ({
      slug: 'chicago-il',
      scene_name: 'Chicago, IL',
      city: 'Chicago',
      state: 'IL',
      iso_week: '2026-W31',
      start_date: '2026-07-27',
      end_date: '2026-08-02',
      timezone: 'America/Chicago',
      show_count: 0,
      prev_week: '2026-W30',
      next_week: '2026-W32',
      is_current_week: false,
      is_past_week: true,
      days: [],
      tracked_venues: [],
      ...over,
    }) as SceneWeekResponse

  it('uses the server count', () => {
    expect(countShows(week({ show_count: 32 }))).toBe(32)
  })

  // `days` is typed nullable by the generator even though the API always emits
  // an array; a header disagreeing with the list below it is worse than a
  // recount.
  it('survives a null days array', () => {
    expect(countShows(week({ days: null, show_count: 0 }))).toBe(0)
  })
})
