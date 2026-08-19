import { beforeEach, describe, expect, it, vi } from 'vitest'

const { fetchSceneWeek } = vi.hoisted(() => ({ fetchSceneWeek: vi.fn() }))
vi.mock('./sceneWeekApi', () => ({ fetchSceneWeek }))

import { fetchSceneWeekChain } from './sceneWindowApi'
import type { SceneWeekResponse } from './sceneWeek'

const week = (iso: string, next: string | null = null): SceneWeekResponse =>
  ({ iso_week: iso, next_week: next, days: [] }) as unknown as SceneWeekResponse

beforeEach(() => {
  vi.clearAllMocks()
})

describe('fetchSceneWeekChain', () => {
  it('follows next_week rather than computing keys', async () => {
    fetchSceneWeek
      .mockResolvedValueOnce(week('2026-W34', '2026-W35'))
      .mockResolvedValueOnce(week('2026-W35', '2026-W36'))
      .mockResolvedValueOnce(week('2026-W36'))

    const weeks = await fetchSceneWeekChain('phoenix-az', 3)

    expect(weeks?.map(w => w.iso_week)).toEqual(['2026-W34', '2026-W35', '2026-W36'])
    // The first ask carries no key: "current" is the backend's to resolve, in
    // the scene's own timezone.
    expect(fetchSceneWeek).toHaveBeenNthCalledWith(1, 'phoenix-az', undefined, 'scene-week')
    expect(fetchSceneWeek).toHaveBeenNthCalledWith(2, 'phoenix-az', '2026-W35', 'scene-week')
  })

  // The case that must reach `notFound()` rather than rendering an empty city.
  it('is null when the first week fails', async () => {
    fetchSceneWeek.mockResolvedValueOnce(null)
    expect(await fetchSceneWeekChain('nope-zz', 5)).toBeNull()
  })

  // A blip four weeks out is not a reason to 404 a page whose first weeks
  // loaded — the header names the span it actually rendered.
  it('returns the shorter chain when a later week fails', async () => {
    fetchSceneWeek
      .mockResolvedValueOnce(week('2026-W34', '2026-W35'))
      .mockResolvedValueOnce(null)

    const weeks = await fetchSceneWeekChain('phoenix-az', 5)
    expect(weeks?.map(w => w.iso_week)).toEqual(['2026-W34'])
  })

  it('stops when a payload offers no next week', async () => {
    fetchSceneWeek.mockResolvedValueOnce(week('2026-W34', null))
    const weeks = await fetchSceneWeekChain('phoenix-az', 5)
    expect(weeks).toHaveLength(1)
    expect(fetchSceneWeek).toHaveBeenCalledTimes(1)
  })

  // `next_week` is an untrusted wire value. A payload pointing at itself would
  // otherwise repeat its days N times — duplicate dates, duplicate React keys,
  // and the same night printed twice.
  it('stops rather than walking a week that points at itself', async () => {
    fetchSceneWeek.mockResolvedValue(week('2026-W34', '2026-W34'))

    const weeks = await fetchSceneWeekChain('phoenix-az', 5)

    expect(weeks).toHaveLength(1)
    expect(fetchSceneWeek).toHaveBeenCalledTimes(1)
  })

  it('stops rather than re-walking a week already in the chain', async () => {
    fetchSceneWeek
      .mockResolvedValueOnce(week('2026-W34', '2026-W35'))
      .mockResolvedValueOnce(week('2026-W35', '2026-W34'))

    const weeks = await fetchSceneWeekChain('phoenix-az', 5)

    expect(weeks?.map(w => w.iso_week)).toEqual(['2026-W34', '2026-W35'])
    expect(fetchSceneWeek).toHaveBeenCalledTimes(2)
  })
})
