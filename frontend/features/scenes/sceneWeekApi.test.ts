import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const { captureException, captureMessage } = vi.hoisted(() => ({
  captureException: vi.fn(),
  captureMessage: vi.fn(),
}))

vi.mock('@sentry/nextjs', () => ({
  captureException,
  captureMessage,
}))

import { API_BASE_URL } from '@/lib/api-base'
import type { SceneWeekResponse } from './sceneWeek'
import { fetchSceneWeek } from './sceneWeekApi'
import {
  ARCHIVED_PERIOD_REVALIDATE,
  CURRENT_PERIOD_REVALIDATE,
} from './scenePeriodApi'

const week = (over: Partial<SceneWeekResponse> = {}): SceneWeekResponse =>
  ({
    slug: 'chicago-il',
    scene_name: 'Chicago, IL',
    city: 'Chicago',
    state: 'IL',
    iso_week: '2026-W31',
    start_date: '2026-07-27',
    end_date: '2026-08-02',
    timezone: 'America/Chicago',
    show_count: 12,
    prev_week: '2026-W30',
    next_week: '2026-W32',
    is_current_week: true,
    is_past_week: false,
    days: [],
    tracked_venues: [],
    ...over,
  }) as SceneWeekResponse

const jsonResponse = (body: unknown) =>
  new Response(JSON.stringify(body), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  })

const KEYED_URL = `${API_BASE_URL}/scenes/chicago-il/week/2026-W31`
const ROLLING_URL = `${API_BASE_URL}/scenes/chicago-il/week`

type NextInit = { next: { revalidate: number } }

/** The `revalidate` each call asked for, in order. */
const windowsRequested = (fetchMock: ReturnType<typeof vi.fn>): number[] =>
  fetchMock.mock.calls.map(([, init]) => (init as NextInit).next.revalidate)

describe('fetchSceneWeek', () => {
  let fetchMock: ReturnType<typeof vi.fn>

  beforeEach(() => {
    vi.clearAllMocks()
    fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  // The whole point of PSY-1639. `/scenes/{slug}/2026-W31` is the CANONICAL url
  // — search engines index it and shares resolve to it — so during W31 it must
  // not be a day staler than `/scenes/{slug}/week`, which serves the same
  // content. Keying the window on "was a key supplied" is what made it so.
  it('gives the CURRENT week the short window even when asked for by key', async () => {
    fetchMock.mockResolvedValue(jsonResponse(week({ is_current_week: true, is_past_week: false })))

    await expect(fetchSceneWeek('chicago-il', '2026-W31', 'scene-week')).resolves.toMatchObject({
      iso_week: '2026-W31',
    })

    expect(windowsRequested(fetchMock)).toEqual([
      ARCHIVED_PERIOD_REVALIDATE,
      CURRENT_PERIOD_REVALIDATE,
    ])
    expect(fetchMock.mock.calls.map(([url]) => url)).toEqual([KEYED_URL, KEYED_URL])
  })

  // The constraint that made the earlier fix stop short: a week that genuinely
  // cannot change any more must not drop to 900s. It costs exactly one request,
  // on the long window.
  it('leaves a genuinely past week on the long window, in one request', async () => {
    fetchMock.mockResolvedValue(jsonResponse(week({ is_current_week: false, is_past_week: true })))

    await expect(fetchSceneWeek('chicago-il', '2026-W31', 'scene-week')).resolves.toMatchObject({
      is_past_week: true,
    })

    expect(windowsRequested(fetchMock)).toEqual([ARCHIVED_PERIOD_REVALIDATE])
  })

  // A future week is neither current nor past. It is the one that must never be
  // frozen: "next week →" is on every page, so it gets fetched days before it
  // goes live and the rolling page then points at exactly that URL.
  it('keeps a future week on the short window', async () => {
    fetchMock.mockResolvedValue(jsonResponse(week({ is_current_week: false, is_past_week: false })))

    await fetchSceneWeek('chicago-il', '2026-W35', 'scene-week')

    expect(windowsRequested(fetchMock)).toEqual([
      ARCHIVED_PERIOD_REVALIDATE,
      CURRENT_PERIOD_REVALIDATE,
    ])
  })

  // No key means "whatever week is live", which can still change by definition.
  // There is nothing to probe for, so it stays one request.
  it('asks the rolling endpoint once, on the short window, when no key is given', async () => {
    fetchMock.mockResolvedValue(jsonResponse(week()))

    await fetchSceneWeek('chicago-il', undefined, 'scene-week')

    expect(windowsRequested(fetchMock)).toEqual([CURRENT_PERIOD_REVALIDATE])
    expect(fetchMock.mock.calls.map(([url]) => url)).toEqual([ROLLING_URL])
  })

  // The week demonstrably exists — the first ask returned it. A blip on the
  // second must not turn a real page into a 404.
  it('falls back to the first answer when the short-window ask fails', async () => {
    fetchMock
      .mockResolvedValueOnce(jsonResponse(week({ show_count: 9 })))
      .mockResolvedValueOnce(new Response(null, { status: 503 }))

    await expect(fetchSceneWeek('chicago-il', '2026-W31', 'scene-week')).resolves.toMatchObject({
      show_count: 9,
    })
    // The slug is the first thing triage needs and the one thing the week key
    // cannot say. It lives inside the URL builder's closure, so it has to be
    // carried explicitly or it silently vanishes from every report.
    expect(captureMessage).toHaveBeenCalledWith(
      'Scene week: API returned 503',
      expect.objectContaining({
        tags: { service: 'scene-week' },
        extra: expect.objectContaining({ slug: 'chicago-il', status: 503 }),
      })
    )
  })

  // A newer frontend can be live before the backend that sends the field —
  // both auto-deploy, and they do not land together. Absent must read as
  // "might still change" so the window errs fresh and heals itself, rather
  // than freezing whatever week is live for a day.
  it('keeps the short window when the backend does not send the flag', async () => {
    const withoutFlag: Record<string, unknown> = { ...week({ is_current_week: true }) }
    delete withoutFlag.is_past_week
    fetchMock.mockResolvedValue(jsonResponse(withoutFlag))

    await fetchSceneWeek('chicago-il', '2026-W31', 'scene-week')

    expect(windowsRequested(fetchMock)).toEqual([
      ARCHIVED_PERIOD_REVALIDATE,
      CURRENT_PERIOD_REVALIDATE,
    ])
  })

  // The wire is not trusted — that is why `asSceneWeek` exists. A body whose
  // flag is not a boolean must not be allowed to freeze a live week.
  it('refuses a non-boolean flag as grounds for the long window', async () => {
    fetchMock.mockResolvedValue(jsonResponse({ ...week(), is_past_week: 'yes' }))

    await fetchSceneWeek('chicago-il', '2026-W31', 'scene-week')

    expect(windowsRequested(fetchMock)).toEqual([
      ARCHIVED_PERIOD_REVALIDATE,
      CURRENT_PERIOD_REVALIDATE,
    ])
  })

  // A 404 is the normal answer for an unknown slug or a week key that does not
  // exist. It must not be probed a second time.
  it('stops at the first ask when the week does not exist', async () => {
    fetchMock.mockResolvedValue(new Response(null, { status: 404 }))

    await expect(fetchSceneWeek('nowhere-zz', '2026-W31', 'scene-week')).resolves.toBeNull()
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(captureMessage).not.toHaveBeenCalled()
  })

  // Next decodes route params before this sees them, so an unescaped slug would
  // truncate the path at a `?` or `#` and hit a different backend endpoint.
  it('encodes both segments on every ask', async () => {
    fetchMock.mockResolvedValue(jsonResponse(week({ is_past_week: false })))

    await fetchSceneWeek('chicago-il?x', '2026-W31#y', 'og-image')

    for (const [url] of fetchMock.mock.calls) {
      expect(url).toBe(`${API_BASE_URL}/scenes/chicago-il%3Fx/week/2026-W31%23y`)
    }
  })
})
