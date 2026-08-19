import { describe, it, expect } from 'vitest'
import { buildSceneSlice, sceneSliceIsQuiet } from './sceneSlice'
import { countWindowShows } from './sceneWindow'
import type { SceneDayResponse } from './sceneDay'
import type { SceneShowSummary } from './types'

function buildShow(overrides: Partial<SceneShowSummary> = {}): SceneShowSummary {
  return {
    id: 1,
    title: 'A show',
    event_date: '2026-08-17',
    starts_at: '2026-08-18T01:00:00Z',
    is_cancelled: false,
    is_sold_out: false,
    venue_timezone: 'America/Chicago',
    venue_state: 'IL',
    venue_city: 'Chicago',
    venue_name: 'Empty Bottle',
    ...overrides,
  }
}

function buildDay(overrides: Partial<SceneDayResponse> = {}): SceneDayResponse {
  return {
    slug: 'chicago-il',
    scene_name: 'Chicago, IL',
    city: 'Chicago',
    state: 'IL',
    date: '2026-08-17',
    timezone: 'America/Chicago',
    iso_week: '2026-W34',
    prev_date: '2026-08-16',
    next_date: '2026-08-18',
    is_tonight: true,
    is_past_day: false,
    show_count: 0,
    shows: [],
    tracked_venues: [],
    ...overrides,
  }
}

describe('buildSceneSlice', () => {
  it('is null when tonight could not be fetched', () => {
    // A failed request is NOT an empty calendar — the caller has to be able to
    // tell them apart, because only one of them is entitled to the honest-zero
    // copy that names a room count in our own voice.
    expect(buildSceneSlice(null, null)).toBeNull()
    expect(buildSceneSlice(null, buildDay())).toBeNull()
  })

  it('carries both whole dates when the next day resolves', () => {
    const slice = buildSceneSlice(
      buildDay({ shows: [buildShow({ id: 1 })] }),
      buildDay({
        date: '2026-08-18',
        is_tonight: false,
        shows: [buildShow({ id: 2 }), buildShow({ id: 3 })],
      })
    )

    expect(slice?.days.map(d => d.date)).toEqual(['2026-08-17', '2026-08-18'])
    expect(slice?.days.map(d => d.is_tonight)).toEqual([true, false])
    expect(countWindowShows(slice!.days)).toBe(3)
  })

  it('degrades to one day rather than fabricating a second', () => {
    // The far edge of the servable window: the backend sends an empty
    // `next_date` and the caller makes no second request.
    const slice = buildSceneSlice(buildDay({ next_date: '' }), null)
    expect(slice?.days).toHaveLength(1)
    expect(slice?.days[0].date).toBe('2026-08-17')
  })

  it('never prints the same night twice', () => {
    // The trap this guards, and the reason the guard is on the DATE rather than
    // on the payload being present: `fetchScenePeriod` resolves the CURRENT
    // period when its key is falsy, so a caller that passed an empty
    // `next_date` through would get tonight back a second time. Two identical
    // headings, one of them lying about which night it describes.
    const tonight = buildDay({ shows: [buildShow()] })
    const slice = buildSceneSlice(tonight, buildDay({ shows: [buildShow()] }))

    expect(slice?.days).toHaveLength(1)
    expect(countWindowShows(slice!.days)).toBe(1)
  })

  it('takes tonight from the payload, never from a clock', () => {
    // The backend owns the 6am night boundary. At 01:00 the current night is
    // still YESTERDAY's date, and a slice that recomputed it here would
    // disagree with /tonight for the same instant.
    const slice = buildSceneSlice(
      buildDay({ date: '2026-08-16', is_tonight: true, next_date: '2026-08-17' }),
      buildDay({ date: '2026-08-17', is_tonight: false })
    )

    expect(slice?.days[0]).toMatchObject({ date: '2026-08-16', is_tonight: true })
    expect(slice?.days[1]).toMatchObject({ date: '2026-08-17', is_tonight: false })
  })

  it('names the scene zone from the backend, even with nothing booked', () => {
    // The old row-derived resolution voted over fetched shows, so a scene with
    // an empty window could not name its own clock at all.
    const slice = buildSceneSlice(buildDay({ shows: [] }), null)
    expect(slice?.timezone).toBe('America/Chicago')
  })

  it('reports no zone rather than an empty one', () => {
    const slice = buildSceneSlice(buildDay({ timezone: '' }), null)
    expect(slice?.timezone).toBeUndefined()
  })

  it('treats a null shows array as no shows', () => {
    // `shows` is typed nullable by the generator even though the API always
    // emits an array.
    const slice = buildSceneSlice(buildDay({ shows: null }), null)
    expect(countWindowShows(slice!.days)).toBe(0)
  })
})

describe('sceneSliceIsQuiet', () => {
  it('is quiet when every day it answered for is empty', () => {
    const slice = buildSceneSlice(
      buildDay({ shows: [] }),
      buildDay({ date: '2026-08-18', is_tonight: false, shows: [] })
    )
    expect(sceneSliceIsQuiet(slice!)).toBe(true)
  })

  it('is not quiet when only one of the two nights has a show', () => {
    const slice = buildSceneSlice(
      buildDay({ shows: [] }),
      buildDay({ date: '2026-08-18', is_tonight: false, shows: [buildShow()] })
    )
    expect(sceneSliceIsQuiet(slice!)).toBe(false)
  })
})
