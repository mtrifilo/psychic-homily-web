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

  it.each([
    ['/scenes/phoenix-az/week', 'http://localhost:8080/scenes/phoenix-az/week'],
    ['/scenes/phoenix-az/tonight', 'http://localhost:8080/scenes/phoenix-az/day'],
    ['/scenes/phoenix-az/2026-W31', 'http://localhost:8080/scenes/phoenix-az/week/2026-W31'],
    ['/scenes/phoenix-az/2026-07-31', 'http://localhost:8080/scenes/phoenix-az/day/2026-07-31'],
  ])('existence-checks %s against %s', async (path, expected) => {
    const fetchMock = mockBackend(200)

    const response = await proxy(requestFor(path))

    expect(fetchMock).toHaveBeenCalledWith(expected, {
      method: 'HEAD',
      redirect: 'manual',
    })
    expect(response.status).toBe(200)
  })

  // The backend owns the calendar maths and the scene's timezone: `2026-02-30`
  // is well-formed and impossible, exactly as `2025-W53` is. Re-deriving that
  // here would drift from the authority that answers the page's own fetch.
  it.each(['/scenes/phoenix-az/2026-02-30', '/scenes/phoenix-az/2025-W53'])(
    'turns a backend 404 for %s into a real 404',
    async path => {
      mockBackend(404)
      const response = await proxy(requestFor(path))
      expect(response.status).toBe(404)
    }
  )

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

    await proxy(requestFor('/scenes/phoenix-az&x/tonight'))

    expect(fetchMock).toHaveBeenCalledWith(
      'http://localhost:8080/scenes/phoenix-az%26x/day',
      { method: 'HEAD', redirect: 'manual' }
    )
  })
})
