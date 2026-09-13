import { beforeEach, describe, expect, it, vi } from 'vitest'

const { fetchSceneWeek } = vi.hoisted(() => ({ fetchSceneWeek: vi.fn() }))
vi.mock('./sceneWeekApi', () => ({ fetchSceneWeek }))

import type { SceneWeekResponse } from './sceneWeek'
import { buildSceneWeekMetadata } from './sceneWeekPage'

const week = (over: Partial<SceneWeekResponse> = {}): SceneWeekResponse =>
  ({
    slug: 'chicago-il',
    scene_name: 'Chicago, IL',
    city: 'Chicago',
    state: 'IL',
    iso_week: '2026-W37',
    start_date: '2026-09-07',
    end_date: '2026-09-13',
    timezone: 'America/Chicago',
    show_count: 4,
    prev_week: '2026-W36',
    next_week: '2026-W38',
    is_current_week: true,
    is_past_week: false,
    days: [{ date: '2026-09-07', shows: [{ id: 1 }, { id: 2 }, { id: 3 }, { id: 4 }] }],
    tracked_venues: [],
    ...over,
  }) as unknown as SceneWeekResponse

/**
 * The `<head>` decisions for the week routes: the title rule, and the
 * route-conditional noindex the day builder has always had and this one did
 * not. Nothing else in the suite reads either, so without this file both can be
 * reworded silently.
 */
describe('buildSceneWeekMetadata', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('titles the current week by the window', async () => {
    fetchSceneWeek.mockResolvedValue(week())

    const meta = await buildSceneWeekMetadata('chicago-il')

    expect(meta.title).toBe('This week in Chicago')
  })

  // A permalink is permanent: a dated week calling itself "this week" is false
  // from the following Monday on, and it is the indexed URL.
  it('titles an archived week by its own Monday', async () => {
    fetchSceneWeek.mockResolvedValue(week({ is_current_week: false, is_past_week: true }))

    const meta = await buildSceneWeekMetadata('chicago-il', '2026-W37')

    expect(meta.title).toBe('Week of Sep 7 in Chicago')
  })

  // `is_current_week` is TRUE for the dated permalink of the week in progress,
  // so a title keyed on the flag rather than on the route would publish "This
  // week in Chicago" at a URL that means one fixed week, and keep saying it,
  // since that URL is what both rolling routes canonicalise to and what the
  // unfurl caches key on.
  it('names the dated permalink of the CURRENT week by its Monday too', async () => {
    fetchSceneWeek.mockResolvedValue(week())

    const meta = await buildSceneWeekMetadata('chicago-il', '2026-W37')

    expect(meta.title).toBe('Week of Sep 7 in Chicago')
  })

  // Thin content: real, worth serving, worth linking out of, not worth an index
  // entry. `follow` stays on because the page's job in that state is to point
  // at the rooms and the neighbouring weeks.
  it('noindexes a dated week with nothing on it, and keeps follow', async () => {
    fetchSceneWeek.mockResolvedValue(
      week({ show_count: 0, days: [], is_current_week: false, is_past_week: true })
    )

    const meta = await buildSceneWeekMetadata('chicago-il', '2026-W37')

    expect(meta.robots).toEqual({ index: false, follow: true })
  })

  it('leaves a dated week that has shows indexable', async () => {
    fetchSceneWeek.mockResolvedValue(week({ is_current_week: false, is_past_week: true }))

    const meta = await buildSceneWeekMetadata('chicago-il', '2026-W37')

    expect(meta.robots).toBeUndefined()
  })

  // The other half of the same contradiction, from the target's end: both
  // rolling routes canonicalise to the CURRENT week's dated permalink, so
  // noindexing it when the week is quiet would consolidate the suppression onto
  // /week and /tonight, a quiet scene's whole discovery surface.
  it('leaves a quiet CURRENT week indexable at its dated permalink', async () => {
    fetchSceneWeek.mockResolvedValue(week({ show_count: 0, days: [] }))

    const meta = await buildSceneWeekMetadata('chicago-il', '2026-W37')

    expect(meta.robots).toBeUndefined()
  })

  // The rolling route declares the DATED permalink as its canonical. A noindex
  // beside a canonical naming a different URL is a contradiction search engines
  // resolve by consolidating the suppression onto the target, which would
  // suppress the archived week along with it.
  it('sets no robots on the rolling route, even when the week is quiet', async () => {
    fetchSceneWeek.mockResolvedValue(week({ show_count: 0, days: [] }))

    const meta = await buildSceneWeekMetadata('chicago-il')

    expect(meta.robots).toBeUndefined()
    expect(meta.alternates?.canonical).toBe(
      'https://psychichomily.com/scenes/chicago-il/2026-W37'
    )
  })

  it('noindexes a week that does not resolve', async () => {
    fetchSceneWeek.mockResolvedValue(null)

    const meta = await buildSceneWeekMetadata('nope-zz', '2026-W37')

    expect(meta.robots).toEqual({ index: false, follow: false })
  })

  // The description still carries the range and the count the title no longer
  // spells, which is what keeps a snippet readable out of context.
  it('keeps the count and the range in the description', async () => {
    fetchSceneWeek.mockResolvedValue(week())

    const meta = await buildSceneWeekMetadata('chicago-il')

    expect(meta.description).toBe(
      '4 shows at the Chicago rooms we track, Mon, Sep 7 – Sun, Sep 13, 2026.'
    )
  })
})
