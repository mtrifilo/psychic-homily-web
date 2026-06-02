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

  // Exhaustive enumeration of ReportableEntityType — verifies that
  // REPORT_PLURAL and REPORT_SUFFIX stay in sync with the union (PSY-766).
  // Adding a new entity type without populating both maps is a TS error;
  // changing a URL shape without updating this list is a test failure.
  // The `entityType + 's'` concatenation antipattern (audited and removed
  // by PSY-766) is intentionally NOT recreated here — each entry below is
  // the literal URL the hook is contractually required to build.
  const cases: Array<{ entityType: ReportableEntityType; expectedPath: string }> = [
    { entityType: 'artist', expectedPath: 'artists/42/report' },
    { entityType: 'venue', expectedPath: 'venues/42/report' },
    { entityType: 'festival', expectedPath: 'festivals/42/report' },
    { entityType: 'collection', expectedPath: 'collections/42/report' },
    // PSY-661: releases use the regular /{plural}/{id}/report shape.
    { entityType: 'release', expectedPath: 'releases/42/report' },
    // comment uses the same /{plural}/{id}/report shape as artist/venue;
    // the dedicated /comments handler family is a backend implementation
    // detail. URL identity is what this test pins.
    { entityType: 'comment', expectedPath: 'comments/42/report' },
    // show is the one irregular suffix — /shows/{id}/entity-report — and
    // is the most likely site of a silent regression if the suffix map
    // ever drifts.
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
