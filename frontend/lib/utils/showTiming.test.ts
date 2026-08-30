import { describe, it, expect } from 'vitest'
import {
  getShowLifecycleState,
  hasReadableStartDate,
  isShowPast,
  hasShowStarted,
  showIsArchived,
} from './showTiming'

/**
 * `isShowPast`'s boundary is venue-local midnight, so its cases are written as
 * "an instant on one side of the venue's calendar day and the other side of
 * somebody else's". A suite that only ever used UTC would pass against the bug
 * this replaces.
 */

const PHOENIX = { timezone: 'America/Phoenix', state: 'AZ' }
const BERLIN = { timezone: 'Europe/Berlin', state: null }
const AUCKLAND = { timezone: 'Pacific/Auckland', state: null }

describe('isShowPast', () => {
  it('is false during the show', () => {
    // 8 PM Phoenix (UTC-7) on Mar 14, read at 10 PM Phoenix.
    expect(
      isShowPast(
        { eventDate: '2026-03-15T03:00:00Z', ...PHOENIX },
        new Date('2026-03-15T05:00:00Z')
      )
    ).toBe(false)
  })

  it('is false after the venue-local date has rolled over in UTC but not at the venue', () => {
    // The old start-instant/UTC comparisons called this past hours ago: it is
    // already Mar 15 in UTC while the venue is still on Mar 14.
    expect(
      isShowPast(
        { eventDate: '2026-03-15T03:00:00Z', ...PHOENIX },
        new Date('2026-03-15T06:59:00Z') // 23:59 Mar 14 Phoenix
      )
    ).toBe(false)
  })

  it('flips exactly at venue-local midnight, not a millisecond either side', () => {
    const show = { eventDate: '2026-03-15T03:00:00Z', ...PHOENIX } // 20:00 Mar 14
    const midnight = Date.parse('2026-03-15T07:00:00Z') // 00:00 Mar 15 Phoenix
    expect(isShowPast(show, new Date(midnight - 1))).toBe(false)
    expect(isShowPast(show, new Date(midnight))).toBe(true)
  })

  it('is false at the instant a midnight show starts', () => {
    // Show and reader share a venue-local date, so the strict comparison says no.
    expect(
      isShowPast(
        { eventDate: '2026-03-14T07:00:00Z', ...PHOENIX }, // 00:00 Mar 14 Phoenix
        new Date('2026-03-14T07:00:00Z')
      )
    ).toBe(false)
  })

  it('is false before the show', () => {
    expect(
      isShowPast(
        { eventDate: '2026-03-15T03:00:00Z', ...PHOENIX },
        new Date('2026-03-01T00:00:00Z')
      )
    ).toBe(false)
  })

  it('is true for a show years in the archive', () => {
    expect(
      isShowPast(
        { eventDate: '2019-06-01T03:00:00Z', ...PHOENIX },
        new Date('2026-03-15T03:00:00Z')
      )
    ).toBe(true)
  })

  describe('after-midnight shows', () => {
    it('keeps a 12:30 AM show live through its own venue-local day', () => {
      // 00:30 Mar 15 Phoenix. Under a start-instant rule this was past 30
      // minutes in; venue-local, it stays live for the rest of Mar 15.
      const show = { eventDate: '2026-03-15T07:30:00Z', ...PHOENIX }
      expect(isShowPast(show, new Date('2026-03-15T20:00:00Z'))).toBe(false)
      expect(isShowPast(show, new Date('2026-03-16T06:59:00Z'))).toBe(false)
      expect(isShowPast(show, new Date('2026-03-16T07:01:00Z'))).toBe(true)
    })
  })

  describe('zones that are not the reader / server zone', () => {
    it('holds a Berlin show live while it is already tomorrow in Auckland', () => {
      // 21:00 Mar 14 Berlin (UTC+1), which is 09:00 Mar 15 in Auckland.
      expect(
        isShowPast(
          { eventDate: '2026-03-14T20:00:00Z', ...BERLIN },
          new Date('2026-03-14T20:30:00Z')
        )
      ).toBe(false)
    })

    it('calls an Auckland show past while it is still the same UTC day', () => {
      // 20:00 Mar 14 Auckland (UTC+13) = 07:00 UTC Mar 14. At 12:00 UTC it is
      // already 01:00 Mar 15 in Auckland, though UTC has not rolled over.
      expect(
        isShowPast(
          { eventDate: '2026-03-14T07:00:00Z', ...AUCKLAND },
          new Date('2026-03-14T12:00:00Z')
        )
      ).toBe(true)
    })
  })

  describe('DST transitions', () => {
    it('moves the midnight boundary across the US spring-forward night', () => {
      // Los Angeles springs forward 02:00 PST → 03:00 PDT on Mar 8 2026, so
      // Mar 8 is a 23-hour local day. Venue-local midnight is 08:00 UTC on the
      // PST side of it and 07:00 UTC on the PDT side. A fixed UTC offset would
      // get one of the two wrong.
      const LA = { timezone: 'America/Los_Angeles', state: 'CA' }
      const beforeShift = { eventDate: '2026-03-08T04:00:00Z', ...LA } // 20:00 Mar 7 PST
      expect(isShowPast(beforeShift, new Date('2026-03-08T07:59:00Z'))).toBe(false) // 23:59 PST Mar 7
      expect(isShowPast(beforeShift, new Date('2026-03-08T08:01:00Z'))).toBe(true) // 00:01 PST Mar 8

      const afterShift = { eventDate: '2026-03-09T04:00:00Z', ...LA } // 21:00 Mar 8 PDT
      expect(isShowPast(afterShift, new Date('2026-03-09T06:59:00Z'))).toBe(false) // 23:59 PDT Mar 8
      expect(isShowPast(afterShift, new Date('2026-03-09T07:01:00Z'))).toBe(true) // 00:01 PDT Mar 9
    })

    it('rolls the day exactly once across the US fall-back repeat hour', () => {
      // Los Angeles falls back 02:00 PDT → 01:00 PST on Nov 1 2026, so 01:00
      // to 02:00 local happens twice. Midnight is still crossed once, and it
      // is crossed BEFORE the repeat, so the day must not roll back.
      const LA = { timezone: 'America/Los_Angeles', state: 'CA' }
      const show = { eventDate: '2026-11-01T03:00:00Z', ...LA } // 20:00 Oct 31 PDT
      expect(isShowPast(show, new Date('2026-11-01T06:59:00Z'))).toBe(false) // 23:59 PDT Oct 31
      expect(isShowPast(show, new Date('2026-11-01T07:01:00Z'))).toBe(true) // 00:01 PDT Nov 1
      expect(isShowPast(show, new Date('2026-11-01T08:30:00Z'))).toBe(true) // 01:30 PDT Nov 1
      expect(isShowPast(show, new Date('2026-11-01T09:30:00Z'))).toBe(true) // 01:30 PST Nov 1, the repeat
    })
  })

  describe('timezone resolution', () => {
    it('falls back to the state map when the venue has no timezone', () => {
      // NY is Eastern; 21:00 Mar 14 EDT = 01:00 Mar 15 UTC.
      const show = { eventDate: '2026-03-15T01:00:00Z', timezone: null, state: 'NY' }
      expect(isShowPast(show, new Date('2026-03-15T03:59:00Z'))).toBe(false) // 23:59 Mar 14 ET
      expect(isShowPast(show, new Date('2026-03-15T04:01:00Z'))).toBe(true) // 00:01 Mar 15 ET
    })

    it('falls through a malformed IANA name instead of throwing', () => {
      const show = { eventDate: '2026-03-15T01:00:00Z', timezone: 'Not/AZone', state: 'NY' }
      expect(() => isShowPast(show, new Date('2026-03-15T03:59:00Z'))).not.toThrow()
      expect(isShowPast(show, new Date('2026-03-15T03:59:00Z'))).toBe(false)
    })
  })

  describe('zones the fallback chain cannot resolve', () => {
    // Documenting a known-wrong case rather than pretending it does not exist.
    // `resolveShowTimezone` ends at `FALLBACK_SHOW_TIMEZONE`, because the state
    // map is US-only, so a non-US venue with no backfilled `timezone` is judged
    // on Arizona's calendar. This predates the module (every show date on the
    // site already renders through that chain); what is new is that the LISTING
    // boundary now depends on it too.
    it('falls back to America/Phoenix for a non-US venue with no resolved timezone', () => {
      const auckland = { eventDate: '2026-03-14T07:00:00Z', timezone: null, state: 'Auckland' }
      // 20:00 Mar 14 Auckland. Its own midnight passed at 11:00 UTC; Phoenix's
      // does not until 07:00 UTC on the 15th, so the listing reads live longer.
      expect(isShowPast(auckland, new Date('2026-03-14T12:00:00Z'))).toBe(false)
      expect(isShowPast(auckland, new Date('2026-03-15T07:01:00Z'))).toBe(true)
    })

    it('falls back to America/Phoenix when neither state nor timezone is known', () => {
      const unknown = { eventDate: '2026-03-15T03:00:00Z', timezone: null, state: null }
      expect(isShowPast(unknown, new Date('2026-03-15T06:59:00Z'))).toBe(false)
      expect(isShowPast(unknown, new Date('2026-03-15T07:01:00Z'))).toBe(true)
    })
  })

  describe('unreadable inputs', () => {
    it('treats an unreadable `now` as not-past rather than throwing', () => {
      // `Intl.formatToParts` throws RangeError on an invalid Date, and this runs
      // inside server components where that is a 500 for the whole page.
      const show = { eventDate: '2026-03-15T03:00:00Z', ...PHOENIX }
      expect(() => isShowPast(show, new Date('nonsense'))).not.toThrow()
      expect(isShowPast(show, new Date('nonsense'))).toBe(false)
    })
  })

  describe('undateable shows', () => {
    it.each([
      ['undefined', undefined],
      ['null', null],
      ['empty string', ''],
      ['a non-date string', 'n/a'],
    ])('counts a show with %s for its start as already happened', (_label, value) => {
      expect(isShowPast({ eventDate: value, ...PHOENIX })).toBe(true)
    })
  })
})

describe('hasShowStarted', () => {
  const START = '2026-03-15T03:00:00Z' // 20:00 Mar 14 Phoenix

  it('is false before the start instant', () => {
    expect(hasShowStarted(START, new Date('2026-03-15T02:59:00Z'))).toBe(false)
  })

  it('is true at the start instant', () => {
    expect(hasShowStarted(START, new Date(START))).toBe(true)
  })

  it('is true mid-show, while `isShowPast` is still false', () => {
    // The boundary that distinguishes the two exports: the show has started
    // but its venue-local day has hours left.
    const midShow = new Date('2026-03-15T03:01:00Z')
    expect(hasShowStarted(START, midShow)).toBe(true)
    expect(isShowPast({ eventDate: START, ...PHOENIX }, midShow)).toBe(false)
  })

  it.each([
    ['undefined', undefined],
    ['null', null],
    ['empty string', ''],
    ['a non-date string', 'n/a'],
  ])('counts a show with %s for its start as already started', (_label, value) => {
    expect(hasShowStarted(value)).toBe(true)
  })
})

describe('getShowLifecycleState', () => {
  const SHOW = { eventDate: '2026-03-15T03:00:00Z', ...PHOENIX } // 20:00 Mar 14

  it('is upcoming on an earlier venue-local day', () => {
    expect(getShowLifecycleState(SHOW, new Date('2026-03-14T06:59:00Z'))).toBe(
      'upcoming'
    )
  })

  it('is today from the first minute of the venue-local day', () => {
    // 00:00 Mar 14 Phoenix, twenty hours before doors.
    expect(getShowLifecycleState(SHOW, new Date('2026-03-14T07:00:00Z'))).toBe(
      'today'
    )
  })

  // The boundary the status stripe had to pick: the band is on stage, the
  // ticket offer is already withdrawn, and the listing still reads as
  // tonight's.
  it('is still today mid-show, when hasShowStarted has already flipped', () => {
    const midShow = new Date('2026-03-15T05:00:00Z') // 22:00 Mar 14 Phoenix
    expect(getShowLifecycleState(SHOW, midShow)).toBe('today')
    expect(hasShowStarted(SHOW.eventDate, midShow)).toBe(true)
  })

  it('flips to past exactly at venue-local midnight', () => {
    const midnight = Date.parse('2026-03-15T07:00:00Z') // 00:00 Mar 15 Phoenix
    expect(getShowLifecycleState(SHOW, new Date(midnight - 1))).toBe('today')
    expect(getShowLifecycleState(SHOW, new Date(midnight))).toBe('past')
  })

  // A reader in Auckland is a day ahead of Phoenix. The show is tonight where
  // it happens, which is the only calendar that matters.
  it('answers on the venue calendar, not the reader clock', () => {
    // 16:00 Mar 15 Auckland is 20:00 Mar 14 Phoenix: doors.
    expect(getShowLifecycleState(SHOW, new Date('2026-03-15T03:00:00Z'))).toBe(
      'today'
    )
  })

  it('agrees with isShowPast on every side of the boundary', () => {
    for (const at of [
      '2026-03-13T12:00:00Z',
      '2026-03-14T07:00:00Z',
      '2026-03-15T06:59:59Z',
      '2026-03-15T07:00:00Z',
      '2026-03-20T00:00:00Z',
    ]) {
      const now = new Date(at)
      expect(getShowLifecycleState(SHOW, now) === 'past').toBe(
        isShowPast(SHOW, now)
      )
    }
  })

  it.each([
    ['undefined', undefined],
    ['null', null],
    ['empty string', ''],
    ['a non-date string', 'n/a'],
  ])('counts a show with %s for its start as past', (_label, value) => {
    expect(getShowLifecycleState({ eventDate: value, ...PHOENIX })).toBe('past')
  })

  it('does not claim a show is past when the clock itself is unreadable', () => {
    expect(getShowLifecycleState(SHOW, new Date(NaN))).toBe('upcoming')
  })

  // Berlin at 20:00 is noon in Phoenix, so a zone-less Berlin venue happens to
  // agree here. Pinned to document the fallback, not to bless it: see the
  // KNOWN LIMIT note on `isShowPast`.
  it('judges a zone-less non-US venue on the state-map fallback', () => {
    expect(
      getShowLifecycleState(
        { eventDate: '2026-03-15T19:00:00Z', state: null, timezone: null },
        new Date('2026-03-15T20:00:00Z')
      )
    ).toBe('today')
  })
})

/**
 * The predicate every past-tense CLAIM on a show page branches through. Its
 * two refinements over the raw lifecycle — cancellation and an unreadable
 * date — are the whole point of it, so they are pinned here directly rather
 * than only through the components that consume them.
 */
describe('showIsArchived', () => {
  const DATED = '2026-03-15T03:00:00Z'

  it('archives a dateable, uncancelled show once its venue-local day has passed', () => {
    expect(showIsArchived({ eventDate: DATED, isCancelled: false }, 'past')).toBe(
      true
    )
  })

  // The lifecycle knows only about the calendar. A show that did not happen
  // cannot be remembered, and the stripe says CANCELLED, never PAST SHOW.
  it('never archives a cancelled show, however long ago it was', () => {
    expect(showIsArchived({ eventDate: DATED, isCancelled: true }, 'past')).toBe(
      false
    )
  })

  // `getShowLifecycleState` returns `past` for an unreadable date by a default
  // inherited from a cache-window caller, where "past" only meant "cache it
  // longer". That is not evidence the show happened.
  it.each([
    ['an unparseable date', 'not-a-date'],
    ['an empty date', ''],
    ['a null date', null],
    ['an undefined date', undefined],
  ])('does not archive a show with %s', (_label, eventDate) => {
    expect(showIsArchived({ eventDate, isCancelled: false }, 'past')).toBe(false)
  })

  // The live states, where a past-tense claim would contradict the stripe.
  it.each(['today', 'upcoming'] as const)(
    'does not archive a %s show',
    lifecycle => {
      expect(
        showIsArchived({ eventDate: DATED, isCancelled: false }, lifecycle)
      ).toBe(false)
    }
  )
})

describe('hasReadableStartDate', () => {
  it('accepts a date the rest of this module can parse', () => {
    expect(hasReadableStartDate('2026-03-15T03:00:00Z')).toBe(true)
  })

  it.each([
    ['unparseable', 'not-a-date'],
    ['empty', ''],
    ['null', null],
    ['undefined', undefined],
  ])('rejects a %s date', (_label, value) => {
    expect(hasReadableStartDate(value)).toBe(false)
  })
})
