import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const { fetchSceneDay } = vi.hoisted(() => ({ fetchSceneDay: vi.fn() }))
vi.mock('./sceneDayApi', () => ({ fetchSceneDay }))

import type { SceneDayResponse } from './sceneDay'
import { buildSceneDayMetadata } from './sceneDayPage'

const day = (over: Partial<SceneDayResponse> = {}): SceneDayResponse =>
  ({
    slug: 'phoenix-az',
    scene_name: 'Phoenix, AZ',
    city: 'Phoenix',
    state: 'AZ',
    date: '2026-07-31',
    timezone: 'America/Phoenix',
    iso_week: '2026-W31',
    show_count: 4,
    prev_date: '2026-07-30',
    next_date: '2026-08-01',
    is_tonight: true,
    is_past_day: false,
    shows: [{ id: 1 }, { id: 2 }, { id: 3 }, { id: 4 }],
    tracked_venues: [],
    ...over,
  }) as SceneDayResponse

/**
 * The `<head>` carries three product decisions that live nowhere else: the
 * query-language title, the split canonical (rolling /tonight names the WEEK
 * permalink, a dated permalink names itself), and the noindex on a night with
 * nothing on it. Nothing else in the suite touches them, so without this file
 * any of the three can be reworded silently.
 */
describe('buildSceneDayMetadata', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    vi.restoreAllMocks()
    // Unconditional: one test below pins the clock, and a leaked fake clock
    // would follow it into every test after it in this file.
    vi.useRealTimers()
  })

  it('titles the live night in the words someone would type', async () => {
    fetchSceneDay.mockResolvedValue(day())

    const meta = await buildSceneDayMetadata('phoenix-az')

    expect(meta.title).toBe('Phoenix Shows Tonight — Friday, July 31, 2026')
  })

  // A dated permalink is permanent. Calling an archived Tuesday "tonight" would
  // be false the day after it was written, and it is the indexed URL.
  it('drops the word from a date that is not the live night', async () => {
    fetchSceneDay.mockResolvedValue(day({ is_tonight: false }))

    const meta = await buildSceneDayMetadata('phoenix-az', '2026-07-31')

    expect(meta.title).toBe('Phoenix Shows — Friday, July 31, 2026')
  })

  it('names the count and the city in the description', async () => {
    fetchSceneDay.mockResolvedValue(day())

    const meta = await buildSceneDayMetadata('phoenix-az')

    expect(meta.description).toContain('4 shows')
    expect(meta.description).toContain('Phoenix')
  })

  // Data-honest even in a snippet read out of context: never "no shows in
  // Phoenix", always our own calendar — and still carrying the count.
  it('says 0 shows LISTED for a quiet night, and does not speak for the city', async () => {
    fetchSceneDay.mockResolvedValue(day({ shows: [], show_count: 0 }))

    const meta = await buildSceneDayMetadata('phoenix-az')

    expect(meta.description).toContain('0 shows listed')
    expect(meta.description).toContain('Phoenix')
    expect(meta.description).not.toMatch(/no shows in/i)
  })

  // Why the week rather than the night is argued once, in buildSceneDayMetadata.
  it('points the rolling /tonight canonical at the WEEK permalink', async () => {
    fetchSceneDay.mockResolvedValue(day())

    const meta = await buildSceneDayMetadata('phoenix-az')

    expect(meta.alternates?.canonical).toBe(
      'https://psychichomily.com/scenes/phoenix-az/2026-W31'
    )
    // og:url must not contradict the canonical tag.
    expect(meta.openGraph?.url).toBe(
      'https://psychichomily.com/scenes/phoenix-az/2026-W31'
    )
  })

  // The fixture is a live night (`is_tonight: true`), so this also pins the
  // DISCRIMINATOR: an implementation keying off `day.is_tonight` instead of the
  // absent `date` argument would collapse every same-day permalink into the
  // week, and only this shape catches it.
  it('self-canonicalizes a dated permalink even on the night it names', async () => {
    fetchSceneDay.mockResolvedValue(day())

    const meta = await buildSceneDayMetadata('phoenix-az', '2026-07-31')

    expect(meta.alternates?.canonical).toBe(
      'https://psychichomily.com/scenes/phoenix-az/2026-07-31'
    )
    expect(meta.openGraph?.url).toBe(
      'https://psychichomily.com/scenes/phoenix-az/2026-07-31'
    )
  })

  // THE BOUNDARY CASE. The clock is pinned to 01:30 on Monday 2026-08-03, where
  // the backend's 6am night boundary still answers Sunday 2026-08-02: the LAST
  // day of 2026-W31, while that same Monday already opens 2026-W32. Without the
  // pinned clock this test cannot fail, because the payload it feeds is
  // internally consistent and any implementation reading `iso_week` OR deriving
  // the week from `date` returns W31. The clock is the whole point: it is what
  // makes a canonical derived on THIS side answer W32 and fail.
  it('uses the payload iso_week when tonight falls in the PREVIOUS ISO week', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-08-03T08:30:00Z')) // 01:30 Mon, Phoenix
    fetchSceneDay.mockResolvedValue(
      day({
        date: '2026-08-02',
        iso_week: '2026-W31',
        prev_date: '2026-08-01',
        next_date: '2026-08-03',
      })
    )

    const meta = await buildSceneDayMetadata('phoenix-az')

    expect(meta.alternates?.canonical).toBe(
      'https://psychichomily.com/scenes/phoenix-az/2026-W31'
    )
    // The week a wall clock on this side would have produced at 01:30 Monday.
    expect(meta.alternates?.canonical).not.toContain('2026-W32')
  })

  // Thin content — real, worth serving, worth linking out of, not worth an
  // index entry. `follow` stays ON precisely because the page's job in that
  // state is to point at the week and the rooms.
  it('noindexes a night with nothing on it, but keeps following', async () => {
    fetchSceneDay.mockResolvedValue(day({ shows: [], show_count: 0 }))

    const meta = await buildSceneDayMetadata('phoenix-az')

    expect(meta.robots).toEqual({ index: false, follow: true })
  })

  it('leaves a night that has shows indexable', async () => {
    fetchSceneDay.mockResolvedValue(day())

    const meta = await buildSceneDayMetadata('phoenix-az')

    expect(meta.robots).toBeUndefined()
  })

  // The sibling [period] route contributes an opengraph-image that renders the
  // WEEK card and 404s a date key. Setting images explicitly is what stops Next
  // injecting it — so this assertion is load-bearing, not decorative.
  it('advertises a share image that exists', async () => {
    fetchSceneDay.mockResolvedValue(day())

    const meta = await buildSceneDayMetadata('phoenix-az')

    expect(meta.openGraph?.images).toBeDefined()
    expect(JSON.stringify(meta.openGraph?.images)).toContain('/og-image.jpg')
  })

  it('does not index a scene that does not resolve', async () => {
    fetchSceneDay.mockResolvedValue(null)

    const meta = await buildSceneDayMetadata('nowhere-zz')

    expect(meta.robots).toEqual({ index: false, follow: false })
    expect(meta.alternates?.canonical).toBeUndefined()
  })
})
