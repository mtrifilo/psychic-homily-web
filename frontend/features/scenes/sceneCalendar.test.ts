import { describe, it, expect } from 'vitest'
import {
  calendarDateInZone,
  formatSliceDateHeading,
  formatTimeZoneLabel,
  sceneStatParts,
  sceneTonightDate,
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

describe('formatSliceDateHeading', () => {
  it('spells the date out, the way the locked mock draws it', () => {
    expect(formatSliceDateHeading('2026-08-17')).toBe('MONDAY, AUGUST 17')
  })

  // The root only ever shows dates within a day of now, so a year would be
  // noise on the one surface where it can never disambiguate anything. The
  // DATED permalinks keep formatDayFull, which does carry it.
  it('omits the year', () => {
    expect(formatSliceDateHeading('2026-08-17')).not.toContain('2026')
  })

  // The bug this guards: `new Date('2026-08-08')` is UTC midnight, which
  // renders as Aug 7 anywhere west of Greenwich.
  it('does not shift the date across a negative UTC offset', () => {
    expect(formatSliceDateHeading('2026-08-08')).toBe('SATURDAY, AUGUST 8')
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
