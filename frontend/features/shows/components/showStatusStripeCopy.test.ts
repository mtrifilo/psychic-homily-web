import { describe, it, expect } from 'vitest'
import {
  buildShowStatusStripeSegments,
  ESTIMATED_SHOW_LENGTH_HOURS,
  type ShowStatusStripeInput,
} from './showStatusStripeCopy'

/**
 * Every case here is written against a venue in a zone the test runner is not
 * in, because the bug this component exists to avoid is a reader's clock
 * leaking into a venue's calendar. A suite written in UTC would pass with that
 * bug present.
 */

const PHOENIX = { state: 'AZ', timezone: 'America/Phoenix' } // UTC-7, no DST

function input(overrides: Partial<ShowStatusStripeInput> = {}): ShowStatusStripeInput {
  return {
    // 8 PM Wed Apr 15 2026 in Phoenix.
    eventDate: '2026-04-16T03:00:00Z',
    isCancelled: false,
    lifecycle: 'upcoming',
    ...PHOENIX,
    ...overrides,
  }
}

describe('buildShowStatusStripeSegments', () => {
  describe('plain upcoming', () => {
    it('is weekday, date and doors, with no label word and no countdown', () => {
      expect(
        buildShowStatusStripeSegments(
          input({ doorsAt: '2026-04-16T02:00:00Z' }) // 7 PM Phoenix
        )
      ).toEqual(['WED', 'APR 15', 'DOORS 7PM'])
    })

    it('degrades to weekday and date when doors are unannounced', () => {
      expect(buildShowStatusStripeSegments(input())).toEqual(['WED', 'APR 15'])
    })

    // Doors is the time a reader plans around days out; the full doors/music
    // call belongs to the day itself.
    it('does not carry the music time days out', () => {
      expect(
        buildShowStatusStripeSegments(
          input({
            doorsAt: '2026-04-16T02:00:00Z',
            musicAt: '2026-04-16T03:00:00Z',
          })
        )
      ).toEqual(['WED', 'APR 15', 'DOORS 7PM'])
    })

    // A show at 8 PM Phoenix is already the NEXT day in UTC. Naming the UTC
    // day would put the wrong weekday on the band for every evening show west
    // of Greenwich.
    it('names the venue-local day, not the UTC one', () => {
      const segments = buildShowStatusStripeSegments(input())
      expect(segments).toContain('WED')
      expect(segments).not.toContain('THU')
    })
  })

  describe('tonight', () => {
    it('is TONIGHT, doors, music and an estimated end', () => {
      expect(
        buildShowStatusStripeSegments(
          input({
            lifecycle: 'today',
            doorsAt: '2026-04-16T02:00:00Z', // 7 PM
            musicAt: '2026-04-16T03:00:00Z', // 8 PM
          })
        )
      ).toEqual(['TONIGHT', 'DOORS 7PM', 'MUSIC 8PM', 'ENDS ~11PM (EST.)'])
    })

    it('drops the estimated end with the doors time it is derived from', () => {
      expect(
        buildShowStatusStripeSegments(
          input({ lifecycle: 'today', musicAt: '2026-04-16T03:00:00Z' })
        )
      ).toEqual(['TONIGHT', 'MUSIC 8PM'])
    })

    it('is the bare word when nothing but the date is known', () => {
      expect(
        buildShowStatusStripeSegments(input({ lifecycle: 'today' }))
      ).toEqual(['TONIGHT'])
    })

    it('estimates the end at exactly the documented constant past doors', () => {
      const doorsAt = '2026-04-16T02:00:00Z'
      const segments = buildShowStatusStripeSegments(
        input({ lifecycle: 'today', doorsAt })
      )
      const expectedEnd = new Date(
        Date.parse(doorsAt) + ESTIMATED_SHOW_LENGTH_HOURS * 3_600_000
      )
      expect(
        expectedEnd.toLocaleTimeString('en-US', {
          timeZone: 'America/Phoenix',
          hour: 'numeric',
          hour12: true,
        })
      ).toBe('11 PM')
      expect(segments.at(-1)).toBe('ENDS ~11PM (EST.)')
    })

    it('carries a half-hour doors time through to the estimate', () => {
      expect(
        buildShowStatusStripeSegments(
          input({ lifecycle: 'today', doorsAt: '2026-04-16T02:30:00Z' })
        )
      ).toEqual(['TONIGHT', 'DOORS 7:30PM', 'ENDS ~11:30PM (EST.)'])
    })

    // Late doors push the estimate past midnight. It is still an estimate, and
    // "12AM"/"1AM" is what the venue's clock will read.
    it('wraps the estimate past midnight without changing the date', () => {
      expect(
        buildShowStatusStripeSegments(
          input({ lifecycle: 'today', doorsAt: '2026-04-16T06:00:00Z' }) // 11 PM
        )
      ).toEqual(['TONIGHT', 'DOORS 11PM', 'ENDS ~3AM (EST.)'])
    })

    // The estimate is added to the INSTANT, so a venue that springs forward
    // between doors and the estimated end reads the wall-clock hour it will
    // actually see. Chicago jumps 2 AM to 3 AM on Mar 8 2026: doors 11 PM plus
    // four hours lands at 4 AM local, not 3 AM.
    it('follows the venue clock across a spring-forward transition', () => {
      expect(
        buildShowStatusStripeSegments(
          input({
            state: 'IL',
            timezone: 'America/Chicago',
            lifecycle: 'today',
            eventDate: '2026-03-08T04:00:00Z', // 10 PM Sat Mar 7 Chicago
            doorsAt: '2026-03-08T05:00:00Z', // 11 PM Sat Mar 7 Chicago
          })
        )
      ).toEqual(['TONIGHT', 'DOORS 11PM', 'ENDS ~4AM (EST.)'])
    })
  })

  describe('past', () => {
    it('states the show is past and when, and promises nothing below it', () => {
      expect(
        buildShowStatusStripeSegments(
          input({ lifecycle: 'past', doorsAt: '2026-04-16T02:00:00Z' })
        )
      ).toEqual(['PAST SHOW', 'WED, APR 15 2026'])
    })
  })

  describe('cancelled', () => {
    it('outranks every timing state', () => {
      for (const lifecycle of ['upcoming', 'today', 'past'] as const) {
        expect(
          buildShowStatusStripeSegments(
            input({
              lifecycle,
              isCancelled: true,
              doorsAt: '2026-04-16T02:00:00Z',
            })
          )
        ).toEqual(['CANCELLED', '15 APR'])
      }
    })
  })

  describe('unreadable input', () => {
    // The band renders nothing rather than inventing a date. Saying nothing is
    // recoverable; a confident wrong date is not.
    it('returns no segments for an undateable show', () => {
      expect(buildShowStatusStripeSegments(input({ eventDate: '' }))).toEqual([])
      expect(
        buildShowStatusStripeSegments(input({ eventDate: 'whenever' }))
      ).toEqual([])
    })

    it('drops an unreadable doors time instead of the whole band', () => {
      expect(
        buildShowStatusStripeSegments(
          input({ lifecycle: 'today', doorsAt: 'soon', musicAt: null })
        )
      ).toEqual(['TONIGHT'])
    })
  })

  // A venue with no `timezone` falls back to the US state map, which defaults
  // to Phoenix for anything it does not know. That fallback is wrong for
  // non-US venues and is tracked separately; pinned here so a change to it is
  // a deliberate one.
  it('falls back to the state map when the venue has no timezone', () => {
    expect(
      buildShowStatusStripeSegments(
        input({ timezone: null, state: 'NY', doorsAt: '2026-04-16T02:00:00Z' })
      )
    ).toEqual(['WED', 'APR 15', 'DOORS 10PM'])
  })
})
