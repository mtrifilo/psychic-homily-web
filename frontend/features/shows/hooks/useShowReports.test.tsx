import { describe, it, expect } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { server } from '@/test/mocks/server'
import { TEST_API_BASE } from '@/test/mocks/handlers'
import { createWrapper } from '@/test/utils'
import { useMyShowReport, useReportShow } from './useShowReports'

describe('useMyShowReport', () => {
  it('fetches user report for a show by numeric ID', async () => {
    server.use(
      http.get(`${TEST_API_BASE}/shows/:showId/my-report`, () => {
        return HttpResponse.json({
          report: {
            id: 1,
            show_id: 42,
            report_type: 'inaccurate',
            details: 'Off by one day',
            status: 'pending',
            created_at: '2026-03-30T12:00:00Z',
            updated_at: '2026-03-30T12:00:00Z',
          },
        })
      })
    )

    const { result } = renderHook(() => useMyShowReport(42), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data?.report?.report_type).toBe('inaccurate')
  })

  it('fetches user report for a show by string slug', async () => {
    const { result } = renderHook(() => useMyShowReport('my-slug'), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    // Default handler returns { report: null }
    expect(result.current.data?.report).toBeNull()
  })

  it('does not fetch when showId is null', () => {
    const { result } = renderHook(() => useMyShowReport(null), {
      wrapper: createWrapper(),
    })

    expect(result.current.fetchStatus).toBe('idle')
  })

  it('handles API errors', async () => {
    server.use(
      http.get(`${TEST_API_BASE}/shows/:showId/my-report`, () => {
        return HttpResponse.json({ message: 'Not found' }, { status: 404 })
      })
    )

    const { result } = renderHook(() => useMyShowReport(999), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
  })
})

describe('useReportShow', () => {
  it('reports a show with POST and receives the created report', async () => {
    let capturedBody: Record<string, unknown> | null = null
    server.use(
      http.post(
        `${TEST_API_BASE}/shows/:showId/report`,
        async ({ request, params }) => {
          capturedBody = (await request.json()) as Record<string, unknown>
          return HttpResponse.json({
            id: 1,
            show_id: Number(params.showId),
            report_type: capturedBody.report_type,
            details: capturedBody.details,
            status: 'pending',
            created_at: '2026-03-30T12:00:00Z',
            updated_at: '2026-03-30T12:00:00Z',
          })
        }
      )
    )

    const { result } = renderHook(() => useReportShow(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({
        showId: 42,
        reportType: 'inaccurate',
        details: 'The date is March 20, not March 19',
      })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    // Verify the request body was sent correctly
    expect(capturedBody).toEqual({
      report_type: 'inaccurate',
      details: 'The date is March 20, not March 19',
    })

    // Verify the response data
    expect(result.current.data?.id).toBe(1)
    expect(result.current.data?.report_type).toBe('inaccurate')
  })

  it('sends null for details when not provided', async () => {
    let capturedBody: Record<string, unknown> | null = null
    server.use(
      http.post(`${TEST_API_BASE}/shows/:showId/report`, async ({ request }) => {
        capturedBody = (await request.json()) as Record<string, unknown>
        return HttpResponse.json({
          id: 1,
          show_id: 42,
          report_type: 'cancelled',
          details: null,
          status: 'pending',
          created_at: '2026-03-30T12:00:00Z',
          updated_at: '2026-03-30T12:00:00Z',
        })
      })
    )

    const { result } = renderHook(() => useReportShow(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({
        showId: 42,
        reportType: 'cancelled',
      })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(capturedBody).toEqual({
      report_type: 'cancelled',
      details: null,
    })
  })

  it('handles conflict errors (already reported)', async () => {
    server.use(
      http.post(`${TEST_API_BASE}/shows/:showId/report`, () => {
        return HttpResponse.json({ message: 'Already reported' }, { status: 409 })
      })
    )

    const { result } = renderHook(() => useReportShow(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ showId: 42, reportType: 'inaccurate' })
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
  })
})
