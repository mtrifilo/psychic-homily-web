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
  parseArchiveYear,
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
