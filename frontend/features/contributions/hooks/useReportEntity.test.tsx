import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { createWrapper } from '@/test/utils'
import type { ReportableEntityType } from '../types'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_BASE_URL: 'http://localhost:8080',
}))

// Import hook after mocks are wired.
import { useReportEntity } from './useReportEntity'

describe('useReportEntity', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('starts idle and exposes a mutate function', () => {
    mockApiRequest.mockResolvedValue({ success: true })

    const { result } = renderHook(() => useReportEntity(), {
      wrapper: createWrapper(),
    })

    expect(result.current.isPending).toBe(false)
    expect(result.current.isSuccess).toBe(false)
    expect(typeof result.current.mutate).toBe('function')
  })

  // URL-shape enumeration locks the CURRENT behavior of useReportEntity per
  // entity type. The non-comment, non-show branch builds its path via a raw
  // `entityType + 's'` plural concatenation (the "plural-concat" anti-pattern)
  // — that is the subject of audit PSY-766 and is intentionally NOT fixed
  // here. These cases document what the hook builds today so PSY-766 has a
  // characterization baseline; if PSY-766 swaps in an explicit plural map,
  // the expected URLs below stay identical for these inputs.
  const cases: Array<{ entityType: ReportableEntityType; expectedPath: string }> = [
    { entityType: 'artist', expectedPath: 'artists/42/report' },
    { entityType: 'venue', expectedPath: 'venues/42/report' },
    { entityType: 'festival', expectedPath: 'festivals/42/report' },
    // collection happens to pluralize correctly under raw +'s', but it still
    // flows through the same concat branch as artist/venue/festival.
    { entityType: 'collection', expectedPath: 'collections/42/report' },
    // show uses /entity-report (not /report) — distinct backend route.
    { entityType: 'show', expectedPath: 'shows/42/entity-report' },
  ]

  it.each(cases)(
    'POSTs the report for $entityType to /$expectedPath',
    async ({ entityType, expectedPath }) => {
      mockApiRequest.mockResolvedValueOnce({
        success: true,
        report: {
          id: 1,
          entity_type: entityType,
          entity_id: 42,
          report_type: 'inaccurate',
          status: 'pending',
        },
      })

      const { result } = renderHook(() => useReportEntity(), {
        wrapper: createWrapper(),
      })

      result.current.mutate({
        entityType,
        entityId: 42,
        reportType: 'inaccurate',
        details: 'this is wrong',
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))

      expect(mockApiRequest).toHaveBeenCalledWith(
        `http://localhost:8080/${expectedPath}`,
        {
          method: 'POST',
          body: JSON.stringify({
            report_type: 'inaccurate',
            details: 'this is wrong',
          }),
        }
      )
    }
  )

  it('routes comment reports to the dedicated /comments/{id}/report endpoint', async () => {
    mockApiRequest.mockResolvedValueOnce({ success: true })

    const { result } = renderHook(() => useReportEntity(), {
      wrapper: createWrapper(),
    })

    result.current.mutate({
      entityType: 'comment',
      entityId: 88,
      reportType: 'spam',
      details: 'advertising',
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      'http://localhost:8080/comments/88/report',
      {
        method: 'POST',
        body: JSON.stringify({
          report_type: 'spam',
          details: 'advertising',
        }),
      }
    )
  })

  it('omits the details field from the body when not provided', async () => {
    mockApiRequest.mockResolvedValueOnce({ success: true })

    const { result } = renderHook(() => useReportEntity(), {
      wrapper: createWrapper(),
    })

    result.current.mutate({
      entityType: 'artist',
      entityId: 5,
      reportType: 'duplicate',
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      'http://localhost:8080/artists/5/report',
      {
        method: 'POST',
        body: JSON.stringify({ report_type: 'duplicate' }),
      }
    )
  })

  it('surfaces an error when the report request fails', async () => {
    mockApiRequest.mockRejectedValueOnce(new Error('Rate limited'))

    const { result } = renderHook(() => useReportEntity(), {
      wrapper: createWrapper(),
    })

    result.current.mutate({
      entityType: 'venue',
      entityId: 3,
      reportType: 'closed_permanently',
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect(result.current.error).toBeInstanceOf(Error)
  })
})
