import { describe, it, expect, vi, beforeEach } from 'vitest'

const fetchSceneDay = vi.fn()
vi.mock('./sceneDayApi', () => ({
  fetchSceneDay: (slug: string, date?: string) => fetchSceneDay(slug, date),
}))

import { fetchSceneSlice } from './sceneSliceApi'
import type { SceneDayResponse } from './sceneDay'

function buildDay(overrides: Partial<SceneDayResponse> = {}): SceneDayResponse {
  return {
    slug: 'phoenix-az',
    scene_name: 'Phoenix, AZ',
    city: 'Phoenix',
    state: 'AZ',
    date: '2026-08-18',
    timezone: 'America/Phoenix',
    iso_week: '2026-W34',
    prev_date: '2026-08-17',
    next_date: '2026-08-19',
    is_tonight: true,
    is_past_day: false,
    show_count: 0,
    shows: [],
    tracked_venues: [],
    ...overrides,
  }
}

/**
 * The sequencing half of the slice — what gets REQUESTED, and in what order.
 * `sceneSlice.test.ts` covers the assembly of whatever comes back.
 */
describe('fetchSceneSlice', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('asks for tonight, then the date tonight names as next', async () => {
    fetchSceneDay
      .mockResolvedValueOnce(buildDay())
      .mockResolvedValueOnce(buildDay({ date: '2026-08-19', is_tonight: false }))

    const slice = await fetchSceneSlice('phoenix-az')

    expect(fetchSceneDay.mock.calls).toEqual([
      ['phoenix-az', undefined],
      ['phoenix-az', '2026-08-19'],
    ])
    expect(slice?.days.map(d => d.date)).toEqual(['2026-08-18', '2026-08-19'])
  })

  // Tonight is requested with the date OMITTED so the backend resolves it
  // against the scene's own 6am boundary. Passing a date computed here would be
  // the mirrored-clock bug this whole surface exists to avoid.
  it('never names the date for tonight', async () => {
    fetchSceneDay.mockResolvedValue(buildDay({ next_date: '' }))
    await fetchSceneSlice('phoenix-az')
    expect(fetchSceneDay).toHaveBeenCalledWith('phoenix-az', undefined)
  })

  // The far edge of the servable window. `fetchScenePeriod` resolves the
  // CURRENT period when its key is falsy, so an unguarded second request would
  // fetch tonight AGAIN and the slice would print the same night twice.
  it('makes no second request when there is no next date', async () => {
    fetchSceneDay.mockResolvedValueOnce(buildDay({ next_date: '' }))

    const slice = await fetchSceneSlice('phoenix-az')

    expect(fetchSceneDay).toHaveBeenCalledTimes(1)
    expect(slice?.days).toHaveLength(1)
  })

  it('gives up on the whole slice when tonight cannot be fetched', async () => {
    fetchSceneDay.mockResolvedValueOnce(null)

    expect(await fetchSceneSlice('phoenix-az')).toBeNull()
    // No point asking for a day whose date only the first payload could name.
    expect(fetchSceneDay).toHaveBeenCalledTimes(1)
  })

  // A blip on the SECOND request costs the second night, not the page.
  it('degrades to one day when the next day fails', async () => {
    fetchSceneDay.mockResolvedValueOnce(buildDay()).mockResolvedValueOnce(null)

    const slice = await fetchSceneSlice('phoenix-az')

    expect(slice?.days).toHaveLength(1)
    expect(slice?.days[0].date).toBe('2026-08-18')
  })
})
