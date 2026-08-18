import { describe, it, expect } from 'vitest'
import {
  archiveDocumentTitle,
  groupByMonth,
  monthRangeLabel,
  monthRangeLabelsByPage,
  parseArchiveYear,
  type ArchiveMonthCount,
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

/**
 * The defect PSY-1769 closes: labels used to come from a page's own ROWS, so
 * only pages already in the query cache could carry one and the rest of the
 * strip rendered bare numerals. These derive from the histogram instead, which
 * is what makes "every visible page link carries its label" reachable at all.
 */
describe('monthRangeLabelsByPage', () => {
  // Six months, ten shows each: page boundaries land mid-month at a page size
  // of 25, which is the case a naive month-per-page mapping gets wrong.
  const SIXTY_SHOWS: ArchiveMonthCount[] = [
    { year: 2025, month: 6, count: 10 },
    { year: 2025, month: 5, count: 10 },
    { year: 2025, month: 4, count: 10 },
    { year: 2025, month: 3, count: 10 },
    { year: 2025, month: 2, count: 10 },
    { year: 2025, month: 1, count: 10 },
  ]

  it('labels every requested page, not only the ones already fetched', () => {
    expect(
      monthRangeLabelsByPage({
        months: SIXTY_SHOWS,
        pageSize: 25,
        pages: [1, 2, 3],
      })
    ).toEqual({
      1: 'Jun–Apr', // rows 0-24: Jun, May, and half of Apr
      2: 'Apr–Feb', // rows 25-49: the rest of Apr through Feb
      3: 'Jan', // rows 50-59: a short last page that never leaves Jan
    })
  })

  it('names a single month when a page does not leave it', () => {
    expect(
      monthRangeLabelsByPage({
        months: [{ year: 2025, month: 9, count: 40 }],
        pageSize: 10,
        pages: [2],
      })
    ).toEqual({ 2: 'Sep' })
  })

  it('keeps both years when a page straddles the turn of one', () => {
    expect(
      monthRangeLabelsByPage({
        months: [
          { year: 2025, month: 1, count: 5 },
          { year: 2024, month: 12, count: 5 },
        ],
        pageSize: 10,
        pages: [1],
      })
    ).toEqual({ 1: 'Jan 2025–Dec 2024' })
  })

  it('never uses an em dash for the range', () => {
    const label = monthRangeLabelsByPage({
      months: SIXTY_SHOWS,
      pageSize: 25,
      pages: [1],
    })[1]
    expect(label).toContain('–')
    expect(label).not.toContain('—')
  })

  it('reads the histogram in the order it is given', () => {
    // The same six months ascending, as an upcoming list would page them.
    expect(
      monthRangeLabelsByPage({
        months: [...SIXTY_SHOWS].reverse(),
        pageSize: 25,
        pages: [1],
      })
    ).toEqual({ 1: 'Jan–Mar' })
  })

  it('omits a page that lies past the end of the histogram', () => {
    // The graceful edge when the counts and the list total disagree: the pager
    // falls back to a bare numeral rather than a clamped, wrong label.
    expect(
      monthRangeLabelsByPage({
        months: [{ year: 2025, month: 9, count: 10 }],
        pageSize: 25,
        pages: [1, 2, 3],
      })
    ).toEqual({ 1: 'Sep' })
  })

  it('returns nothing at all for an empty or unusable histogram', () => {
    expect(
      monthRangeLabelsByPage({ months: [], pageSize: 25, pages: [1] })
    ).toEqual({})
    // A malformed bucket must be dropped rather than trusted: rolled over, a
    // thirteenth month would print "Jan" of the wrong year, and a NaN count
    // would slide every later page's label.
    expect(
      monthRangeLabelsByPage({
        months: [
          { year: 2025, month: 13, count: 5 },
          { year: 2025, month: 9, count: Number.NaN },
        ],
        pageSize: 25,
        pages: [1],
      })
    ).toEqual({})
  })

  it('refuses a page size that could not have produced the pages', () => {
    expect(
      monthRangeLabelsByPage({ months: SIXTY_SHOWS, pageSize: 0, pages: [1] })
    ).toEqual({})
  })
})
