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
 * query-language title, the canonical pointing at the dated permalink, and the
 * noindex on a night with nothing on it. Nothing else in the suite touches
 * them, so without this file any of the three can be reworded silently.
 */
describe('buildSceneDayMetadata', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    vi.restoreAllMocks()
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

  // The rolling URL's content changes every night, so pointing search engines
  // at it would leave every indexed snippet describing a night that has passed.
  it('points the canonical at the DATED permalink from both routes', async () => {
    fetchSceneDay.mockResolvedValue(day())
    const rolling = await buildSceneDayMetadata('phoenix-az')

    fetchSceneDay.mockResolvedValue(day({ is_tonight: false }))
    const dated = await buildSceneDayMetadata('phoenix-az', '2026-07-31')

    for (const meta of [rolling, dated]) {
      expect(meta.alternates?.canonical).toBe(
        'https://psychichomily.com/scenes/phoenix-az/2026-07-31'
      )
    }
  })

  // Thin content — real, worth serving, worth linking out of, not worth an
  // index entry. `follow` stays ON precisely because the page's job in that
  // state is to point at the week and the rooms.
  it('noindexes a night with nothing on it, but keeps following', async () => {
    fetchSceneDay.mockResolvedValue(day({ shows: [], show_count: 0 }))

    const meta = await buildSceneDayMetadata('phoenix-az')

    expect(meta.robots).toEqual({ index: false, follow: true })
    // The canonical still points at the permalink, so the noindex applies to
    // one URL rather than orphaning the date.
    expect(meta.alternates?.canonical).toBe(
      'https://psychichomily.com/scenes/phoenix-az/2026-07-31'
    )
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
