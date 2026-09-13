import { describe, expect, it } from 'vitest'
import {
  SCENE_WINDOW_LABEL,
  SCENE_WINDOW_ORDER,
  allUpcomingHref,
  capWindowRows,
  countWindowShows,
  flattenWeekDays,
  formatMonthDay,
  formatWindowRange,
  rollingDays,
  sceneCityListHref,
  sceneDayPhrase,
  sceneWeekPhrase,
  sceneWeekStepLabel,
  sceneWindowHref,
  sceneWindowTitle,
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

describe('sceneCityListHref', () => {
  it('filters a list surface to one scene pair', () => {
    expect(sceneCityListHref('/artists', 'Phoenix', 'AZ')).toBe(
      '/artists?cities=Phoenix%2CAZ'
    )
  })

  // Percent-encoding is transport hygiene, not separation: `parseCitiesParam`
  // reads the value already decoded, so `%2C` and `,` are the same character
  // by the time the pair is split. What this pins is the space, which must not
  // arrive as a raw space or a `+`.
  it('keeps a multi-word city in one pair', () => {
    expect(sceneCityListHref('/artists', 'San Francisco', 'CA')).toBe(
      '/artists?cities=San%20Francisco%2CCA'
    )
  })

  // The gap line's destination is the list of the bands it counts, which is a
  // second param on the same href rather than a second href builder.
  it('carries extra params after the city pair', () => {
    expect(
      sceneCityListHref('/artists', 'Phoenix', 'AZ', { missing: 'listen' })
    ).toBe('/artists?cities=Phoenix%2CAZ&missing=listen')
  })

  // The shows link and the artists link agree about the format because both go
  // through this one function.
  it('is what allUpcomingHref points at /shows with', () => {
    expect(allUpcomingHref('Phoenix', 'AZ')).toBe(
      sceneCityListHref('/shows', 'Phoenix', 'AZ')
    )
  })
})

/**
 * The one title rule, as a table across every state that has to obey it.
 *
 * The faults it guards are the ones that read as confident and are wrong: a
 * permalink calling itself "tonight" or "this week" long after the night or the
 * week has ended, and a date parsed as a UTC instant naming the day before.
 */
describe('the window title rule', () => {
  it.each(SCENE_WINDOW_ORDER)('titles the %s window by its label', key => {
    expect(sceneWindowTitle(SCENE_WINDOW_LABEL[key], 'Chicago')).toBe(
      `${SCENE_WINDOW_LABEL[key]} in Chicago`
    )
  })

  it('titles the rolling night as the window, and a dated one as its date', () => {
    expect(sceneWindowTitle(sceneDayPhrase('2026-09-14', true), 'Chicago')).toBe(
      'Tonight in Chicago'
    )
    expect(sceneWindowTitle(sceneDayPhrase('2026-09-14', false), 'Chicago')).toBe(
      'Sep 14 in Chicago'
    )
  })

  it('titles the current week as the window, and an archived one by its Monday', () => {
    expect(sceneWindowTitle(sceneWeekPhrase('2026-09-14', true), 'Chicago')).toBe(
      'This week in Chicago'
    )
    expect(sceneWindowTitle(sceneWeekPhrase('2026-09-07', false), 'Chicago')).toBe(
      'Week of Sep 7 in Chicago'
    )
  })

  // A week that has not happened yet is not "this week" either, and it is not
  // archived — one rule covers both by keying on `is_current_week` alone.
  it('names a future week by its Monday, like an archived one', () => {
    expect(sceneWeekPhrase('2026-09-21', false)).toBe('Week of Sep 21')
  })

  // The killer bug: `new Date('2026-09-14')` is UTC midnight, which prints as
  // Sep 13 in every negative-offset zone.
  it('reads a calendar date as a calendar date, not a UTC instant', () => {
    expect(formatMonthDay('2026-09-14')).toBe('Sep 14')
    expect(formatMonthDay('2026-01-01')).toBe('Jan 1')
  })
})

describe('sceneWeekStepLabel', () => {
  // "Last" and "next" are claims about now; only the current week may make them.
  it('names the neighbours of the current week relatively', () => {
    expect(sceneWeekStepLabel('2026-09-14', -1, true)).toBe('Last week')
    expect(sceneWeekStepLabel('2026-09-14', 1, true)).toBe('Next week')
  })

  it('names the neighbours of any other week by their own Monday', () => {
    expect(sceneWeekStepLabel('2026-09-07', -1, false)).toBe('Week of Aug 31')
    expect(sceneWeekStepLabel('2026-09-07', 1, false)).toBe('Week of Sep 14')
  })

  // Seven days back from the first Monday of a month is the previous month,
  // and from the first week of a year the previous one.
  it('crosses a month and a year boundary', () => {
    expect(sceneWeekStepLabel('2026-03-02', -1, false)).toBe('Week of Feb 23')
    expect(sceneWeekStepLabel('2026-01-05', -1, false)).toBe('Week of Dec 29')
  })
})
