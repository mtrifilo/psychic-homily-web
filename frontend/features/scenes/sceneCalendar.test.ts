import { describe, it, expect } from 'vitest'
import {
  SCENE_CALENDAR_ROW_CAP,
  SCENE_CALENDAR_WINDOW_DAYS,
  calendarDateInZone,
  formatCalendarDateHeading,
  formatTimeZoneLabel,
  groupShowsByDate,
  resolveSceneTimeZone,
  sceneStatParts,
  sceneTonightDate,
  showCalendarDate,
  venueSubLocality,
} from './sceneCalendar'
import type { SceneShowSummary } from './types'

function buildShow(overrides: Partial<SceneShowSummary> = {}): SceneShowSummary {
  return {
    id: 1,
    title: '',
    // What the endpoint actually sends: the UTC-rendered date. A 20:00 Phoenix
    // show is already the NEXT day in UTC, which is exactly the trap the date
    // helpers exist to avoid.
    event_date: '2026-08-09',
    starts_at: '2026-08-09T03:00:00Z',
    is_cancelled: false,
    is_sold_out: false,
    venue_timezone: 'America/Phoenix',
    venue_state: 'AZ',
    venue_city: 'Phoenix',
    venue_name: 'Valley Bar',
    ...overrides,
  }
}

describe('window constants', () => {
  // The endpoint caps `days` at 30 and `limit` at 20 (GetSceneShowsRequest);
  // a frontend that asked for more would get a 422, not a longer window.
  it('stay inside what the endpoint will serve', () => {
    expect(SCENE_CALENDAR_WINDOW_DAYS).toBeLessThanOrEqual(30)
    expect(SCENE_CALENDAR_ROW_CAP).toBeLessThanOrEqual(20)
  })
})

describe('calendarDateInZone', () => {
  it('files an evening show under the date its patrons experienced', () => {
    // 20:00 Saturday in Phoenix, which is 03:00 Sunday UTC.
    expect(
      calendarDateInZone(new Date('2026-08-09T03:00:00Z'), 'America/Phoenix')
    ).toBe('2026-08-08')
  })

  it('reads the same instant differently on the other side of a zone line', () => {
    expect(
      calendarDateInZone(new Date('2026-08-09T03:00:00Z'), 'Europe/London')
    ).toBe('2026-08-09')
  })
})

describe('sceneTonightDate', () => {
  // Mirrors the backend's 6am nightStartHour: at 01:00 Sunday, "tonight" is
  // still Saturday night, and this page's TONIGHT tag must point at the same
  // date /scenes/{slug}/tonight serves.
  it('names the evening date during the evening', () => {
    expect(
      sceneTonightDate(new Date('2026-08-09T03:00:00Z'), 'America/Phoenix')
    ).toBe('2026-08-08')
  })

  it('still names the previous date after midnight, before 6am', () => {
    // 01:00 Sunday in Phoenix.
    expect(
      sceneTonightDate(new Date('2026-08-09T08:00:00Z'), 'America/Phoenix')
    ).toBe('2026-08-08')
  })

  it('rolls over at 6am, not at midnight', () => {
    // 06:00 Sunday in Phoenix.
    expect(
      sceneTonightDate(new Date('2026-08-09T13:00:00Z'), 'America/Phoenix')
    ).toBe('2026-08-09')
  })

  it('crosses a month boundary backwards', () => {
    // 01:00 on Sep 1 in Phoenix is still the night of Aug 31.
    expect(
      sceneTonightDate(new Date('2026-09-01T08:00:00Z'), 'America/Phoenix')
    ).toBe('2026-08-31')
  })

  it('returns null for an unusable clock rather than guessing a night', () => {
    expect(sceneTonightDate(new Date('nonsense'), 'America/Phoenix')).toBeNull()
  })

  // The guard that matters most. `Intl.DateTimeFormat` treats an undefined
  // `timeZone` as the RUNTIME's zone, so without this a reader in Tokyo and a
  // reader in Los Angeles would each be told a different night was tonight in
  // Phoenix, and one of them would be shown an empty bucket asserting a zero
  // the page never checked.
  it('returns null rather than falling back to the viewer zone', () => {
    expect(sceneTonightDate(new Date('2026-08-09T03:00:00Z'))).toBeNull()
    expect(sceneTonightDate(new Date('2026-08-09T03:00:00Z'), undefined)).toBeNull()
  })
})

describe('resolveSceneTimeZone', () => {
  it('returns undefined when nothing can name a zone', () => {
    expect(resolveSceneTimeZone([])).toBeUndefined()
    expect(
      resolveSceneTimeZone([
        buildShow({ venue_timezone: undefined, venue_state: undefined }),
      ])
    ).toBeUndefined()
  })

  // `venues.timezone` is nullable and a geocode miss leaves it NULL, so the
  // state map is the second source. It is gated on `isShowTimezoneResolved`
  // because `resolveShowTimezone` silently answers America/Phoenix for any
  // state it does not recognise.
  it('falls back to the state map when the venue has no zone', () => {
    expect(
      resolveSceneTimeZone([
        buildShow({ venue_timezone: undefined, venue_state: 'IL' }),
      ])
    ).toBe('America/Chicago')
  })

  it('ignores a zone name the platform does not know', () => {
    expect(
      resolveSceneTimeZone([
        buildShow({ venue_timezone: 'Mars/Olympus', venue_state: undefined }),
      ])
    ).toBeUndefined()
  })

  it('picks the majority zone, not the first row', () => {
    const shows = [
      buildShow({ id: 1, venue_timezone: 'America/Los_Angeles', venue_state: 'CA' }),
      buildShow({ id: 2, venue_timezone: 'America/Phoenix' }),
      buildShow({ id: 3, venue_timezone: 'America/Phoenix' }),
    ]
    expect(resolveSceneTimeZone(shows)).toBe('America/Phoenix')
  })
})

describe('showCalendarDate', () => {
  it('prefers the row instant in the venue zone over the UTC event_date', () => {
    expect(showCalendarDate(buildShow())).toBe('2026-08-08')
  })

  it('falls back to the scene zone when the row has none', () => {
    const show = buildShow({ venue_timezone: undefined, venue_state: undefined })
    expect(showCalendarDate(show, 'America/Phoenix')).toBe('2026-08-08')
  })

  it('falls back to event_date when there is no usable instant at all', () => {
    const show = buildShow({ starts_at: 'not-a-date' })
    expect(showCalendarDate(show)).toBe('2026-08-09')
  })

  it('falls back to event_date when no zone is known', () => {
    const show = buildShow({ venue_timezone: undefined, venue_state: undefined })
    expect(showCalendarDate(show)).toBe('2026-08-09')
  })
})

describe('groupShowsByDate', () => {
  it('buckets by scene-local date, earliest first, preserving row order', () => {
    const shows = [
      buildShow({ id: 1, starts_at: '2026-08-09T02:00:00Z' }), // Aug 8, 19:00
      buildShow({ id: 2, starts_at: '2026-08-09T03:00:00Z' }), // Aug 8, 20:00
      buildShow({ id: 3, starts_at: '2026-08-10T03:00:00Z' }), // Aug 9, 20:00
    ]
    const groups = groupShowsByDate(shows)
    expect(groups.map(g => g.date)).toEqual(['2026-08-08', '2026-08-09'])
    expect(groups[0].shows.map(s => s.id)).toEqual([1, 2])
    expect(groups[1].shows.map(s => s.id)).toEqual([3])
  })

  it('returns nothing for an empty window', () => {
    expect(groupShowsByDate([])).toEqual([])
  })

  it('keeps a member city across a zone line on its own local date', () => {
    const shows = [
      // 23:00 Aug 8 in Los Angeles is 01:00 Aug 9 in Phoenix. The LA room's
      // patrons were out on the 8th, so that is the date it files under.
      buildShow({
        id: 1,
        starts_at: '2026-08-09T06:00:00Z',
        venue_timezone: 'America/Los_Angeles',
        venue_state: 'CA',
      }),
    ]
    expect(groupShowsByDate(shows, 'America/Phoenix')[0].date).toBe('2026-08-08')
  })
})

describe('formatCalendarDateHeading', () => {
  it('renders the middot register the mock draws', () => {
    expect(formatCalendarDateHeading('2026-08-08')).toBe('SAT · AUG 8')
  })

  it('does not shift the date across a negative UTC offset', () => {
    // The bug this guards: `new Date('2026-08-08')` is UTC midnight, which
    // renders as Aug 7 anywhere west of Greenwich.
    expect(formatCalendarDateHeading('2026-08-08')).toContain('AUG 8')
  })
})

// No date-group count helper is tested here: the calendar renders
// `formatDayCountLine` from sceneDay.ts, so the nightly page and this one say
// "0 shows listed" about the same night in the same words. That helper has its
// own coverage in sceneDay.test.ts.

describe('formatTimeZoneLabel', () => {
  it('names the scene zone, not the reader zone', () => {
    expect(
      formatTimeZoneLabel(new Date('2026-08-09T03:00:00Z'), 'America/Phoenix')
    ).toBe('MST')
    expect(
      formatTimeZoneLabel(new Date('2026-08-09T03:00:00Z'), 'America/Los_Angeles')
    ).toBe('PDT')
  })

  it('says nothing when no zone is known', () => {
    expect(formatTimeZoneLabel(new Date('2026-08-09T03:00:00Z'))).toBeNull()
  })
})

describe('venueSubLocality', () => {
  it('prints the room city so the scene reads as a region', () => {
    expect(venueSubLocality(buildShow({ venue_city: 'Mesa' }))).toBe('(Mesa)')
  })

  it('prints nothing rather than empty parentheses', () => {
    expect(venueSubLocality(buildShow({ venue_city: '   ' }))).toBeNull()
    expect(venueSubLocality(buildShow({ venue_city: undefined }))).toBeNull()
  })
})

describe('sceneStatParts', () => {
  it('keeps zero-valued parts (the London bug)', () => {
    expect(
      sceneStatParts({ venue_count: 2, artist_count: 0, upcoming_show_count: 197 })
    ).toEqual(['2 venues', '0 artists based here', '197 upcoming shows'])
  })

  it('pluralizes each part independently', () => {
    expect(
      sceneStatParts({ venue_count: 1, artist_count: 1, upcoming_show_count: 1 })
    ).toEqual(['1 venue', '1 artist based here', '1 upcoming show'])
  })

  it('names every category on a scene with nothing at all', () => {
    expect(
      sceneStatParts({ venue_count: 0, artist_count: 0, upcoming_show_count: 0 })
    ).toEqual(['0 venues', '0 artists based here', '0 upcoming shows'])
  })
})
