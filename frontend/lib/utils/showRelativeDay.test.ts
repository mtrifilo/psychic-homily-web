import { describe, expect, it } from 'vitest'
import { showRelativeDay } from './showDateBadge'
import { venueLocalDaysUntil } from './showTiming'

// Unmocked on purpose: these cases are about real zone arithmetic (offsets,
// DST, month and year rollover), which a stubbed formatter would not exercise.
// November 2026 dates sit after the US DST change on Nov 1, so Chicago is
// UTC-6 (CST) and Phoenix is UTC-7 all year.
const CHICAGO = { state: 'IL', timezone: 'America/Chicago' }
const PHOENIX = { state: 'AZ', timezone: 'America/Phoenix' }

function relativeDay(
  eventDate: string,
  venue: { state: string; timezone: string | null },
  now: string
) {
  return showRelativeDay(eventDate, venue.state, venue.timezone, new Date(now))
}

describe('showRelativeDay', () => {
  it('reads the VENUE calendar, not the reader clock, across a zone boundary', () => {
    // A Phoenix reader at 11 PM on Nov 3 (06:00Z Nov 4) is already past
    // midnight in Chicago, where it is Nov 4.
    const now = '2026-11-04T06:00:00Z'
    // 00:30 Nov 5 in Chicago: the reader's own calendar says two days away,
    // the venue's says tomorrow.
    expect(relativeDay('2026-11-05T06:30:00Z', CHICAGO, now)).toBe('tomorrow')
    // 1 AM Nov 4 in Chicago is today there, though it is still Nov 3 for
    // the reader.
    expect(relativeDay('2026-11-04T07:00:00Z', CHICAGO, now)).toBe('today')
  })

  it('dates an after-midnight set by the calendar day it starts on', () => {
    // 10 PM Nov 3 in Chicago. A 1 AM Nov 4 set is the venue's tomorrow, not
    // part of tonight: the rule is the calendar date, with no late-night grace.
    const now = '2026-11-04T04:00:00Z'
    expect(relativeDay('2026-11-04T07:00:00Z', CHICAGO, now)).toBe('tomorrow')
  })

  it('keeps a show that already started as today until the venue day ends', () => {
    // 8 PM Nov 7 in Phoenix (03:00Z Nov 8).
    const eventDate = '2026-11-08T03:00:00Z'
    // 10:30 PM: started, still today.
    expect(relativeDay(eventDate, PHOENIX, '2026-11-08T05:30:00Z')).toBe('today')
    // 11:59 PM: still today.
    expect(relativeDay(eventDate, PHOENIX, '2026-11-08T06:59:00Z')).toBe('today')
    // 12:01 AM Nov 8: the day is over, so the row prints its date.
    expect(relativeDay(eventDate, PHOENIX, '2026-11-08T07:01:00Z')).toBeNull()
  })

  it('names nothing two or more days out', () => {
    // Noon Nov 3 in Phoenix; the show is 8 PM Nov 5.
    expect(
      relativeDay('2026-11-06T03:00:00Z', PHOENIX, '2026-11-03T19:00:00Z')
    ).toBeNull()
  })

  it('rolls over month and year boundaries', () => {
    // 11 PM Oct 31 Phoenix -> 8 PM Nov 1.
    expect(
      relativeDay('2026-11-02T03:00:00Z', PHOENIX, '2026-11-01T06:00:00Z')
    ).toBe('tomorrow')
    // 11 PM Dec 31 Phoenix -> 8 PM Jan 1.
    expect(
      relativeDay('2027-01-02T03:00:00Z', PHOENIX, '2027-01-01T06:00:00Z')
    ).toBe('tomorrow')
  })

  it('counts a 25-hour DST day as one day', () => {
    // 11:30 PM Oct 31 in Chicago (CDT, 04:30Z Nov 1); Nov 1 is 25 hours long.
    // The show is 11:30 PM Nov 1 (CST, 05:30Z Nov 2), 25 hours later.
    expect(
      relativeDay('2026-11-02T05:30:00Z', CHICAGO, '2026-11-01T04:30:00Z')
    ).toBe('tomorrow')
  })

  it('withholds the word on a guessed zone', () => {
    // No venue zone and a state outside the US map: the zone is the
    // fallback guess, so the row keeps its date even when it would be today.
    expect(
      relativeDay(
        '2026-11-08T03:00:00Z',
        { state: '', timezone: null },
        '2026-11-08T01:00:00Z'
      )
    ).toBeNull()
    // A state the map knows is a named zone: the word is allowed.
    expect(
      relativeDay(
        '2026-11-08T03:00:00Z',
        { state: 'AZ', timezone: null },
        '2026-11-08T01:00:00Z'
      )
    ).toBe('today')
  })

  it('returns null for an unreadable date', () => {
    expect(relativeDay('not a date', PHOENIX, '2026-11-08T01:00:00Z')).toBeNull()
  })
})

describe('venueLocalDaysUntil', () => {
  it('goes negative once the venue day has passed', () => {
    expect(
      venueLocalDaysUntil(
        { eventDate: '2026-11-08T03:00:00Z', ...PHOENIX },
        // Noon Nov 8 in Phoenix, the day after the 8 PM Nov 7 show.
        new Date('2026-11-08T19:00:00Z')
      )
    ).toBe(-1)
  })

  it('returns null for an unreadable now', () => {
    expect(
      venueLocalDaysUntil(
        { eventDate: '2026-11-08T03:00:00Z', ...PHOENIX },
        new Date('nope')
      )
    ).toBeNull()
  })
})
