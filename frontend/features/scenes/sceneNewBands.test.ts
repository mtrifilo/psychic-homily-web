import { describe, it, expect } from 'vitest'
import { formatFirstListed, formatNewBandShow } from './sceneNewBands'
import type { SceneNewArtistShow } from './types'

function show(overrides: Partial<SceneNewArtistShow> = {}): SceneNewArtistShow {
  return {
    id: 1,
    event_date: '2026-08-22',
    starts_at: '2026-08-23T03:00:00Z',
    is_upcoming: true,
    venue_name: 'Nile Theater',
    venue_slug: 'nile-theater',
    ...overrides,
  }
}

describe('formatFirstListed', () => {
  it('formats the catalog timestamp as a short month and day', () => {
    expect(formatFirstListed('2026-07-14T18:30:00Z')).toBe('Jul 14')
  })

  // The window selected on this instant in UTC. Reading it in the viewer's own
  // zone would print "Jul 13" for anyone west of UTC, contradicting the cutoff
  // that put the band in the list.
  it('reads the instant in UTC, not the viewer zone', () => {
    expect(formatFirstListed('2026-07-14T02:00:00Z')).toBe('Jul 14')
    expect(formatFirstListed('2026-07-14T23:59:00Z')).toBe('Jul 14')
  })

  it('returns null rather than "Invalid Date" for an unparseable value', () => {
    expect(formatFirstListed('')).toBeNull()
    expect(formatFirstListed('not a date')).toBeNull()
  })
})

describe('formatNewBandShow', () => {
  it('names an upcoming show as the next one', () => {
    expect(formatNewBandShow(show())).toBe('next show Aug 22, Nile Theater')
  })

  // The payload attaches the most recent PAST show when there is no upcoming
  // one, so a single "next show" phrasing would be wrong on exactly the bands
  // whose row a reader most needs to read carefully.
  it('names a past show as the last one played', () => {
    expect(
      formatNewBandShow(
        show({ is_upcoming: false, event_date: '2026-03-02', venue_name: 'Valley Bar' })
      )
    ).toBe('last played Mar 2, Valley Bar')
  })

  it('states the honest absence when the band has no show at all', () => {
    expect(formatNewBandShow(undefined)).toBe('no show listed yet')
  })

  it('drops the venue clause rather than printing a dangling comma', () => {
    expect(formatNewBandShow(show({ venue_name: undefined }))).toBe('next show Aug 22')
    expect(formatNewBandShow(show({ venue_name: '  ' }))).toBe('next show Aug 22')
  })

  it('keeps the room when the date will not parse', () => {
    expect(formatNewBandShow(show({ event_date: '' }))).toBe(
      'next show at Nile Theater'
    )
  })

  it('falls back to the absence line when neither date nor room survives', () => {
    expect(formatNewBandShow(show({ event_date: '', venue_name: '' }))).toBe(
      'no show listed yet'
    )
  })

  // `event_date` is a CALENDAR date resolved at the venue, not an instant. A
  // UTC-midnight parse would render Aug 21 for every reader west of UTC.
  it('reads the event date component-wise so it cannot shift a day', () => {
    expect(formatNewBandShow(show({ event_date: '2026-01-01' }))).toBe(
      'next show Jan 1, Nile Theater'
    )
  })

  // The mock draws "first show" on every row. The payload carries no ordinal —
  // a band listed three weeks ago may already have played twice.
  it('never claims the show is the band first', () => {
    expect(formatNewBandShow(show())).not.toMatch(/first/i)
    expect(formatNewBandShow(show({ is_upcoming: false }))).not.toMatch(/first/i)
  })
})
