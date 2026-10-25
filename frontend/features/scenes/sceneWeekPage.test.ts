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
 * The `<head>` decisions for the week routes: the title and description rule,
 * the self-canonical on both routes with the share identity and card held on
 * the dated permalink, and the route-conditional noindex. Nothing else in the
 * suite reads them, so without this file each can be reworded silently.
 */
describe('buildSceneWeekMetadata', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('titles the current week by the window', async () => {
    fetchSceneWeek.mockResolvedValue(week())

    const meta = await buildSceneWeekMetadata('chicago-il')

    expect(meta.title).toBe('Live shows in Chicago this week')
  })

  // A permalink is permanent: a dated week calling itself "this week" is false
  // from the following Monday on, and it is the indexed URL.
  it('titles an archived week by its own Monday', async () => {
    fetchSceneWeek.mockResolvedValue(week({ is_current_week: false, is_past_week: true }))

    const meta = await buildSceneWeekMetadata('chicago-il', '2026-W37')

    expect(meta.title).toBe('Week of Sep 7 in Chicago')
  })

  // `is_current_week` is TRUE for the dated permalink of the week in progress,
  // so a title keyed on the flag rather than on the route would publish "this
  // week" at a URL that means one fixed week, and keep saying it, since that
  // URL is what the unfurl caches key on.
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

  // The current week's dated permalink follows the same rule as every other
  // dated week: no rolling route names it as a canonical, and the sitemap never
  // announces a week with no show.
  it('noindexes a quiet CURRENT week at its dated permalink', async () => {
    fetchSceneWeek.mockResolvedValue(week({ show_count: 0, days: [] }))

    const meta = await buildSceneWeekMetadata('chicago-il', '2026-W37')

    expect(meta.robots).toEqual({ index: false, follow: true })
    expect(meta.alternates?.canonical).toBe(
      'https://psychichomily.com/scenes/chicago-il/2026-W37'
    )
  })

  // /week is announced in the sitemap every day; a quiet week's noindex would
  // drop the URL from the index until a crawler next saw a busy one.
  it('keeps a quiet /week indexable and self-canonical', async () => {
    fetchSceneWeek.mockResolvedValue(week({ show_count: 0, days: [] }))

    const meta = await buildSceneWeekMetadata('chicago-il')

    expect(meta.robots).toBeUndefined()
    expect(meta.alternates?.canonical).toBe('https://psychichomily.com/scenes/chicago-il/week')
  })

  it('makes the rolling /week route its own canonical', async () => {
    fetchSceneWeek.mockResolvedValue(week())

    const meta = await buildSceneWeekMetadata('chicago-il')

    expect(meta.alternates?.canonical).toBe('https://psychichomily.com/scenes/chicago-il/week')
  })

  it('keeps the dated permalink its own canonical', async () => {
    fetchSceneWeek.mockResolvedValue(week())

    const meta = await buildSceneWeekMetadata('chicago-il', '2026-W37')

    expect(meta.alternates?.canonical).toBe(
      'https://psychichomily.com/scenes/chicago-il/2026-W37'
    )
  })

  // og:url and the card stay on the dated permalink on BOTH routes. The rolling
  // URL's own image would be a constant URL, pinned in unfurl caches to
  // whichever week a scraper saw first, and its own og:url would make every
  // week one cached share object.
  it('keeps og:url and the card on the dated permalink from the rolling route', async () => {
    fetchSceneWeek.mockResolvedValue(week())

    const meta = await buildSceneWeekMetadata('chicago-il')
    const images = meta.openGraph?.images
    const image = Array.isArray(images) ? images[0] : images

    expect(meta.openGraph?.url).toBe('https://psychichomily.com/scenes/chicago-il/2026-W37')
    expect(image).toMatchObject({
      url: 'https://psychichomily.com/scenes/chicago-il/2026-W37/opengraph-image',
    })
  })

  it('noindexes a week that does not resolve', async () => {
    fetchSceneWeek.mockResolvedValue(null)

    const meta = await buildSceneWeekMetadata('nope-zz', '2026-W37')

    expect(meta.robots).toEqual({ index: false, follow: false })
  })

  // The snippet opens with the title phrase and still carries the range and
  // the count the title does not spell, which keeps it readable out of context.
  it('opens the rolling description with the title phrase, then the count and the range', async () => {
    fetchSceneWeek.mockResolvedValue(week())

    const meta = await buildSceneWeekMetadata('chicago-il')

    expect(meta.description).toBe(
      'Live shows in Chicago this week: 4 shows at the Chicago rooms we track, Mon, Sep 7 – Sun, Sep 13, 2026.'
    )
  })

  it('states a quiet rolling week without speaking for the city', async () => {
    fetchSceneWeek.mockResolvedValue(week({ show_count: 0, days: [] }))

    const meta = await buildSceneWeekMetadata('chicago-il')

    expect(meta.description).toBe(
      'Live shows in Chicago this week: no shows at the Chicago rooms we track, Mon, Sep 7 – Sun, Sep 13, 2026.'
    )
  })

  it('keeps the dated description to the count and the range', async () => {
    fetchSceneWeek.mockResolvedValue(week())

    const meta = await buildSceneWeekMetadata('chicago-il', '2026-W37')

    expect(meta.description).toBe(
      '4 shows at the Chicago rooms we track, Mon, Sep 7 – Sun, Sep 13, 2026.'
    )
  })
})
