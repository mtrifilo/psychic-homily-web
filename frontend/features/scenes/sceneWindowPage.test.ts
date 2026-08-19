import { beforeEach, describe, expect, it, vi } from 'vitest'

const { fetchSceneWeekChain } = vi.hoisted(() => ({ fetchSceneWeekChain: vi.fn() }))
vi.mock('./sceneWindowApi', () => ({ fetchSceneWeekChain }))

import type { SceneWeekDay, SceneWeekResponse } from './sceneWeek'
import { buildSceneWindowMetadata, getSceneWindow } from './sceneWindowPage'

/**
 * Where each window's bounds are actually decided.
 *
 * These are the assertions that can catch an off-by-one-day fault, which is the
 * only class of bug on this surface that renders confidently and is wrong: the
 * page still lists real shows under real headings, just for the wrong nights.
 * The clock is pinned and the timezone is fixed, because the answer depends on
 * BOTH and a test that let either float would pass in one hemisphere.
 */

function day(date: string, n = 0): SceneWeekDay {
  return {
    date,
    shows: Array.from({ length: n }, (_, i) => ({ id: `${date}-${i}` })),
  } as unknown as SceneWeekDay
}

/** Seven Monday-anchored days from `monday`, with per-date show counts. */
function week(monday: string, counts: Record<string, number> = {}): SceneWeekResponse {
  const start = new Date(`${monday}T00:00:00`)
  const days = Array.from({ length: 7 }, (_, i) => {
    const d = new Date(start)
    d.setDate(d.getDate() + i)
    const iso = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(
      d.getDate()
    ).padStart(2, '0')}`
    return day(iso, counts[iso] ?? 0)
  })
  return {
    slug: 'phoenix-az',
    scene_name: 'Phoenix, AZ',
    city: 'Phoenix',
    state: 'AZ',
    timezone: 'America/Phoenix',
    days,
    tracked_venues: [{ name: 'Valley Bar', slug: 'valley-bar' }],
  } as unknown as SceneWeekResponse
}

/**
 * 19:00 in Phoenix on the stated date — showtime, and well clear of both the
 * midnight and the 6am boundary.
 *
 * The offset is written into the literal rather than expressed as a UTC hour.
 * `2026-08-23T02:00:00Z` is the EVENING OF THE 22nd in Phoenix, so spelling
 * these as UTC makes every fixture silently one day early — which is the exact
 * fault class these tests exist to catch, and it caught this helper first.
 */
function phoenixEvening(date: string): Date {
  return new Date(`${date}T19:00:00-07:00`)
}

const dates = (days: SceneWeekDay[]) => days.map(d => d.date)

beforeEach(() => {
  vi.clearAllMocks()
})

describe('getSceneWindow — this-weekend', () => {
  // 2026-08-17 is a Monday; the weekend it anchors is Aug 21/22/23.
  it('serves Friday, Saturday and Sunday when viewed midweek', async () => {
    fetchSceneWeekChain.mockResolvedValue([week('2026-08-17', { '2026-08-21': 2 })])

    // Tuesday evening in Phoenix.
    const data = await getSceneWindow('phoenix-az', 'this-weekend', phoenixEvening('2026-08-19'))

    expect(dates(data!.days)).toEqual(['2026-08-21', '2026-08-22', '2026-08-23'])
    expect(data!.rendered).toBe(2)
  })

  // A weekend page renders its quiet nights, unlike the four-week window:
  // three bare rules is what tells a reader we checked.
  it('keeps empty weekend nights rather than dropping them', async () => {
    fetchSceneWeekChain.mockResolvedValue([week('2026-08-17')])

    const data = await getSceneWindow('phoenix-az', 'this-weekend', phoenixEvening('2026-08-19'))

    expect(dates(data!.days)).toHaveLength(3)
    expect(data!.rendered).toBe(0)
  })

  // The nights already behind the reader are not "this weekend".
  it('leaves a Sunday viewer only Sunday', async () => {
    fetchSceneWeekChain.mockResolvedValue([
      week('2026-08-17', { '2026-08-21': 3, '2026-08-23': 1 }),
    ])

    const data = await getSceneWindow('phoenix-az', 'this-weekend', phoenixEvening('2026-08-23'))

    expect(dates(data!.days)).toEqual(['2026-08-23'])
    expect(data!.rendered).toBe(1)
  })

  // The 6am night boundary, mirrored from the backend: at 01:00 Saturday the
  // night in progress is still FRIDAY, so Friday must not drop out from under
  // a reader who is out at the show.
  it('still counts Friday at 01:00 on Saturday', async () => {
    fetchSceneWeekChain.mockResolvedValue([week('2026-08-17', { '2026-08-21': 2 })])

    // 01:00 Saturday in Phoenix = 08:00 UTC Saturday.
    const data = await getSceneWindow(
      'phoenix-az',
      'this-weekend',
      new Date('2026-08-22T08:00:00Z')
    )

    expect(dates(data!.days)).toEqual(['2026-08-21', '2026-08-22', '2026-08-23'])
  })

  it('asks for exactly one week payload', async () => {
    fetchSceneWeekChain.mockResolvedValue([week('2026-08-17')])
    await getSceneWindow('phoenix-az', 'this-weekend', phoenixEvening('2026-08-18'))
    expect(fetchSceneWeekChain).toHaveBeenCalledWith('phoenix-az', 1)
  })
})

describe('getSceneWindow — next-4-weeks', () => {
  // FIVE payloads, not four: four Monday-anchored weeks only reach 28 days
  // ahead when today is Monday.
  it('asks for five week payloads', async () => {
    fetchSceneWeekChain.mockResolvedValue([week('2026-08-17')])
    await getSceneWindow('phoenix-az', 'next-4-weeks', phoenixEvening('2026-08-19'))
    expect(fetchSceneWeekChain).toHaveBeenCalledWith('phoenix-az', 5)
  })

  // The window ROLLS from tonight. A Monday-anchored four weeks would spend the
  // first days of the list on nights that already happened.
  it('drops nights before tonight and reaches 28 days ahead', async () => {
    fetchSceneWeekChain.mockResolvedValue([
      // Wednesday is "tonight"; Monday and Tuesday are behind it.
      week('2026-08-17', { '2026-08-17': 1, '2026-08-19': 1 }),
      week('2026-08-24'),
      week('2026-08-31'),
      week('2026-09-07'),
      // Sep 15 is 28 days after Aug 19 — one past the window's last night.
      week('2026-09-14', { '2026-09-15': 1, '2026-09-16': 1 }),
    ])

    const data = await getSceneWindow(
      'phoenix-az',
      'next-4-weeks',
      phoenixEvening('2026-08-19')
    )

    // Only dates that can answer are listed, so the past Monday is gone and so
    // is the night beyond the 28th day.
    expect(dates(data!.days)).toEqual(['2026-08-19', '2026-09-15'])
    expect(data!.rendered).toBe(2)
  })

  // 28 headings mostly reading `0` would bury the nights that have something.
  it('lists only nights that have shows', async () => {
    fetchSceneWeekChain.mockResolvedValue([
      week('2026-08-17', { '2026-08-20': 2 }),
      week('2026-08-24'),
      week('2026-08-31'),
      week('2026-09-07'),
      week('2026-09-14'),
    ])

    const data = await getSceneWindow(
      'phoenix-az',
      'next-4-weeks',
      phoenixEvening('2026-08-19')
    )

    expect(dates(data!.days)).toEqual(['2026-08-20'])
  })

  it('offers nothing wider, being the widest window in the family', async () => {
    fetchSceneWeekChain.mockResolvedValue([week('2026-08-17')])
    const data = await getSceneWindow(
      'phoenix-az',
      'next-4-weeks',
      phoenixEvening('2026-08-19')
    )
    expect(data!.widerWindow).toBeNull()
  })
})

describe('getSceneWindow — failure', () => {
  // The case that must reach `notFound()` rather than rendering an empty city.
  it('is null when the scene has no week to serve', async () => {
    fetchSceneWeekChain.mockResolvedValue(null)
    expect(
      await getSceneWindow('nope-zz', 'this-weekend', phoenixEvening('2026-08-19'))
    ).toBeNull()
  })
})

describe('buildSceneWindowMetadata', () => {
  // Each rolling window is its OWN canonical. Pointing two windows at one URL
  // is exactly the collapse this ticket exists to undo.
  it('canonicalises each window to its own segment', async () => {
    fetchSceneWeekChain.mockResolvedValue([week('2026-08-17', { '2026-08-21': 1 })])
    const meta = await buildSceneWindowMetadata(
      'phoenix-az',
      'this-weekend',
      phoenixEvening('2026-08-19')
    )
    expect(meta.alternates?.canonical).toContain('/scenes/phoenix-az/this-weekend')
  })

  it('states an honest zero rather than claiming the city is quiet', async () => {
    fetchSceneWeekChain.mockResolvedValue([week('2026-08-17')])
    const meta = await buildSceneWindowMetadata(
      'phoenix-az',
      'this-weekend',
      phoenixEvening('2026-08-19')
    )
    expect(meta.description).toBe(
      'No shows at the Phoenix rooms we track this weekend.'
    )
  })

  it('noindexes a scene that does not resolve', async () => {
    fetchSceneWeekChain.mockResolvedValue(null)
    const meta = await buildSceneWindowMetadata(
      'nope-zz',
      'next-4-weeks',
      phoenixEvening('2026-08-19')
    )
    expect(meta.robots).toEqual({ index: false, follow: false })
  })
})
