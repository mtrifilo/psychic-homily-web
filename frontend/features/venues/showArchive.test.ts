import { describe, expect, it } from 'vitest'
import { clampPage, parseArchiveYear } from '@/features/shows/showArchive'
import { groupByMonth, venueArchiveHref } from './showArchive'

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

/**
 * The archive's URL space, pinned in one place (PSY-1756). These strings are
 * what the year strip, the pager, the sitemap family and the crawlable route all
 * have to agree on, so a change here should be a deliberate migration rather
 * than a passing edit.
 */
describe('venueArchiveHref', () => {
  const slug = 'the-van-buren'

  it('addresses every year, page 1 as the bare venue page', () => {
    expect(venueArchiveHref(slug, null, 1)).toBe(
      '/venues/the-van-buren#venue-past-shows'
    )
  })

  it('keeps later all-year pages on the venue page as ?page=', () => {
    expect(venueArchiveHref(slug, null, 3)).toBe(
      '/venues/the-van-buren?page=3#venue-past-shows'
    )
  })

  it('gives a year its own path, with no query and no fragment', () => {
    expect(venueArchiveHref(slug, 2025, 1)).toBe(
      '/venues/the-van-buren/shows/2025'
    )
  })

  it('pages within a year with ?page=, never a deeper path segment', () => {
    expect(venueArchiveHref(slug, 2025, 2)).toBe(
      '/venues/the-van-buren/shows/2025?page=2'
    )
  })

  /**
   * The duplicate-content rule this ticket exists to enforce: a year is at
   * exactly one address. A `?year=` form would put the same rows at a second
   * URL with no canonical relationship to the first.
   */
  it('never emits a ?year= form', () => {
    for (const page of [1, 2, 50]) {
      for (const year of [null, 1999, 2025]) {
        expect(venueArchiveHref(slug, year, page)).not.toContain('year=')
      }
    }
  })

  /** Page 1 is the canonical view of a scope, so it never carries ?page=1. */
  it('leaves page 1 bare on both scopes', () => {
    expect(venueArchiveHref(slug, null, 1)).not.toContain('page=')
    expect(venueArchiveHref(slug, 2025, 1)).not.toContain('page=')
  })
})
