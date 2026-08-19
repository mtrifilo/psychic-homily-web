import { describe, expect, it } from 'vitest'
import {
  SCENE_WINDOW_ORDER,
  capWindowRows,
  countWindowShows,
  flattenWeekDays,
  formatWindowRange,
  rollingDays,
  sceneWindowHref,
  weekendDays,
} from './sceneWindow'
import type { SceneWeekDay, SceneWeekResponse } from './sceneWeek'

/** A day row carrying `n` distinguishable shows. */
function day(date: string, n = 0): SceneWeekDay {
  return {
    date,
    shows: Array.from({ length: n }, (_, i) => ({ id: `${date}-${i}` })),
  } as unknown as SceneWeekDay
}

/** 2026-08-17 is a Monday, so this is one full Monday-anchored week. */
const WEEK_2026_W34 = [
  day('2026-08-17'), // Mon
  day('2026-08-18'), // Tue
  day('2026-08-19'), // Wed
  day('2026-08-20'), // Thu
  day('2026-08-21'), // Fri
  day('2026-08-22'), // Sat
  day('2026-08-23'), // Sun
]

describe('sceneWindowHref', () => {
  // The one window whose segment does not match its key. The route shipped as
  // `/week` before the family had names, and a shared URL outlives the naming.
  it('maps this-week to the /week segment that actually shipped', () => {
    expect(sceneWindowHref('phoenix-az', 'this-week')).toBe('/scenes/phoenix-az/week')
  })

  it('maps every other window to its own segment', () => {
    expect(sceneWindowHref('phoenix-az', 'tonight')).toBe('/scenes/phoenix-az/tonight')
    expect(sceneWindowHref('phoenix-az', 'this-weekend')).toBe(
      '/scenes/phoenix-az/this-weekend'
    )
    expect(sceneWindowHref('phoenix-az', 'next-4-weeks')).toBe(
      '/scenes/phoenix-az/next-4-weeks'
    )
  })

  // The bug this ticket exists to fix: two chips pointing at one page.
  it('gives all four windows distinct destinations', () => {
    const hrefs = SCENE_WINDOW_ORDER.map(key => sceneWindowHref('phoenix-az', key))
    expect(new Set(hrefs).size).toBe(SCENE_WINDOW_ORDER.length)
  })
})

describe('weekendDays', () => {
  it('picks Friday, Saturday and Sunday out of a Monday-anchored week', () => {
    expect(weekendDays(WEEK_2026_W34).map(d => d.date)).toEqual([
      '2026-08-21',
      '2026-08-22',
      '2026-08-23',
    ])
  })

  // Selection is by WEEKDAY, not by slicing days[4..6] — an index would keep
  // returning three rows while silently labelling the wrong nights.
  it('is driven by the weekday, not by position in the array', () => {
    expect(weekendDays([day('2026-08-23'), day('2026-08-19')]).map(d => d.date)).toEqual([
      '2026-08-23',
    ])
  })

  // `new Date('2026-08-23')` is UTC midnight, which is Saturday the 22nd in
  // every US zone. Parsing component-wise is what stops the whole weekend
  // sliding back a day.
  it('does not shift the weekend in a negative-offset timezone', () => {
    const sunday = weekendDays([day('2026-08-23')])
    expect(sunday).toHaveLength(1)
    expect(sunday[0].date).toBe('2026-08-23')
  })
})

describe('rollingDays', () => {
  it('drops days before the anchor', () => {
    expect(rollingDays(WEEK_2026_W34, '2026-08-20', 28).map(d => d.date)).toEqual([
      '2026-08-20',
      '2026-08-21',
      '2026-08-22',
      '2026-08-23',
    ])
  })

  it('takes at most count days', () => {
    expect(rollingDays(WEEK_2026_W34, '2026-08-17', 3).map(d => d.date)).toEqual([
      '2026-08-17',
      '2026-08-18',
      '2026-08-19',
    ])
  })

  // A weekend viewed on Sunday is one night, not three. Listing the two behind
  // it under a "this weekend" header would describe a backward stretch of time
  // with a forward label.
  it('leaves a Sunday viewer only Sunday', () => {
    expect(rollingDays(weekendDays(WEEK_2026_W34), '2026-08-23', 3).map(d => d.date)).toEqual(
      ['2026-08-23']
    )
  })
})

describe('flattenWeekDays', () => {
  it('concatenates consecutive week payloads in order', () => {
    const weeks = [
      { days: [day('2026-08-17')] },
      { days: [day('2026-08-24')] },
    ] as unknown as SceneWeekResponse[]
    expect(flattenWeekDays(weeks).map(d => d.date)).toEqual(['2026-08-17', '2026-08-24'])
  })

  it('tolerates a payload with no days array', () => {
    expect(flattenWeekDays([{} as SceneWeekResponse])).toEqual([])
  })
})

describe('capWindowRows', () => {
  it('reports no truncation when the window fits', () => {
    const result = capWindowRows([day('2026-08-21', 2), day('2026-08-22', 3)], 60)
    expect(result.truncated).toBe(false)
    expect(result.rendered).toBe(5)
    expect(result.days).toHaveLength(2)
  })

  // Truncation is REPORTED, not inferred from `rendered === cap` — that
  // comparison cannot tell a cut list from a window holding exactly that many.
  it('reports truncation when the window holds exactly the cap plus one', () => {
    const result = capWindowRows([day('2026-08-21', 4)], 3)
    expect(result.truncated).toBe(true)
    expect(result.rendered).toBe(3)
  })

  it('does not report truncation when the window holds exactly the cap', () => {
    const result = capWindowRows([day('2026-08-21', 3)], 3)
    expect(result.truncated).toBe(false)
    expect(result.rendered).toBe(3)
  })

  // A day split by the cap is KEPT with its rows trimmed, so the reader still
  // sees the date rather than having it vanish mid-window.
  it('trims the day the cap lands inside rather than dropping it', () => {
    const result = capWindowRows([day('2026-08-21', 2), day('2026-08-22', 5)], 4)
    expect(result.days.map(d => d.date)).toEqual(['2026-08-21', '2026-08-22'])
    expect(result.days[1].shows).toHaveLength(2)
    expect(result.rendered).toBe(4)
    expect(result.truncated).toBe(true)
  })

  it('does not mutate the input day rows', () => {
    const days = [day('2026-08-21', 5)]
    capWindowRows(days, 2)
    expect(days[0].shows).toHaveLength(5)
  })
})

describe('countWindowShows', () => {
  it('sums across days and tolerates a missing shows array', () => {
    expect(countWindowShows([day('2026-08-21', 2), {} as SceneWeekDay])).toBe(2)
  })
})

describe('formatWindowRange', () => {
  it('names a span across days', () => {
    // Same spelling as the week page's `formatWeekRange`, so a reader moving
    // between windows never sees the date style change under them.
    expect(formatWindowRange([day('2026-08-21'), day('2026-08-23')])).toBe(
      'Fri, Aug 21 – Sun, Aug 23, 2026'
    )
  })

  // A weekend viewed on Sunday renders one night; a header still promising
  // three would be a claim the page cannot keep.
  it('names a single day without a range', () => {
    expect(formatWindowRange([day('2026-08-23')])).toBe('Sun, Aug 23, 2026')
  })

  it('has no span to state for an empty window', () => {
    expect(formatWindowRange([])).toBeNull()
  })
})
