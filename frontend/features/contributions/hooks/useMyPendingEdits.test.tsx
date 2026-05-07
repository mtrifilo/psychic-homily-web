import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { createWrapper } from '@/test/utils'

// Create mocks
const mockApiRequest = vi.fn()

// Mock the api module — useMyPendingEdits builds its URL with API_BASE_URL.
vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_BASE_URL: 'http://localhost:8080',
}))

// Import hook after mocks are set up
import { useMyPendingEdits } from './useMyPendingEdits'

describe('useMyPendingEdits', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches the signed-in user pending edits with default pagination', async () => {
    const mockResponse = {
      edits: [
        {
          id: 1,
          entity_type: 'artist',
          entity_id: 42,
          entity_name: 'Phantogram',
          entity_slug: 'phantogram',
          submitted_by: 7,
          submitter_name: 'Alice',
          submitter_username: 'alice',
          field_changes: [
            { field: 'description', old_value: 'old', new_value: 'new' },
          ],
          summary: 'fix bio',
          status: 'pending',
          created_at: '2026-05-05T10:00:00Z',
          updated_at: '2026-05-05T10:00:00Z',
        },
      ],
      total: 1,
    }
    mockApiRequest.mockResolvedValueOnce(mockResponse)

    const { result } = renderHook(() => useMyPendingEdits(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    // Default limit=20, offset=0
    expect(mockApiRequest).toHaveBeenCalledWith(
      'http://localhost:8080/my/pending-edits?limit=20&offset=0'
    )
    expect(result.current.data).toEqual(mockResponse)
  })

  it('passes custom limit and offset for pagination', async () => {
    mockApiRequest.mockResolvedValueOnce({ edits: [], total: 0 })

    const { result } = renderHook(() => useMyPendingEdits(50, 100), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      'http://localhost:8080/my/pending-edits?limit=50&offset=100'
    )
  })

  it('returns an empty list when the user has no pending edits', async () => {
    mockApiRequest.mockResolvedValueOnce({ edits: [], total: 0 })

    const { result } = renderHook(() => useMyPendingEdits(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data?.edits).toEqual([])
    expect(result.current.data?.total).toBe(0)
  })
})
