import { describe, it, expect } from 'vitest'
import { rosterUpcomingLine } from './sceneRosterLine'
import type { SceneArtist, SceneArtistNextShow } from './types'

function artist(overrides: Partial<SceneArtist> = {}): SceneArtist {
  return {
    id: 1,
    slug: 'gatecreeper',
    name: 'Gatecreeper',
    city: 'Phoenix',
    state: 'AZ',
    show_count: 6,
    is_active: true,
    ...overrides,
  }
}

function nextShow(overrides: Partial<SceneArtistNextShow> = {}): SceneArtistNextShow {
  return {
    id: 42,
    slug: 'gatecreeper-valley-bar',
    event_date: '2026-09-09',
    venue_name: 'Valley Bar',
    venue_slug: 'valley-bar',
    ...overrides,
  }
}

describe('rosterUpcomingLine', () => {
  it('states the count and the soonest show, with both link targets', () => {
    expect(
      rosterUpcomingLine(artist({ upcoming_show_count: 2, next_show: nextShow() }))
    ).toEqual({
      countText: '2 upcoming',
      day: 'Sep 9',
      dayHref: '/shows/gatecreeper-valley-bar',
      venueName: 'Valley Bar',
      venueHref: '/venues/valley-bar',
    })
  })

  it('counts one show without pluralising a word it does not print', () => {
    expect(
      rosterUpcomingLine(artist({ upcoming_show_count: 1, next_show: nextShow() }))?.countText
    ).toBe('1 upcoming')
  })

  // A band with nothing booked renders as a bare name. `0 upcoming` under every
  // quiet band in a forty-band roster is the noise this module exists to avoid.
  it('says nothing at all for a band with nothing booked', () => {
    expect(rosterUpcomingLine(artist({ upcoming_show_count: 0 }))).toBeNull()
  })

  // Absent is NOT zero. The field is required on the wire, but a cached body
  // fetched before the backend widened does not carry it, and printing
  // "0 upcoming" for a band that may have three would state something false.
  it('says nothing when the payload does not carry the count', () => {
    expect(rosterUpcomingLine(artist())).toBeNull()
    expect(rosterUpcomingLine(artist({ next_show: nextShow() }))).toBeNull()
  })

  it('refuses a negative count rather than printing it', () => {
    expect(rosterUpcomingLine(artist({ upcoming_show_count: -1 }))).toBeNull()
  })

  // The count is the reason the line exists; the show is what it can add. A
  // payload carrying one without the other still tells the reader how much is
  // booked.
  it('keeps the count when no show is attached', () => {
    expect(rosterUpcomingLine(artist({ upcoming_show_count: 3, next_show: null }))).toEqual({
      countText: '3 upcoming',
      day: null,
      dayHref: null,
      venueName: null,
      venueHref: null,
    })
  })

  // Shows are announced before the room is settled.
  it('dates a show with no venue and names no room', () => {
    const line = rosterUpcomingLine(
      artist({
        upcoming_show_count: 1,
        next_show: nextShow({ venue_name: '', venue_slug: '' }),
      })
    )
    expect(line?.day).toBe('Sep 9')
    expect(line?.venueName).toBeNull()
    expect(line?.venueHref).toBeNull()
  })

  it('treats a whitespace-only venue name as no room', () => {
    expect(
      rosterUpcomingLine(
        artist({ upcoming_show_count: 1, next_show: nextShow({ venue_name: '   ' }) })
      )?.venueName
    ).toBeNull()
  })

  // Show and venue slugs are both nullable, and `/shows/` or `/venues/` with an
  // empty slug resolves to the INDEX rather than 404ing (PSY-1754).
  it('falls back to the show id when the show has no slug', () => {
    expect(
      rosterUpcomingLine(
        artist({ upcoming_show_count: 1, next_show: nextShow({ slug: undefined }) })
      )?.dayHref
    ).toBe('/shows/42')
  })

  it('names a slugless room without linking it to the venues index', () => {
    const line = rosterUpcomingLine(
      artist({ upcoming_show_count: 1, next_show: nextShow({ venue_slug: '' }) })
    )
    expect(line?.venueName).toBe('Valley Bar')
    expect(line?.venueHref).toBeNull()
  })

  it('refuses a traversal-shaped venue slug', () => {
    expect(
      rosterUpcomingLine(
        artist({ upcoming_show_count: 1, next_show: nextShow({ venue_slug: '..' }) })
      )?.venueHref
    ).toBeNull()
  })

  // `event_date` is a calendar date, and anything else is not one it may print.
  // An undateable show still names its room, so the row is not left blank.
  it('drops an undateable show without dropping its room', () => {
    const line = rosterUpcomingLine(
      artist({
        upcoming_show_count: 1,
        next_show: nextShow({ event_date: '2026-09-09T00:00:00Z' }),
      })
    )
    expect(line?.day).toBeNull()
    expect(line?.dayHref).toBeNull()
    expect(line?.venueName).toBe('Valley Bar')
  })

  // A calendar date is read component-wise, never as an instant: `new Date`
  // would parse it as UTC midnight and print Sep 8 for every reader west of
  // Greenwich.
  it('reads the date on its own calendar, not as an instant', () => {
    expect(
      rosterUpcomingLine(
        artist({ upcoming_show_count: 1, next_show: nextShow({ event_date: '2026-01-01' }) })
      )?.day
    ).toBe('Jan 1')
  })
})
