import { describe, it, expect } from 'vitest'
import { isShowPast, hasShowStarted } from './showTiming'

/**
 * The boundary these pin is venue-local midnight, so every case is written as
 * "an instant that is on one side of the venue's calendar day and the other
 * side of somebody else's". A test that only ever used UTC would pass against
 * the bug this replaces.
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

  it('is true one minute past venue-local midnight', () => {
    expect(
      isShowPast(
        { eventDate: '2026-03-15T03:00:00Z', ...PHOENIX },
        new Date('2026-03-15T07:01:00Z') // 00:01 Mar 15 Phoenix
      )
    ).toBe(true)
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
      // 21:00 Mar 14 Berlin (UTC+1) — 09:00 Mar 15 in Auckland.
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

    it('is unaffected in a zone that does not observe DST', () => {
      const show = { eventDate: '2026-03-09T03:00:00Z', ...PHOENIX } // 20:00 Mar 8
      expect(isShowPast(show, new Date('2026-03-09T06:59:00Z'))).toBe(false)
      expect(isShowPast(show, new Date('2026-03-09T07:01:00Z'))).toBe(true)
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
  it('is false before the start instant', () => {
    expect(
      hasShowStarted(
        { eventDate: '2026-03-15T03:00:00Z', ...PHOENIX },
        new Date('2026-03-15T02:59:00Z')
      )
    ).toBe(false)
  })

  it('is true at the start instant', () => {
    expect(
      hasShowStarted(
        { eventDate: '2026-03-15T03:00:00Z', ...PHOENIX },
        new Date('2026-03-15T03:00:00Z')
      )
    ).toBe(true)
  })

  it('is true one minute after the start instant, before venue-local midnight', () => {
    // The boundary that distinguishes it from `isShowPast`: mid-show, the show
    // has started but is not past.
    const show = { eventDate: '2026-03-15T03:00:00Z', ...PHOENIX }
    const midShow = new Date('2026-03-15T03:01:00Z')
    expect(hasShowStarted(show, midShow)).toBe(true)
    expect(isShowPast(show, midShow)).toBe(false)
  })

  it('does not depend on the venue timezone', () => {
    const at = new Date('2026-03-15T03:30:00Z')
    expect(hasShowStarted({ eventDate: '2026-03-15T03:00:00Z', ...PHOENIX }, at)).toBe(true)
    expect(hasShowStarted({ eventDate: '2026-03-15T03:00:00Z', ...AUCKLAND }, at)).toBe(true)
  })

  it.each([
    ['undefined', undefined],
    ['null', null],
    ['empty string', ''],
    ['a non-date string', 'n/a'],
  ])('counts a show with %s for its start as already started', (_label, value) => {
    expect(hasShowStarted({ eventDate: value, ...PHOENIX })).toBe(true)
  })
})
