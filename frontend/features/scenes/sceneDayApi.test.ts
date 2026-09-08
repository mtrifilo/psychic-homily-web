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
import type { SceneDayResponse } from './sceneDay'
import { fetchSceneDay } from './sceneDayApi'
import {
  ARCHIVED_PERIOD_REVALIDATE,
  CURRENT_PERIOD_REVALIDATE,
} from './scenePeriodApi'

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
    shows: [],
    tracked_venues: [],
    ...over,
  }) as SceneDayResponse

const jsonResponse = (body: unknown) =>
  new Response(JSON.stringify(body), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  })

const DATED_URL = `${API_BASE_URL}/scenes/phoenix-az/day/2026-07-31`
const ROLLING_URL = `${API_BASE_URL}/scenes/phoenix-az/day`

type NextInit = { next: { revalidate: number } }

/** The `revalidate` each call asked for, in order. */
const windowsRequested = (fetchMock: ReturnType<typeof vi.fn>): number[] =>
  fetchMock.mock.calls.map(([, init]) => (init as NextInit).next.revalidate)

describe('fetchSceneDay', () => {
  let fetchMock: ReturnType<typeof vi.fn>

  beforeEach(() => {
    vi.clearAllMocks()
    fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  // No date means "whatever night is live", which can still change by
  // definition — there is nothing to probe for, so it stays one request. The
  // URL matters as much as the window: this is the path `/tonight` renders from
  // and the one the proxy HEADs.
  it('asks the rolling endpoint once, on the short window, when no date is given', async () => {
    fetchMock.mockResolvedValue(jsonResponse(day()))

    await fetchSceneDay('phoenix-az')

    expect(windowsRequested(fetchMock)).toEqual([CURRENT_PERIOD_REVALIDATE])
    expect(fetchMock.mock.calls.map(([url]) => url)).toEqual([ROLLING_URL])
  })

  // Tonight's DATED permalink is a real URL people reach and the breadcrumb
  // leaf points at, so while it is still tonight it must not be a day staler
  // than /tonight, which serves the same content.
  it('gives a night that has not ended the short window, even when asked for by date', async () => {
    fetchMock.mockResolvedValue(jsonResponse(day({ is_past_day: false })))

    await expect(fetchSceneDay('phoenix-az', '2026-07-31')).resolves.toMatchObject({
      date: '2026-07-31',
    })

    expect(windowsRequested(fetchMock)).toEqual([
      ARCHIVED_PERIOD_REVALIDATE,
      CURRENT_PERIOD_REVALIDATE,
    ])
    expect(fetchMock.mock.calls.map(([url]) => url)).toEqual([DATED_URL, DATED_URL])
  })

  // A night that is over cannot gain shows, so it costs exactly one request on
  // the long window — which is what keeps a crawler walking the archive cheap.
  it('leaves a night that is over on the long window, in one request', async () => {
    fetchMock.mockResolvedValue(jsonResponse(day({ is_past_day: true })))

    await fetchSceneDay('phoenix-az', '2026-07-30')

    expect(windowsRequested(fetchMock)).toEqual([ARCHIVED_PERIOD_REVALIDATE])
  })

  // A newer frontend can be live before the backend that sends the field — both
  // auto-deploy, and they do not land together. Absent must read as "might still
  // change" so the window errs fresh and heals itself.
  it('keeps the short window when the backend does not send the flag', async () => {
    const withoutFlag: Record<string, unknown> = { ...day() }
    delete withoutFlag.is_past_day
    fetchMock.mockResolvedValue(jsonResponse(withoutFlag))

    await fetchSceneDay('phoenix-az', '2026-07-31')

    expect(windowsRequested(fetchMock)).toEqual([
      ARCHIVED_PERIOD_REVALIDATE,
      CURRENT_PERIOD_REVALIDATE,
    ])
  })

  // The wire is not trusted. A body whose flag is not a boolean must not be
  // allowed to freeze tonight's page in the CDN for a day.
  it('refuses a non-boolean flag as grounds for the long window', async () => {
    fetchMock.mockResolvedValue(jsonResponse({ ...day(), is_past_day: 'yes' }))

    await fetchSceneDay('phoenix-az', '2026-07-31')

    expect(windowsRequested(fetchMock)).toEqual([
      ARCHIVED_PERIOD_REVALIDATE,
      CURRENT_PERIOD_REVALIDATE,
    ])
  })

  // A 200 is not proof of the right endpoint: a redirect or a CDN error page can
  // answer 200 with something else, and `date` goes straight into date maths.
  it('rejects a 200 body that is not a day payload', async () => {
    fetchMock.mockResolvedValue(jsonResponse({ hello: 'world' }))

    await expect(fetchSceneDay('phoenix-az', '2026-07-31')).resolves.toBeNull()
  })

  // 404 is the normal answer for an unknown slug or an impossible date. It must
  // not be probed a second time, and it is not worth reporting.
  it('stops at the first ask when the day does not exist', async () => {
    fetchMock.mockResolvedValue(new Response(null, { status: 404 }))

    await expect(fetchSceneDay('phoenix-az', '2026-02-30')).resolves.toBeNull()
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(captureMessage).not.toHaveBeenCalled()
  })

  // A 5xx IS worth reporting, tagged so triage can tell the day surface from
  // the week one.
  // The tag says which surface; the slug says which scene, and that is the
  // first thing triage needs. It lives inside `buildUrl`'s closure, so it has
  // to be carried explicitly or it silently vanishes from every report.
  it('reports a 5xx with the service tag AND the scene', async () => {
    fetchMock.mockResolvedValue(new Response(null, { status: 503 }))

    await expect(fetchSceneDay('phoenix-az')).resolves.toBeNull()
    expect(captureMessage).toHaveBeenCalledWith(
      'Scene day: API returned 503',
      expect.objectContaining({
        tags: { service: 'scene-day' },
        extra: expect.objectContaining({ slug: 'phoenix-az', status: 503 }),
      })
    )
  })

  // A body missing any of these is not this payload, whichever list the field
  // belongs to.
  it.each(['date', 'prev_date', 'next_date', 'city', 'slug', 'iso_week'])(
    'rejects a body missing %s',
    async field => {
      const thin: Record<string, unknown> = { ...day() }
      delete thin[field]
      fetchMock.mockResolvedValue(jsonResponse(thin))

      await expect(fetchSceneDay('phoenix-az')).resolves.toBeNull()
    }
  )

  // An identity field carrying no text reaches a URL as `/scenes//`, so it
  // fails here rather than there.
  it.each(
    ['date', 'city', 'slug', 'iso_week'].flatMap(field =>
      ['', '   '].map(bad => [field, bad] as const)
    )
  )('rejects a body whose %s is %j', async (field, bad) => {
    fetchMock.mockResolvedValue(jsonResponse({ ...day(), [field]: bad }))

    await expect(fetchSceneDay('phoenix-az')).resolves.toBeNull()
  })

  // An untrimmed value on an ADDRESSED field names a page that does not exist,
  // and it is truthy at every consumer. Each of these fields refuses it through
  // its own shape rule; `city` is printed and is deliberately absent from this
  // list.
  it.each(
    ['date', 'slug', 'iso_week'].flatMap(field =>
      [' 2026-07-31', '2026-07-31 '].map(bad => [field, bad] as const)
    )
  )('rejects a body whose %s is untrimmed (%j)', async (field, bad) => {
    fetchMock.mockResolvedValue(jsonResponse({ ...day(), [field]: bad }))

    await expect(fetchSceneDay('phoenix-az')).resolves.toBeNull()
  })

  // Emptiness is the answer at the edges of the servable window, so it must not
  // fail the payload. This is the case that keeps the first and last servable
  // days of every scene reachable.
  it.each([
    ['prev_date', { prev_date: '' }],
    ['next_date', { next_date: '' }],
    ['both', { prev_date: '', next_date: '' }],
  ])('serves a day with an empty %s', async (_label, over) => {
    fetchMock.mockResolvedValue(jsonResponse(day(over)))

    await expect(fetchSceneDay('phoenix-az')).resolves.toMatchObject(over)
  })

  // Blank is not empty. A presence field carrying spaces is truthy at every
  // consumer, so it would be read as "there IS a neighbour" and build a link to
  // an address made of whitespace.
  it.each([
    ['prev_date', { prev_date: '   ' }],
    ['next_date', { next_date: ' 2026-08-01' }],
  ])('rejects a body whose %s is blank rather than empty', async (_label, over) => {
    fetchMock.mockResolvedValue(jsonResponse(day(over)))

    await expect(fetchSceneDay('phoenix-az')).resolves.toBeNull()
  })

  // Naming something is not naming the right KIND of thing. Each of these
  // survives every truthiness and trim check and then reaches a URL
  // interpolation that has no second chance to look at it.
  it.each([
    ['a date that is a word', { date: 'tonight' }],
    ['a date with no day', { date: '2026-07' }],
    ['a week key that is not one', { iso_week: '2026-31' }],
    ['a week key with no week', { iso_week: '2026-W' }],
    ['a slug that walks up the path', { slug: '../..' }],
    ['a slug carrying a query', { slug: 'phoenix-az?x' }],
    ['a slug that is the index', { slug: '.' }],
  ])('rejects a body carrying %s', async (_label, over) => {
    fetchMock.mockResolvedValue(jsonResponse(day(over)))

    await expect(fetchSceneDay('phoenix-az')).resolves.toBeNull()
  })

  // The shapes a US scene slug legitimately takes. `lower(replace(city,' ','-'))`
  // keeps whatever punctuation and accents the city name carries, and losing
  // those pages would be a worse bug than the one the rule above closes.
  it.each([['st.-louis-mo'], ['española-nm'], ["coeur-d'alene-id"]])(
    'serves a day whose slug is %s',
    async slug => {
      fetchMock.mockResolvedValue(jsonResponse(day({ slug })))

      await expect(fetchSceneDay('phoenix-az')).resolves.toMatchObject({ slug })
    }
  )

  // `date` is checked for shape and not for the day route's year bounds: the
  // route already applies those to the segment it serves, and importing the
  // bounded rule here would pull the day formatting stack into the edge-runtime
  // share card.
  it('serves a well-formed date outside the day route year bounds', async () => {
    fetchMock.mockResolvedValue(jsonResponse(day({ date: '1998-07-31' })))

    await expect(fetchSceneDay('phoenix-az')).resolves.toMatchObject({
      date: '1998-07-31',
    })
  })

  // A day's week key is NOT bounded by the day's own year: a Monday, Tuesday or
  // Wednesday 31 December opens week 01 of the following year, so the last day
  // this route serves names a week the WEEK route refuses. Bounding it here
  // would 404 that day. The view drops the one link it cannot offer instead.
  it('serves a day whose week key is past the week route horizon', async () => {
    const beyond = `${new Date().getUTCFullYear() + 2}-W01`
    fetchMock.mockResolvedValue(jsonResponse(day({ iso_week: beyond })))

    await expect(fetchSceneDay('phoenix-az')).resolves.toMatchObject({
      iso_week: beyond,
    })
  })

  // `venues.city` carries untrimmed values (see the note on `buildSceneSlug` in
  // catalog/charts_service.go), and the display city is MIN(city) over the
  // group. `city` is printed and addresses nothing, so surrounding space must
  // not cost the scene its page.
  it('serves a day whose printed city carries surrounding space', async () => {
    fetchMock.mockResolvedValue(jsonResponse(day({ city: ' Phoenix' })))

    await expect(fetchSceneDay('phoenix-az')).resolves.toMatchObject({
      city: ' Phoenix',
    })
  })

  // A rejected 200 is otherwise invisible: no status check fires, and the body
  // stays cached for the caller's whole window.
  it('reports which field a rejected payload failed on', async () => {
    fetchMock.mockResolvedValue(jsonResponse({ ...day(), slug: '' }))

    await expect(fetchSceneDay('phoenix-az')).resolves.toBeNull()
    expect(captureMessage).toHaveBeenCalledWith(
      'Scene day: rejected a payload on `slug`',
      expect.objectContaining({
        tags: { service: 'scene-day' },
        extra: expect.objectContaining({ slug: 'phoenix-az', field: 'slug' }),
      })
    )
  })

  // Next decodes route params before this sees them, so an unescaped slug would
  // truncate the path at a `?` or `#` and hit a different backend endpoint.
  it('encodes both segments on every ask', async () => {
    fetchMock.mockResolvedValue(jsonResponse(day({ is_past_day: false })))

    await fetchSceneDay('phoenix-az?x', '2026-07-31#y')

    for (const [url] of fetchMock.mock.calls) {
      expect(url).toBe(`${API_BASE_URL}/scenes/phoenix-az%3Fx/day/2026-07-31%23y`)
    }
  })

  // The day demonstrably exists — the first ask returned it. A blip on the
  // second must not turn a real page into a 404.
  it('falls back to the first answer when the short-window ask fails', async () => {
    fetchMock
      .mockResolvedValueOnce(jsonResponse(day({ show_count: 9 })))
      .mockResolvedValueOnce(new Response(null, { status: 503 }))

    await expect(fetchSceneDay('phoenix-az', '2026-07-31')).resolves.toMatchObject({
      show_count: 9,
    })
  })

  // A network error must not take the page down; it becomes the ordinary "no
  // data" path, reported for triage.
  it('reports a network failure and resolves null', async () => {
    fetchMock.mockRejectedValue(new Error('ECONNREFUSED'))

    await expect(fetchSceneDay('phoenix-az')).resolves.toBeNull()
    expect(captureException).toHaveBeenCalled()
  })
})
