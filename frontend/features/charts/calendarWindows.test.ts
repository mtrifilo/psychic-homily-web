import { describe, expect, it } from 'vitest'
import {
  adjacentPeriodNav,
  archiveHref,
  calendarBounds,
  calendarWindowFromRoute,
  formatArchiveSubtitle,
  formatArchiveTitle,
  frontPageArchiveLinks,
  isCalendarWindowClosed,
  parseCalendarChartWindow,
} from './calendarWindows'

describe('calendarWindows', () => {
  const mid2026 = new Date('2026-07-18T12:00:00Z')
  const early2026 = new Date('2026-01-01T00:00:00Z')
  const after2026 = new Date('2027-01-15T00:00:00Z')

  it('parses year and quarter bounds as UTC [start,end)', () => {
    expect(calendarBounds('2026')).toEqual({
      start: new Date('2026-01-01T00:00:00.000Z'),
      end: new Date('2027-01-01T00:00:00.000Z'),
      year: 2026,
      quarter: null,
    })
    expect(calendarBounds('2026-q2')?.start.toISOString()).toBe(
      '2026-04-01T00:00:00.000Z'
    )
    expect(calendarBounds('2026-q2')?.end.toISOString()).toBe(
      '2026-07-01T00:00:00.000Z'
    )
  })

  it('rejects pre-launch, future, and malformed calendar windows', () => {
    expect(parseCalendarChartWindow('2025', mid2026)).toBeNull()
    expect(parseCalendarChartWindow('2025-q4', mid2026)).toBeNull()
    expect(parseCalendarChartWindow('2026-q4', mid2026)).toBeNull()
    expect(parseCalendarChartWindow('2027', mid2026)).toBeNull()
    expect(parseCalendarChartWindow('2026-Q1', mid2026)).toBeNull()
    expect(parseCalendarChartWindow('2026-q0', mid2026)).toBeNull()
    expect(parseCalendarChartWindow('bogus', mid2026)).toBeNull()
  })

  it('accepts started calendar periods including the current open year', () => {
    expect(parseCalendarChartWindow('2026', early2026)).toBe('2026')
    expect(parseCalendarChartWindow('2026-q1', mid2026)).toBe('2026-q1')
    expect(parseCalendarChartWindow('2026-q2', mid2026)).toBe('2026-q2')
    expect(parseCalendarChartWindow('2026-q3', mid2026)).toBe('2026-q3')
  })

  it('builds windows from archive route segments', () => {
    expect(calendarWindowFromRoute('2026', undefined, mid2026)).toBe('2026')
    expect(calendarWindowFromRoute('2026', 'q2', mid2026)).toBe('2026-q2')
    expect(calendarWindowFromRoute('2026', 'q9', mid2026)).toBeNull()
    expect(calendarWindowFromRoute('most-active-artists', undefined, mid2026)).toBeNull()
  })

  it('formats archive titles, subtitles, and hrefs', () => {
    expect(formatArchiveTitle('2026')).toBe('Charts — 2026')
    expect(formatArchiveTitle('2026-q2')).toBe('Charts — Q2 2026')
    expect(formatArchiveSubtitle('2026-q1', mid2026)).toBe(
      'The quarter in the ledger — closed March 31, 2026.'
    )
    expect(formatArchiveSubtitle('2026', mid2026)).toContain('open through')
    expect(formatArchiveSubtitle('2026', after2026)).toBe(
      'The year in the ledger — closed December 31, 2026.'
    )
    expect(archiveHref('2026')).toBe('/charts/2026')
    expect(archiveHref('2026-q2')).toBe('/charts/2026/q2')
  })

  it('marks closed vs open windows', () => {
    expect(isCalendarWindowClosed('2026-q1', mid2026)).toBe(true)
    expect(isCalendarWindowClosed('2026-q2', mid2026)).toBe(true)
    expect(isCalendarWindowClosed('2026-q3', mid2026)).toBe(false)
    expect(isCalendarWindowClosed('2026', mid2026)).toBe(false)
    expect(isCalendarWindowClosed('2026', after2026)).toBe(true)
  })

  it('builds adjacent-period nav with gated year/quarter links', () => {
    const nav = adjacentPeriodNav('2026', mid2026)
    expect(nav?.prevYear).toBeNull()
    expect(nav?.nextYear).toBeNull()
    expect(nav?.viewingYear).toBe(true)
    expect(nav?.quarters.map(q => [q.quarter, q.available, q.current])).toEqual([
      [1, true, false],
      [2, true, false],
      [3, true, false],
      [4, false, false],
    ])

    const qNav = adjacentPeriodNav('2026-q2', mid2026)
    expect(qNav?.viewingYear).toBe(false)
    expect(qNav?.quarters.find(q => q.quarter === 2)?.current).toBe(true)
    expect(qNav?.yearHref).toBe('/charts/2026')
  })

  it('exposes front-page archive entry links', () => {
    const links = frontPageArchiveLinks(mid2026)
    expect(links.year).toEqual({ href: '/charts/2026', label: '2026' })
    expect(links.previousQuarter).toEqual({
      href: '/charts/2026/q2',
      label: 'Q2 2026',
    })
  })
})
