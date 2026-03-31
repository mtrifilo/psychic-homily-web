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
        },
        {
          city: 'Chicago',
          state: 'IL',
          slug: 'chicago-il',
          venue_count: 30,
          upcoming_show_count: 120,
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
// All Handlers (combined for default server setup)
// ============================================================================

export const handlers = [...adminHandlers, ...sceneHandlers, ...showReportHandlers]
