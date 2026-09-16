/**
 * MSW Request Handlers
 *
 * Shared request handlers for use in tests. Tests that need custom responses
 * should override these using server.use() within individual test cases.
 *
 * Convention:
 * - Default handlers return realistic success responses
 * - Tests override specific handlers via server.use() for error/edge cases
 * - Handler URLs match the real API endpoints from lib/api.ts
 */

import { http, HttpResponse } from 'msw'

/**
 * The base URL used in tests. In the test environment (Node/jsdom),
 * API_BASE_URL resolves to 'http://localhost:8080' because vitest.config.mts
 * sets NEXT_PUBLIC_API_URL to this value.
 */
export const TEST_API_BASE = 'http://localhost:8080'

// ============================================================================
// Admin Handlers
// ============================================================================

export const adminHandlers = [
  http.get(`${TEST_API_BASE}/admin/stats`, () => {
    return HttpResponse.json({
      pending_shows: 5,
      pending_venue_edits: 2,
      pending_reports: 1,
      unverified_venues: 3,
      total_shows: 100,
      total_venues: 20,
      total_artists: 50,
      total_users: 15,
      shows_submitted_last_7_days: 12,
      users_registered_last_7_days: 3,
      total_shows_trend: 8,
      total_venues_trend: 2,
      total_artists_trend: 5,
      total_users_trend: 1,
    })
  }),

  http.get(`${TEST_API_BASE}/admin/activity`, () => {
    return HttpResponse.json({
      events: [
        {
          id: 1,
          event_type: 'show_approved',
          description: 'Show "Sonic Youth at The Rebel Lounge" approved',
          entity_type: 'show',
          entity_slug: 'sonic-youth-rebel-lounge-2026-04-15',
          timestamp: '2026-03-30T12:00:00Z',
          actor_name: 'admin',
        },
        {
          id: 2,
          event_type: 'artist_updated',
          description: 'Artist "Sonic Youth" updated',
          entity_type: 'artist',
          entity_slug: 'sonic-youth',
          timestamp: '2026-03-30T11:00:00Z',
          actor_name: 'admin',
        },
      ],
    })
  }),
]

// ============================================================================
// Scene Handlers
// ============================================================================

export const sceneHandlers = [
  http.get(`${TEST_API_BASE}/scenes`, () => {
    return HttpResponse.json({
      scenes: [
        {
          city: 'Phoenix',
          state: 'AZ',
          slug: 'phoenix-az',
          venue_count: 12,
          upcoming_show_count: 45,
          total_show_count: 200,
        },
        {
          city: 'Chicago',
          state: 'IL',
          slug: 'chicago-il',
          venue_count: 30,
          upcoming_show_count: 120,
          total_show_count: 500,
        },
      ],
      count: 2,
    })
  }),

  http.get(`${TEST_API_BASE}/scenes/:slug`, ({ params }) => {
    const { slug } = params
    return HttpResponse.json({
      city: 'Phoenix',
      state: 'AZ',
      slug,
      description: null,
      stats: {
        venue_count: 12,
        artist_count: 85,
        upcoming_show_count: 45,
        festival_count: 2,
      },
      pulse: {
        shows_this_month: 30,
        shows_prev_month: 25,
        shows_trend: 20,
        new_artists_30d: 8,
        active_venues_this_month: 10,
        shows_by_month: [20, 22, 25, 28, 30, 30],
      },
      // Tracked rooms, busiest first. The second row is the sparse shape the
      // API really sends — zero count, no slug, no website, and a sub-locality
      // city that is not the scene's own.
      venues: [
        {
          id: 1,
          name: 'Crescent Ballroom',
          slug: 'crescent-ballroom',
          website: 'https://crescentphx.com',
          city: 'Phoenix',
          state: 'AZ',
          upcoming_show_count: 12,
        },
        { id: 2, name: 'Quiet Room', city: 'Tempe', state: 'AZ', upcoming_show_count: 0 },
      ],
    })
  }),

  http.get(`${TEST_API_BASE}/scenes/:slug/artists`, ({ request }) => {
    const url = new URL(request.url)
    const limit = Number(url.searchParams.get('limit') || 20)
    return HttpResponse.json({
      artists: Array.from({ length: Math.min(limit, 3) }, (_, i) => ({
        id: i + 1,
        slug: `artist-${i + 1}`,
        name: `Artist ${i + 1}`,
        city: 'Phoenix',
        state: 'AZ',
        show_count: 10 - i,
      })),
      total: 3,
    })
  }),
]

// ============================================================================
// Venue Handlers
// ============================================================================

/**
 * The directory's rooms, as the API orders them.
 *
 * The ORDER is the point. `GET /venues` puts every room with something booked
 * ahead of every quiet one under every sort value, and orders the quiet block
 * by its last show, most recent first. Nothing in the frontend re-sorts, so
 * without a fixture that reproduces that order the directory's quiet-rooms
 * divider is only ever tested against hand-built arrays.
 */
const VENUE_ROWS = [
  {
    id: 1,
    slug: 'the-van-buren',
    name: 'The Van Buren',
    address: '401 W Van Buren St',
    city: 'Phoenix',
    state: 'AZ',
    timezone: 'America/Phoenix',
    verified: true,
    upcoming_show_count: 74,
    shows_this_week: 3,
    social: { website: 'https://thevanburenphx.test' },
    next_show: {
      event_date: '2026-09-16T02:30:00Z',
      slug: 'a-show',
      title: '',
    },
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  },
  {
    id: 2,
    slug: 'crescent-ballroom',
    name: 'Crescent Ballroom',
    address: '308 N 2nd Ave',
    city: 'Phoenix',
    state: 'AZ',
    timezone: 'America/Phoenix',
    verified: true,
    upcoming_show_count: 66,
    shows_this_week: 2,
    social: {},
    next_show: {
      event_date: '2026-09-16T03:00:00Z',
      slug: 'b-show',
      title: '',
    },
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  },
  {
    id: 3,
    slug: 'arizona-financial-theatre',
    name: 'Arizona Financial Theatre',
    address: '400 W Washington St',
    city: 'Phoenix',
    state: 'AZ',
    timezone: 'America/Phoenix',
    verified: true,
    upcoming_show_count: 0,
    shows_this_week: 0,
    social: { website: 'https://aft.test' },
    last_show: {
      event_date: '2026-08-23T02:00:00Z',
      slug: 'c-show',
      title: '',
    },
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
  },
]

/** Row ids in each accepted order. The quiet room is last in all of them. */
const VENUE_ORDERS: Record<string, number[]> = {
  upcoming: [1, 2, 3],
  next: [1, 2, 3],
  name: [2, 1, 3],
}

export const venueHandlers = [
  http.get(`${TEST_API_BASE}/venues`, ({ request }) => {
    const url = new URL(request.url)
    const limit = Number(url.searchParams.get('limit') || 50)
    const offset = Number(url.searchParams.get('offset') || 0)
    const sort = url.searchParams.get('sort') ?? 'upcoming'
    const cities = url.searchParams.get('cities')

    const order = VENUE_ORDERS[sort]
    if (!order) {
      return HttpResponse.json(
        { title: 'Unprocessable Entity', detail: 'invalid sort' },
        { status: 422 }
      )
    }

    // Every fixture room is in Phoenix, so any other city filter answers empty
    // rather than being ignored: a test that filters has to see it applied.
    const citySelected =
      !cities ||
      cities === 'all' ||
      cities.split('|').includes('Phoenix,AZ')
    const scoped = citySelected
      ? order.map(id => VENUE_ROWS.find(v => v.id === id))
      : []

    return HttpResponse.json({
      venues: scoped.slice(offset, offset + limit),
      total: scoped.length,
      limit,
      offset,
    })
  }),

  http.get(`${TEST_API_BASE}/venues/cities`, () => {
    return HttpResponse.json({
      cities: [
        { city: 'Chicago', state: 'IL', venue_count: 42 },
        { city: 'Phoenix', state: 'AZ', venue_count: 8 },
      ],
    })
  }),
]

// ============================================================================
// Show Report Handlers
// ============================================================================

export const showReportHandlers = [
  http.get(`${TEST_API_BASE}/shows/:showId/my-report`, () => {
    return HttpResponse.json({
      report: null,
    })
  }),

  http.post(`${TEST_API_BASE}/shows/:showId/report`, async ({ params, request }) => {
    const body = (await request.json()) as {
      report_type: string
      details: string | null
    }
    const showId = Number(params.showId)
    return HttpResponse.json({
      id: 1,
      show_id: showId,
      report_type: body.report_type,
      details: body.details,
      status: 'pending',
      created_at: '2026-03-30T12:00:00Z',
      updated_at: '2026-03-30T12:00:00Z',
    })
  }),
]

// ============================================================================
// Next.js Route Handlers (same-origin /api/* routes, NOT the Go backend)
// ============================================================================

export const nextRouteHandlers = [
  /**
   * IP-geo default-city route (PSY-946). ShowList / HomeShowList fire
   * `fetch('/api/geo')` on mount via useGeoDefaultCity for anonymous visitors,
   * so EVERY test that renders them hits this route. Default: geolocation
   * unavailable (`geo: null`) → no city is seeded, preserving pre-PSY-946
   * behavior for tests that don't exercise geo. The PSY-946-specific tests
   * stub `globalThis.fetch` directly (bypassing MSW), so this handler never
   * interferes with them. Wildcard origin because the fetch uses a relative
   * URL resolved against jsdom's document origin, not TEST_API_BASE.
   */
  http.get('*/api/geo', () => {
    return HttpResponse.json({ geo: null })
  }),
]

// ============================================================================
// All Handlers (combined for default server setup)
// ============================================================================

export const handlers = [
  ...adminHandlers,
  ...sceneHandlers,
  ...venueHandlers,
  ...showReportHandlers,
  ...nextRouteHandlers,
]
