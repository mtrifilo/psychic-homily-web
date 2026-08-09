import { describe, it, expect } from 'vitest'
import {
  archiveDocumentTitle,
  groupByMonth,
  monthRangeLabel,
  parseArchiveYear,
  type ShowZone,
} from './showArchive'

/**
 * The venue archive's own derivations are covered end to end in
 * `features/venues/showArchive.test.ts`, through the venue-shaped face of this
 * module. What is pinned HERE is the axis that face hides: the zone is resolved
 * PER ROW, which is what lets an artist archive place a bill in the zone of the
 * venue that hosted it rather than in one zone for the whole table.
 */

interface Row {
  event_date: string
  zone: ShowZone
}

const CHICAGO: ShowZone = { state: 'IL', timezone: 'America/Chicago' }
const LONDON: ShowZone = { state: '', timezone: 'Europe/London' }
const zoneOf = (row: Row) => row.zone

// 04:00 UTC on new year's day: still Dec 31 in Chicago, already Jan 1 in
// London. One instant that two venues put in two different months, and two
// different YEARS.
const NEW_YEAR_EDGE = '2025-01-01T04:00:00Z'

describe('groupByMonth', () => {
  it('reads each row in its own zone rather than in the first one', () => {
    const groups = groupByMonth(
      [
        { event_date: NEW_YEAR_EDGE, zone: LONDON },
        { event_date: NEW_YEAR_EDGE, zone: CHICAGO },
      ],
      zoneOf
    )
    expect(groups.map(group => group.label)).toEqual(['Jan 2025', 'Dec 2024'])
  })

  it('merges consecutive rows that land in the same month from different zones', () => {
    // Two venues an ocean apart, one calendar month. The zone decides the
    // label; the label decides the group.
    const groups = groupByMonth(
      [
        { event_date: '2025-06-10T18:00:00Z', zone: CHICAGO },
        { event_date: '2025-06-11T18:00:00Z', zone: LONDON },
      ],
      zoneOf
    )
    expect(groups).toHaveLength(1)
    expect(groups[0].rows).toHaveLength(2)
  })

  it('falls back to the state map when a row has no resolved timezone', () => {
    const groups = groupByMonth(
      [{ event_date: '2025-06-01T03:00:00Z', zone: { state: 'AZ' } }],
      zoneOf
    )
    // 03:00 UTC is still May 31 in Arizona.
    expect(groups[0].label).toBe('May 2025')
  })
})

describe('monthRangeLabel', () => {
  it('spans the first and last rows in their own zones', () => {
    expect(
      monthRangeLabel(
        [
          { event_date: NEW_YEAR_EDGE, zone: LONDON },
          { event_date: NEW_YEAR_EDGE, zone: CHICAGO },
        ],
        zoneOf
      )
      // Both ends disagree about the year, so the year is the news and stays.
    ).toBe('Jan 2025–Dec 2024')
  })

  it('drops the year when both ends agree on it', () => {
    expect(
      monthRangeLabel(
        [
          { event_date: '2025-09-10T18:00:00Z', zone: CHICAGO },
          { event_date: '2025-06-10T18:00:00Z', zone: LONDON },
        ],
        zoneOf
      )
    ).toBe('Sep–Jun')
  })

  it('returns null for an empty page rather than an empty separator', () => {
    expect(monthRangeLabel([], zoneOf)).toBeNull()
  })

  it('never uses an em dash for the range', () => {
    const label = monthRangeLabel(
      [
        { event_date: '2025-09-10T18:00:00Z', zone: CHICAGO },
        { event_date: '2025-06-10T18:00:00Z', zone: CHICAGO },
      ],
      zoneOf
    )
    expect(label).not.toContain('—')
  })
})

describe('archiveDocumentTitle', () => {
  it('names whichever entity the archive belongs to', () => {
    for (const entityName of ['Turnstile', 'The Rebel Lounge']) {
      expect(
        archiveDocumentTitle({
          baseTitle: `${entityName} | Psychic Homily`,
          entityName,
          year: 2025,
          page: 1,
          totalPages: 4,
        })
      ).toBe(`${entityName} shows in 2025 | Psychic Homily`)
    }
  })

  it('leaves the title the route rendered alone on the default view', () => {
    expect(
      archiveDocumentTitle({
        baseTitle: 'Turnstile | Psychic Homily',
        entityName: 'Turnstile',
        year: null,
        page: 1,
        totalPages: 4,
      })
    ).toBe('Turnstile | Psychic Homily')
  })

  it('takes the brand from the LAST separator, so a name containing one survives', () => {
    expect(
      archiveDocumentTitle({
        baseTitle: 'Godspeed You | Black Emperor | Psychic Homily',
        entityName: 'Godspeed You | Black Emperor',
        year: 2025,
        page: 2,
        totalPages: 3,
      })
    ).toBe('Godspeed You | Black Emperor shows in 2025 (page 2 of 3) | Psychic Homily')
  })
})

describe('parseArchiveYear', () => {
  it('rejects anything outside the range the backend would answer', () => {
    expect(parseArchiveYear(1_759_000_000)).toBeNull()
    expect(parseArchiveYear(1899)).toBeNull()
    expect(parseArchiveYear(2025.5)).toBeNull()
    expect(parseArchiveYear(null)).toBeNull()
    expect(parseArchiveYear(2025)).toBe(2025)
  })
})
