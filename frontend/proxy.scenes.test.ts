import { afterEach, describe, expect, it, vi } from 'vitest'
import type { NextRequest } from 'next/server'
import { proxy } from './proxy'
import { looksLikeISOWeek } from '@/features/scenes/sceneWeek'
import { looksLikeCalendarDate } from '@/features/scenes/sceneDay'

function requestFor(pathname: string): NextRequest {
  return {
    nextUrl: new URL(`http://localhost:3000${pathname}`),
    url: `http://localhost:3000${pathname}`,
  } as unknown as NextRequest
}

function mockBackend(status: number) {
  return vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(null, { status }))
}

/**
 * The year window both copies of the period-shape rule apply. Restated here
 * rather than imported, so the assertions below are pinned to the INTENT and
 * would fail if BOTH copies drifted together — importing the constant would
 * make the test agree with whatever the code happens to say.
 */
const FIRST_TRACKED_YEAR = 2015
const LAST_SERVABLE_YEAR = new Date().getUTCFullYear() + 1

/**
 * The scene period routes sit one level BELOW the entity-detail shape the
 * generic check handles, so each needs its own branch here or it soft-404s —
 * a 404 BODY committed at HTTP 200, because the shell has already streamed
 * (PSY-897 arc, and again for the week routes in PSY-1577).
 */
describe('proxy — scene period routes', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  // Both DAY routes probe the cheap scene-existence endpoint: `/tonight` has no
  // key, and a calendar date's validity is settled locally, so rebuilding the
  // whole night just to discard the body would be pure waste — and the dated
  // route is the one with thousands of keys per scene. The WEEK key still goes
  // to its own endpoint, because only the backend owns the ISO-8601 arithmetic
  // that decides `2025-W53` is unreal.
  it.each([
    ['/scenes/phoenix-az/week', 'http://localhost:8080/scenes/phoenix-az/week'],
    ['/scenes/phoenix-az/tonight', 'http://localhost:8080/entities/scenes/phoenix-az/exists'],
    [
      '/scenes/phoenix-az/opengraph-image',
      'http://localhost:8080/entities/scenes/phoenix-az/exists',
    ],
    ['/scenes/phoenix-az/2026-W31', 'http://localhost:8080/scenes/phoenix-az/week/2026-W31'],
    ['/scenes/phoenix-az/2026-07-31', 'http://localhost:8080/entities/scenes/phoenix-az/exists'],
  ])('existence-checks %s against %s', async (path, expected) => {
    const fetchMock = mockBackend(200)

    const response = await proxy(requestFor(path))

    expect(fetchMock).toHaveBeenCalledWith(
      expected,
      expect.objectContaining({ method: 'HEAD', redirect: 'manual' })
    )
    expect(response.status).toBe(200)
  })

  // `2025-W53` is well-formed and unreal, and only the backend owns the ISO
  // week arithmetic that says so.
  it('turns a backend 404 for a nonexistent week into a real 404', async () => {
    mockBackend(404)
    const response = await proxy(requestFor('/scenes/phoenix-az/2025-W53'))
    expect(response.status).toBe(404)
  })

  // An impossible DATE is settled here, without a round-trip. February never
  // has 30 days in any timezone, so this needs no backend — and refusing it
  // locally is what lets the dated route use the cheap scene probe at all.
  it.each([
    '2026-02-30', // February never has 30 days
    '2027-02-29', // 2027 is not a leap year
    '2026-13-01', // month out of range
    '2026-07-32', // day out of range
    '2026-00-10', // month zero
    '2026-07-00', // day zero
  ])('404s the impossible date %s without a backend round-trip', async segment => {
    const fetchMock = mockBackend(200)

    const response = await proxy(requestFor(`/scenes/phoenix-az/${segment}`))

    expect(response.status).toBe(404)
    expect(fetchMock).not.toHaveBeenCalled()
  })

  // A leap day inside the window — the check must reject impossible dates
  // without rejecting the unusual real ones. 2024 stays in range for as long
  // as this code will plausibly live, since the window's floor is fixed.
  it('still serves a real leap day', async () => {
    const fetchMock = mockBackend(200)

    const response = await proxy(requestFor('/scenes/phoenix-az/2024-02-29'))

    expect(response.status).toBe(200)
    expect(fetchMock).toHaveBeenCalled()
  })

  // The scene can still be missing, and that answer still comes from the
  // backend — the local date check replaces the day-payload probe, not the
  // existence check.
  it('turns a backend 404 for the scene into a real 404 on a dated permalink', async () => {
    mockBackend(404)
    const response = await proxy(requestFor('/scenes/nowhere-zz/2026-07-31'))
    expect(response.status).toBe(404)
  })

  it('404s a junk period segment without a backend round-trip', async () => {
    const fetchMock = mockBackend(200)

    const response = await proxy(requestFor('/scenes/phoenix-az/garbage'))

    expect(response.status).toBe(404)
    expect(fetchMock).not.toHaveBeenCalled()
  })

  /**
   * The proxy keeps its OWN copy of the period-shape rule — it stays free of
   * `features/` imports, matching the charts branch — so the two copies are
   * pinned together here instead. They must agree in BOTH directions:
   *
   * - proxy accepts, page rejects → the page's `notFound()` fires after the
   *   shell has streamed, committing a 404 body at HTTP 200. That is the exact
   *   soft-404 this branch exists to prevent, and it is silent.
   * - proxy rejects, page accepts → a real page 404s and is unreachable.
   *
   * The out-of-range years are the cases that actually diverged: the backend
   * happily serves `1998-W12`, so only this bound stops it from rendering a
   * not-found page at HTTP 200.
   */
  it.each([
    // The BOUNDARIES, computed rather than written down — a fixture list of
    // 1998 and 2400 would still pass if one copy drifted to 2010 or to +5,
    // which is exactly the drift this test exists to catch.
    [`${FIRST_TRACKED_YEAR}-01-01`, true],
    [`${FIRST_TRACKED_YEAR - 1}-12-31`, false],
    [`${LAST_SERVABLE_YEAR}-12-31`, true],
    [`${LAST_SERVABLE_YEAR + 1}-01-01`, false],
    [`${FIRST_TRACKED_YEAR}-W01`, true],
    [`${FIRST_TRACKED_YEAR - 1}-W52`, false],
    [`${LAST_SERVABLE_YEAR}-W52`, true],
    [`${LAST_SERVABLE_YEAR + 1}-W01`, false],
    ['2026-W31', true],
    ['2026-07-31', true],
    ['garbage', false],
    ['2026-7-31', false],
  ])('agrees with the page about %s', async (segment, servable) => {
    const fetchMock = mockBackend(200)

    const response = await proxy(requestFor(`/scenes/phoenix-az/${segment}`))
    const pageAccepts = looksLikeISOWeek(segment) || looksLikeCalendarDate(segment)

    expect(pageAccepts).toBe(servable)
    // Probing at all IS the proxy's "this might be servable" answer.
    expect(fetchMock.mock.calls.length > 0).toBe(servable)
    if (!servable) expect(response.status).toBe(404)
  })

  // A transient backend failure must never be reported as "this page does not
  // exist" — the page renders and applies its own handling.
  it('fails open on a backend 5xx', async () => {
    mockBackend(503)
    const response = await proxy(requestFor('/scenes/phoenix-az/tonight'))
    expect(response.status).toBe(200)
  })

  // The day path must be encoded for the same reason the week path is: Next
  // decodes route params before the proxy sees them, so an unencoded slug could
  // truncate the URL at a `?` and probe a different endpoint entirely.
  it('encodes the scene slug into the probe URL', async () => {
    const fetchMock = mockBackend(200)

    await proxy(requestFor('/scenes/phoenix-az&x/2026-W31'))

    expect(fetchMock).toHaveBeenCalledWith(
      'http://localhost:8080/scenes/phoenix-az%26x/week/2026-W31',
      expect.objectContaining({ method: 'HEAD', redirect: 'manual' })
    )
  })
})
