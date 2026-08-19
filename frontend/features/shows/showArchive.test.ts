import { describe, it, expect } from 'vitest'
// The CLIENT entry point, deliberately. This is the half `VenuePastShows` uses;
// the module under test uses `nuqs/server`. Importing both is the only way to
// compare them, and a test file is the only place that may.
import { parseAsInteger } from 'nuqs'
import {
  archiveDocumentTitle,
  clampPage,
  groupByMonth,
  monthRangeLabel,
  monthRangeLabelsByPage,
  parseArchiveYear,
  type ArchiveMonthCount,
  type ShowZone,
} from './showArchive'
import { archiveIsFirstPage } from './showArchive.server'

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
  it('spans the end rows in their own zones, earlier month first', () => {
    expect(
      monthRangeLabel(
        [
          { event_date: '2025-01-01T12:00:00Z', zone: LONDON },
          { event_date: NEW_YEAR_EDGE, zone: CHICAGO },
        ],
        zoneOf,
        'one-year'
      )
      // Both ends disagree about the year, so the year is the news and stays;
      // the newest-first rows come back out chronologically.
    ).toBe('Dec 2024–Jan 2025')
  })

  it('drops the year when both ends agree on it', () => {
    expect(
      monthRangeLabel(
        [
          { event_date: '2025-09-10T18:00:00Z', zone: CHICAGO },
          { event_date: '2025-06-10T18:00:00Z', zone: LONDON },
        ],
        zoneOf,
        'one-year'
      )
    ).toBe('Jun–Sep')
  })

  // The row-derived twin of the histogram rule, and the form the ARTIST archive
  // still uses on its unfiltered view.
  it('keeps the year on an all-years pager, where nothing else supplies it', () => {
    expect(
      monthRangeLabel(
        [
          { event_date: '2025-09-10T18:00:00Z', zone: CHICAGO },
          { event_date: '2025-06-10T18:00:00Z', zone: CHICAGO },
        ],
        zoneOf,
        'all-years'
      )
    ).toBe('Jun–Sep 2025')
  })

  it('returns null for an empty page rather than an empty separator', () => {
    expect(monthRangeLabel([], zoneOf, 'one-year')).toBeNull()
  })

  it('never uses an em dash for the range', () => {
    const label = monthRangeLabel(
      [
        { event_date: '2025-09-10T18:00:00Z', zone: CHICAGO },
        { event_date: '2025-06-10T18:00:00Z', zone: CHICAGO },
      ],
      zoneOf,
      'one-year'
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

  // The four below came from `features/venues/showArchive.test.ts` when PSY-1842
  // deleted the venue-shaped wrapper this function had grown. They were never
  // venue-specific — both archives write their title through this one function —
  // and leaving them behind a venue-only entry point is how the artist archive
  // ends up untested for behaviour it also has.
  it('omits the page clause for a single-page scope', () => {
    expect(
      archiveDocumentTitle({
        baseTitle: 'The Van Buren | Psychic Homily',
        entityName: 'The Van Buren',
        year: 2025,
        page: 1,
        totalPages: 1,
      })
    ).toBe('The Van Buren shows in 2025 | Psychic Homily')
  })

  it('names the page on a deep all-years link', () => {
    expect(
      archiveDocumentTitle({
        baseTitle: 'The Van Buren | Psychic Homily',
        entityName: 'The Van Buren',
        year: null,
        page: 3,
        totalPages: 9,
      })
    ).toBe('The Van Buren shows (page 3 of 9) | Psychic Homily')
  })

  it('survives a route title with no brand suffix', () => {
    expect(
      archiveDocumentTitle({
        baseTitle: 'The Van Buren',
        entityName: 'The Van Buren',
        year: 2025,
        page: 2,
        totalPages: 2,
      })
    ).toBe('The Van Buren shows in 2025 (page 2 of 2)')
  })

  it('leaves page 1 of a multi-page scope unnumbered, matching its bare URL', () => {
    expect(
      archiveDocumentTitle({
        baseTitle: 'The Van Buren | Psychic Homily',
        entityName: 'The Van Buren',
        year: 2025,
        page: 1,
        totalPages: 4,
      })
    ).toBe('The Van Buren shows in 2025 | Psychic Homily')
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

  const labelPages = (
    overrides: Partial<Parameters<typeof monthRangeLabelsByPage>[0]> = {}
  ) =>
    monthRangeLabelsByPage({
      months: SIXTY_SHOWS,
      pageSize: 25,
      pages: [1, 2, 3],
      listTotal: 60,
      scope: 'one-year',
      ...overrides,
    })

  it('labels every requested page, not only the ones already fetched', () => {
    expect(labelPages()).toEqual({
      1: 'Apr–Jun', // rows 0-24: Jun, May, and half of Apr
      2: 'Feb–Apr', // rows 25-49: the rest of Apr through Feb
      3: 'Jan', // rows 50-59: a short last page that never leaves Jan
    })
  })

  it('names a single month when a page does not leave it', () => {
    expect(
      labelPages({
        months: [{ year: 2025, month: 9, count: 40 }],
        pageSize: 10,
        pages: [2],
        listTotal: 40,
      })
    ).toEqual({ 2: 'Sep' })
  })

  // PSY-1769's sharpest edge. On the ALL-YEARS pager the year is nowhere else on
  // the page — the year strip has nothing selected — so eliding it gives a deep
  // archive several page links reading "Apr–Jun", including, at seven pages or
  // fewer, two of them in the same control with the same accessible name.
  describe('all-years scope', () => {
    it('keeps the year on every label', () => {
      expect(labelPages({ scope: 'all-years' })).toEqual({
        1: 'Apr–Jun 2025',
        2: 'Feb–Apr 2025',
        3: 'Jan 2025',
      })
    })

    it('gives two same-month spans in different years distinct labels', () => {
      const labels = labelPages({
        scope: 'all-years',
        months: [
          { year: 2025, month: 8, count: 50 },
          { year: 2023, month: 8, count: 50 },
        ],
        pageSize: 50,
        pages: [1, 2],
        listTotal: 100,
      })
      expect(labels).toEqual({ 1: 'Aug 2025', 2: 'Aug 2023' })
      expect(labels[1]).not.toBe(labels[2])
    })

    it('names both years when a page straddles the turn of one', () => {
      expect(
        labelPages({
          scope: 'all-years',
          months: [
            { year: 2025, month: 1, count: 5 },
            { year: 2024, month: 12, count: 5 },
          ],
          pageSize: 10,
          pages: [1],
          listTotal: 10,
        })
      ).toEqual({ 1: 'Dec 2024–Jan 2025' })
    })
  })

  it('keeps both years on a year-scoped page that straddles one', () => {
    expect(
      labelPages({
        months: [
          { year: 2025, month: 1, count: 5 },
          { year: 2024, month: 12, count: 5 },
        ],
        pageSize: 10,
        pages: [1],
        listTotal: 10,
      })
    ).toEqual({ 1: 'Dec 2024–Jan 2025' })
  })

  it('never uses an em dash for the range', () => {
    const label = labelPages({ pages: [1] })[1]
    expect(label).toContain('–')
    expect(label).not.toContain('—')
  })

  it('reads the histogram in the order it is given', () => {
    // The same six months ascending, as an upcoming list would page them.
    expect(
      labelPages({ months: [...SIXTY_SHOWS].reverse(), pages: [1] })
    ).toEqual({ 1: 'Jan–Mar' })
  })

  // The two counts are separate reads and can drift apart by a row. The walk's
  // whole premise is that ordinal N in the histogram is ordinal N in the list,
  // so any disagreement means every span may be shifted — and this list is
  // newest-first, so a missing row is missing from the FRONT and the result
  // still looks well-formed. Labelling nothing is the only safe answer.
  describe('when the histogram and the list disagree', () => {
    it('produces no labels when the histogram is SHORT', () => {
      // 55 rows in the list, 50 in the histogram: the 5 newest are missing, so
      // walking ordinals would name page 1 for months five rows too old.
      expect(
        labelPages({
          months: [{ year: 2025, month: 6, count: 50 }],
          pageSize: 50,
          pages: [1, 2],
          listTotal: 55,
        })
      ).toEqual({})
    })

    it('produces no labels when the histogram is LONG', () => {
      expect(labelPages({ pages: [1, 2, 3], listTotal: 35 })).toEqual({})
    })

    it('trusts the histogram when the caller has no count to offer', () => {
      // A placeholder page, or a list that has not answered yet: the caller
      // passes nothing rather than a count it does not stand behind.
      expect(labelPages({ pages: [1], listTotal: undefined })).toEqual({
        1: 'Apr–Jun',
      })
    })
  })

  it('returns nothing at all for an empty or unusable histogram', () => {
    expect(labelPages({ months: [], listTotal: 0 })).toEqual({})
    // A malformed bucket must be dropped rather than trusted: rolled over, a
    // thirteenth month would print "Jan" of the wrong year, and a NaN count
    // would slide every later page's label.
    expect(
      labelPages({
        months: [
          { year: 2025, month: 13, count: 5 },
          { year: 2025, month: 9, count: Number.NaN },
        ],
        pages: [1],
      })
    ).toEqual({})
  })

  it('refuses a page size that could not have produced the pages', () => {
    expect(labelPages({ pageSize: 0, pages: [1] })).toEqual({})
  })
})

/**
 * PSY-1770. The year-archive route decides SERVER-side whether a URL is asking
 * for page 1, because page 1 is the only page it has rows to seed; the browser
 * decides again, with its own parser, which page to request. The two run in
 * different module graphs — `nuqs/server` here, `nuqs` in `VenuePastShows` — so
 * nothing structural forces them to agree.
 *
 * This is what does. A URL the server calls page 1 while the client calls it
 * page 2 wastes a read (harmless — the client refuses rows keyed to another
 * page); the reverse withholds rows from the canonical view, which silently
 * gives back the server-rendered archive PSY-1756 bought. Both directions are
 * caught here.
 */
describe('archiveIsFirstPage matches the client parser', () => {
  /**
   * A MODEL of how `VenuePastShows` resolves the same param — `useQueryState`
   * reads one string out of the URL and hands it to the parser's `parse` — not
   * the hook itself. Be precise about what that buys and what it does not.
   *
   * WHAT IT PINS, which is the volatile part: that `nuqs/server`'s parser agrees
   * with `nuqs`'s. Those are two structurally separate bundled copies (verified
   * in nuqs 2.9.0: `dist/index.js` and `dist/server.js` each carry their own
   * `createParser`), and the server's answer decides whether page 1's rows are
   * seeded while the client's decides which page is asked for. Going through
   * `parseServerSide` on both sides would compare the server helper with itself
   * and prove nothing.
   *
   * WHAT IT DOES NOT PIN: the app-router adapter between the URL and `parse`. A
   * nuqs release that changed how the HOOK resolves a repeated `?page=` while
   * leaving `parser.parse` alone would diverge production and leave this green.
   * On a nuqs bump, re-verify rather than trusting the pass; rendering
   * `VenuePastShows` and asserting the `initialData` it accepts would close it.
   */
  const clientPage = (queryString: string) => {
    const parser = parseAsInteger.withDefault(1)
    const raw = new URLSearchParams(queryString).get('page')
    const parsed = raw === null ? parser.defaultValue : (parser.parse(raw) ?? parser.defaultValue)
    return clampPage(parsed, 1_000)
  }

  /** The same URL, as the server receives it: a params record. */
  const serverParams = (queryString: string) => {
    const params: Record<string, string | string[]> = {}
    for (const [key, value] of new URLSearchParams(queryString)) {
      const existing = params[key]
      if (existing === undefined) params[key] = value
      else params[key] = Array.isArray(existing) ? [...existing, value] : [existing, value]
    }
    return params
  }

  it.each([
    '',
    'page=',
    'page=1',
    'page=2',
    'page=4',
    'page=1000',
    'page=1001',
    'page=0',
    'page=-3',
    'page=abc',
    'page=2abc',
    'page=%2B2',
    'page=%202%20',
    'page=2.7',
    'page=1e3',
    // Repeated: both sides must pick the FIRST occurrence.
    'page=2&page=1',
    'page=1&page=2',
  ])('agrees on ?%s', queryString => {
    expect(archiveIsFirstPage(serverParams(queryString))).toBe(
      clientPage(queryString) === 1
    )
  })

  /**
   * The bound is deliberately NOT a parameter of `archiveIsFirstPage`, and this
   * is why that is safe: a maximum can only pull a number down to itself, and
   * every bound in use is far above 1, so it can never change whether the page
   * is 1. A future surface with a different bound needs no change here.
   */
  it.each([1_000, 201, 2])('is independent of a %i-page bound', maxPage => {
    const parser = parseAsInteger.withDefault(1)
    for (const raw of ['1', '2', '5000', 'abc']) {
      expect(archiveIsFirstPage({ page: raw })).toBe(
        clampPage(parser.parse(raw) ?? parser.defaultValue, maxPage) === 1
      )
    }
  })
})
