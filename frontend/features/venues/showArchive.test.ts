import { describe, expect, it } from 'vitest'
import { clampPage, parseArchiveYear } from '@/features/shows/showArchive'
import {
  archiveDocumentTitle,
  groupByMonth,
  monthRangeLabel,
} from './showArchive'

/**
 * Phoenix does not observe DST, so every fixture below means exactly what it
 * says regardless of when the suite runs. The zone matters: these helpers place
 * a UTC instant on a VENUE-local calendar, and a late-evening show is routinely
 * a different day (and sometimes a different month) in the two.
 */
const PHOENIX = { venueState: 'AZ', venueTimezone: 'America/Phoenix' }

const row = (event_date: string, state?: string | null) => ({
  event_date,
  state,
})

describe('groupByMonth', () => {
  it('collects consecutive rows of the same venue-local month under one label', () => {
    const groups = groupByMonth(
      [
        row('2025-09-20T03:00:00Z'),
        row('2025-09-06T03:00:00Z'),
        row('2025-08-30T03:00:00Z'),
      ],
      PHOENIX
    )
    expect(groups.map(g => g.label)).toEqual(['Sep 2025', 'Aug 2025'])
    expect(groups[0].rows).toHaveLength(2)
    expect(groups[1].rows).toHaveLength(1)
  })

  it('skips months with no rows rather than emitting empty headings', () => {
    const groups = groupByMonth(
      [row('2025-09-20T03:00:00Z'), row('2025-06-06T03:00:00Z')],
      PHOENIX
    )
    expect(groups.map(g => g.label)).toEqual(['Sep 2025', 'Jun 2025'])
  })

  it('places a late-night show on its VENUE-local month, not the UTC one', () => {
    // 2025-10-01T02:00Z is 7pm on September 30 in Phoenix.
    const groups = groupByMonth([row('2025-10-01T02:00:00Z')], PHOENIX)
    expect(groups[0].label).toBe('Sep 2025')
  })

  it("falls back to a row's own state for a venue with no resolved zone", () => {
    // 2025-10-01T05:30Z is 1:30am October 1 in New York and 10:30pm September
    // 30 in Phoenix, so the two calendars genuinely disagree here.
    const noZone = { venueState: 'AZ', venueTimezone: null }
    expect(groupByMonth([row('2025-10-01T05:30:00Z', 'NY')], noZone)[0].label).toBe(
      'Oct 2025'
    )
    expect(groupByMonth([row('2025-10-01T05:30:00Z', 'AZ')], noZone)[0].label).toBe(
      'Sep 2025'
    )
  })

  it("lets the venue's resolved zone win over a row's state", () => {
    // The venue timezone is the authoritative one (PSY-986); a row's state is
    // only the fallback for venues that have not been backfilled.
    expect(groupByMonth([row('2025-10-01T05:30:00Z', 'NY')], PHOENIX)[0].label).toBe(
      'Sep 2025'
    )
  })

  it('returns nothing for an empty page', () => {
    expect(groupByMonth([], PHOENIX)).toEqual([])
  })
})

describe('monthRangeLabel', () => {
  it('drops the year when both ends share it', () => {
    expect(
      monthRangeLabel(
        [row('2025-12-05T03:00:00Z'), row('2025-09-05T03:00:00Z')],
        PHOENIX
      )
    ).toBe('Dec–Sep')
  })

  it('keeps both years when a page straddles a year boundary', () => {
    expect(
      monthRangeLabel(
        [row('2026-01-05T03:00:00Z'), row('2025-11-05T03:00:00Z')],
        PHOENIX
      )
    ).toBe('Jan 2026–Nov 2025')
  })

  it('collapses a single-month page to one month', () => {
    expect(
      monthRangeLabel(
        [row('2025-09-20T03:00:00Z'), row('2025-09-02T03:00:00Z')],
        PHOENIX
      )
    ).toBe('Sep')
  })

  it('returns null for an empty page so callers can omit the label', () => {
    expect(monthRangeLabel([], PHOENIX)).toBeNull()
  })

  it('uses an en dash, never an em dash', () => {
    const label = monthRangeLabel(
      [row('2025-12-05T03:00:00Z'), row('2025-09-05T03:00:00Z')],
      PHOENIX
    )
    expect(label).toContain('–')
    expect(label).not.toContain('—')
  })
})

describe('clampPage', () => {
  it('keeps a page inside the archive bounds', () => {
    expect(clampPage(3, 1000)).toBe(3)
    expect(clampPage(0, 1000)).toBe(1)
    expect(clampPage(-4, 1000)).toBe(1)
    expect(clampPage(99_999, 1000)).toBe(1000)
  })

  it('resolves a non-finite page to 1 instead of propagating NaN', () => {
    expect(clampPage(Number.NaN, 1000)).toBe(1)
    expect(clampPage(Number.POSITIVE_INFINITY, 1000)).toBe(1)
  })

  it('floors a fractional page', () => {
    expect(clampPage(2.9, 1000)).toBe(2)
  })
})

describe('parseArchiveYear', () => {
  it('accepts a plausible calendar year', () => {
    expect(parseArchiveYear(2025)).toBe(2025)
  })

  it('reads an absent, zero, negative or out-of-range year as "every year"', () => {
    expect(parseArchiveYear(null)).toBeNull()
    expect(parseArchiveYear(0)).toBeNull()
    expect(parseArchiveYear(-2025)).toBeNull()
    expect(parseArchiveYear(1899)).toBeNull()
    // The ceiling is the backend's own (`maximum:"9999"`), so anything above it
    // is dropped here rather than sent and rejected with a 422.
    expect(parseArchiveYear(10_000)).toBeNull()
    expect(parseArchiveYear(1_759_000_000)).toBeNull()
  })

  it('rejects a fractional year', () => {
    expect(parseArchiveYear(2025.5)).toBeNull()
  })
})

describe('archiveDocumentTitle', () => {
  const baseTitle = 'The Van Buren | Psychic Homily'

  it('leaves the route title alone on the default view', () => {
    expect(
      archiveDocumentTitle({
        baseTitle,
        venueName: 'The Van Buren',
        year: null,
        page: 1,
        totalPages: 9,
      })
    ).toBe(baseTitle)
  })

  it('names the active year and page, keeping the brand suffix', () => {
    expect(
      archiveDocumentTitle({
        baseTitle,
        venueName: 'The Van Buren',
        year: 2025,
        page: 2,
        totalPages: 4,
      })
    ).toBe('The Van Buren shows in 2025 (page 2 of 4) | Psychic Homily')
  })

  it('omits the page clause for a single-page year', () => {
    expect(
      archiveDocumentTitle({
        baseTitle,
        venueName: 'The Van Buren',
        year: 2025,
        page: 1,
        totalPages: 1,
      })
    ).toBe('The Van Buren shows in 2025 | Psychic Homily')
  })

  it('names the page on a deep all-years link', () => {
    expect(
      archiveDocumentTitle({
        baseTitle,
        venueName: 'The Van Buren',
        year: null,
        page: 3,
        totalPages: 9,
      })
    ).toBe('The Van Buren shows (page 3 of 9) | Psychic Homily')
  })

  it('splits on the LAST separator, so a piped venue name survives', () => {
    // Venue names are contributor-supplied free text; splitting on the first
    // ' | ' would fold half the name into the brand suffix.
    expect(
      archiveDocumentTitle({
        baseTitle: 'Sunshine | Studios | Psychic Homily',
        venueName: 'Sunshine | Studios',
        year: 2025,
        page: 2,
        totalPages: 3,
      })
    ).toBe('Sunshine | Studios shows in 2025 (page 2 of 3) | Psychic Homily')
  })

  it('survives a route title with no brand suffix', () => {
    expect(
      archiveDocumentTitle({
        baseTitle: 'The Van Buren',
        venueName: 'The Van Buren',
        year: 2025,
        page: 2,
        totalPages: 2,
      })
    ).toBe('The Van Buren shows in 2025 (page 2 of 2)')
  })

  it('leaves page 1 of a multi-page year unnumbered, matching its bare URL', () => {
    expect(
      archiveDocumentTitle({
        baseTitle,
        venueName: 'The Van Buren',
        year: 2025,
        page: 1,
        totalPages: 4,
      })
    ).toBe('The Van Buren shows in 2025 | Psychic Homily')
  })
})
