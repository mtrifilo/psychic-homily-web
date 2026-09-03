import { describe, it, expect } from 'vitest'
import {
  billRecurrenceSegments,
  lastPlayedLabel,
  timelineCurrentPlaceLabel,
  timelineDateLabel,
  timelinePlaceLabel,
} from './showTimelineCopy'
import type { ShowTimelineEntry, ShowTimelineRecurrence } from '../types'

// 9:00 PM Aug 9 in Chicago, which is already Aug 10 in UTC and in Sydney.
// Every zone assertion below reads this one instant.
const NIGHT_ACROSS_THE_UTC_BOUNDARY = '2025-08-10T02:00:00Z'

describe('timelineDateLabel', () => {
  it('stamps the month uppercase and drops the year inside the subject year', () => {
    expect(
      timelineDateLabel(
        {
          event_date: NIGHT_ACROSS_THE_UTC_BOUNDARY,
          timezone: 'America/Chicago',
        },
        '2025'
      )
    ).toBe('AUG 9')
  })

  it('carries the year when the stop falls outside the subject year', () => {
    expect(
      timelineDateLabel(
        {
          event_date: NIGHT_ACROSS_THE_UTC_BOUNDARY,
          timezone: 'America/Chicago',
        },
        '2026'
      )
    ).toBe('AUG 9 2025')
  })

  // The year rule reads the stop's own clock too: a New Year's Eve show is
  // still the prior year in the room it happened in.
  it('decides the year rule on the stop clock, not the UTC clock', () => {
    const newYearsEveInChicago = {
      event_date: '2026-01-01T02:00:00Z',
      timezone: 'America/Chicago',
    }
    expect(timelineDateLabel(newYearsEveInChicago, '2026')).toBe('DEC 31 2025')
  })

  it('dates a stop on its own zone, neither UTC nor a fixed one', () => {
    const chicago = timelineDateLabel(
      { event_date: NIGHT_ACROSS_THE_UTC_BOUNDARY, timezone: 'America/Chicago' },
      '2025'
    )
    const sydney = timelineDateLabel(
      {
        event_date: NIGHT_ACROSS_THE_UTC_BOUNDARY,
        timezone: 'Australia/Sydney',
      },
      '2025'
    )
    const utc = timelineDateLabel(
      { event_date: NIGHT_ACROSS_THE_UTC_BOUNDARY, timezone: 'UTC' },
      '2025'
    )

    expect(chicago).toBe('AUG 9')
    expect(sydney).toBe('AUG 10')
    // One instant, two day numbers: the zone on the stop is what is read.
    expect(chicago).not.toBe(sydney)
    // And the room's clock outranks the wire's clock.
    expect(utc).toBe('AUG 10')
    expect(chicago).not.toBe(utc)
  })

  it('falls back to the state map when the stop carries no zone', () => {
    expect(
      timelineDateLabel(
        {
          event_date: NIGHT_ACROSS_THE_UTC_BOUNDARY,
          timezone: null,
          state: 'IL',
        },
        '2025'
      )
    ).toBe('AUG 9')
  })

  it('marks the day when neither the zone nor the state answers', () => {
    expect(
      timelineDateLabel(
        {
          event_date: NIGHT_ACROSS_THE_UTC_BOUNDARY,
          timezone: null,
          state: 'England',
        },
        '2025'
      )
    ).toBe('~AUG 9')
  })

  it('marks a stop once, ahead of a label that carries its year', () => {
    expect(
      timelineDateLabel(
        {
          event_date: NIGHT_ACROSS_THE_UTC_BOUNDARY,
          timezone: null,
          state: '',
        },
        '2026'
      )
    ).toBe('~AUG 9 2025')
  })
})

describe('timelinePlaceLabel', () => {
  it('names the room first and the city after it', () => {
    expect(timelinePlaceLabel({ venue_name: 'Metro', city: 'Chicago' })).toBe(
      'METRO, CHICAGO'
    )
  })

  it('falls back to city and state for a room with no name on record', () => {
    expect(timelinePlaceLabel({ city: 'Chicago', state: 'IL' })).toBe(
      'CHICAGO, IL'
    )
    expect(
      timelinePlaceLabel({ venue_name: '', city: 'Chicago', state: 'IL' })
    ).toBe('CHICAGO, IL')
  })

  // A whitespace-only column is stored as-is upstream, so it is not a name.
  it('treats a whitespace-only room name as no name at all', () => {
    expect(
      timelinePlaceLabel({ venue_name: '   ', city: 'Chicago', state: 'IL' })
    ).toBe('CHICAGO, IL')
  })

  it('trims whitespace around the parts it does keep', () => {
    expect(
      timelinePlaceLabel({ venue_name: ' Metro ', city: ' Chicago ' })
    ).toBe('METRO, CHICAGO')
  })

  it('returns "" when nothing is placeable, so the caller renders the date alone', () => {
    expect(timelinePlaceLabel({})).toBe('')
    expect(
      timelinePlaceLabel({ venue_name: null, city: null, state: null })
    ).toBe('')
    expect(
      timelinePlaceLabel({ venue_name: '  ', city: '  ', state: '  ' })
    ).toBe('')
  })
})

describe('timelineCurrentPlaceLabel', () => {
  // The stop the reader is already on. The city is what distinguishes one
  // neighbour from another; here the venue module below carries the address.
  it('names the room and drops the city', () => {
    expect(
      timelineCurrentPlaceLabel({ venue_name: 'Salt Shed', city: 'Chicago' })
    ).toBe('SALT SHED')
  })

  // A room with no name still has to say where it is, so it falls through to
  // the neighbour rule rather than rendering the date alone.
  it('falls back to city and state for a room with no name on record', () => {
    expect(timelineCurrentPlaceLabel({ city: 'Chicago', state: 'IL' })).toBe(
      'CHICAGO, IL'
    )
    expect(
      timelineCurrentPlaceLabel({
        venue_name: '   ',
        city: 'Chicago',
        state: 'IL',
      })
    ).toBe('CHICAGO, IL')
  })
})

describe('lastPlayedLabel', () => {
  it('states the month and the room in sentence case', () => {
    expect(
      lastPlayedLabel({
        event_date: '2023-11-15T02:00:00Z',
        timezone: 'America/Chicago',
        venue_name: 'Aragon Ballroom',
      })
    ).toBe('Nov 2023, Aragon Ballroom')
  })

  it('drops the room clause when there is no room to name', () => {
    const stop = {
      event_date: '2023-11-15T02:00:00Z',
      timezone: 'America/Chicago',
    }
    expect(lastPlayedLabel({ ...stop, venue_name: '' })).toBe('Nov 2023')
    expect(lastPlayedLabel({ ...stop, venue_name: null })).toBe('Nov 2023')
    expect(lastPlayedLabel({ ...stop, venue_name: '   ' })).toBe('Nov 2023')
  })

  // Month resolution does not exempt the line from the zone rule: a show on
  // the last night of a month is a different month on the wire's clock.
  it('reads the month on the stop zone, not UTC', () => {
    const lastNightOfNovemberInChicago = {
      event_date: '2023-12-01T02:00:00Z',
      venue_name: 'Aragon Ballroom',
    }
    expect(
      lastPlayedLabel({
        ...lastNightOfNovemberInChicago,
        timezone: 'America/Chicago',
      })
    ).toBe('Nov 2023, Aragon Ballroom')
    expect(
      lastPlayedLabel({ ...lastNightOfNovemberInChicago, timezone: 'UTC' })
    ).toBe('Dec 2023, Aragon Ballroom')
  })
})

describe('billRecurrenceSegments', () => {
  const artists = [
    { id: 1, name: 'Modest Mouse' },
    { id: 2, name: 'Califone' },
    { id: 3, name: 'Ocotillo Lights' },
  ]

  function makeStop(
    overrides: Partial<ShowTimelineEntry> = {}
  ): ShowTimelineEntry {
    return {
      show_id: 9,
      show_slug: 'aragon-nov-2023',
      // 8:00 PM Nov 14 2023 in Chicago.
      event_date: '2023-11-15T02:00:00Z',
      timezone: 'America/Chicago',
      venue_name: 'Aragon Ballroom',
      venue_slug: 'aragon-ballroom',
      city: 'Chicago',
      state: 'IL',
      ...overrides,
    }
  }

  /** An act with a prior date here, which is what turns the line on. */
  function priorDate(artist_id: number): ShowTimelineRecurrence {
    return { artist_id, is_hometown: false, last_played: makeStop() }
  }

  function hometown(artist_id: number): ShowTimelineRecurrence {
    return { artist_id, is_hometown: true, last_played: null }
  }

  function texts(
    recurrence: ShowTimelineRecurrence[],
    bill = artists
  ): string[] {
    return billRecurrenceSegments(recurrence, bill).map(segment => segment.text)
  }

  it('says nothing about a bill the archive has nothing on', () => {
    expect(texts([])).toEqual([])
  })

  // A line that says only "hometown show", once per act, repeats what the bill
  // above it already labels each of those acts with.
  it('says nothing when every act on the line is a hometown act', () => {
    expect(texts([hometown(1), hometown(2), hometown(3)])).toEqual([])
  })

  it('says nothing for a single local act playing alone', () => {
    expect(texts([hometown(2)])).toEqual([])
  })

  it('keeps the hometown clauses once one act has a prior date here', () => {
    expect(texts([priorDate(1), hometown(2), hometown(3)])).toEqual([
      'Modest Mouse last played Chicago: Nov 2023, Aragon Ballroom',
      'Califone: hometown show',
      'Ocotillo Lights: hometown show',
    ])
  })

  // The decision's other direction: one local act does not suppress a line the
  // acts around it have a story for.
  it('keeps a single hometown clause among acts with prior dates', () => {
    expect(texts([priorDate(1), hometown(2), priorDate(3)])).toEqual([
      'Modest Mouse last played Chicago: Nov 2023, Aragon Ballroom',
      'Califone: hometown show',
      'Ocotillo Lights last played Chicago: Nov 2023, Aragon Ballroom',
    ])
  })

  // An act with no hometown claim and no prior date is neither local nor
  // touring here. It renders no clause, so it cannot turn an all-local line on.
  it('does not let an act with no claim and no prior date turn the line on', () => {
    expect(
      texts([
        hometown(1),
        { artist_id: 2, is_hometown: false, last_played: null },
      ])
    ).toEqual([])
  })

  // Same rule for an act the bill cannot name: dropped before the test runs.
  it('does not let a prior date for an act off the bill turn the line on', () => {
    expect(texts([hometown(1), priorDate(404)])).toEqual([])
  })

  it('drops an entry for an act that is not on the bill', () => {
    expect(texts([priorDate(404), priorDate(1)])).toEqual([
      'Modest Mouse last played Chicago: Nov 2023, Aragon Ballroom',
    ])
  })
})
