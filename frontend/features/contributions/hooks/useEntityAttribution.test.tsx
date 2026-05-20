import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { createWrapper } from '@/test/utils'

const mockApiRequest = vi.fn()

// Spy on apiRequest but keep the real API_ENDPOINTS / API_BASE_URL so the
// hook builds its true revisions URL (NEXT_PUBLIC_API_URL is pinned to
// http://localhost:8080 in vitest.config.ts).
vi.mock('@/lib/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api')
  return {
    ...actual,
    apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  }
})

// Import hook after mocks are wired.
import { useEntityAttribution } from './useEntityAttribution'

describe('useEntityAttribution', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches the most recent revision and maps it to attribution', async () => {
    mockApiRequest.mockResolvedValueOnce({
      revisions: [
        {
          id: 9,
          user_id: 3,
          user_name: 'Alice',
          user_username: 'alice',
          created_at: '2026-05-10T12:00:00Z',
        },
      ],
      total: 1,
    })

    const { result } = renderHook(
      () => useEntityAttribution('artist', 42),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    // limit=1&offset=0 — only the latest editor is needed.
    expect(mockApiRequest).toHaveBeenCalledWith(
      'http://localhost:8080/revisions/artist/42?limit=1&offset=0'
    )
    expect(result.current.data).toEqual({
      user_name: 'Alice',
      user_username: 'alice',
      created_at: '2026-05-10T12:00:00Z',
    })
  })

  it('returns null when the entity has no revisions', async () => {
    mockApiRequest.mockResolvedValueOnce({ revisions: [], total: 0 })

    const { result } = renderHook(
      () => useEntityAttribution('venue', 'the-rebel-lounge'),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data).toBeNull()
  })

  it('falls back to "Anonymous" and null username when the revision omits them', async () => {
    mockApiRequest.mockResolvedValueOnce({
      revisions: [
        {
          id: 1,
          user_id: 5,
          // user_name / user_username intentionally absent
          created_at: '2026-05-01T00:00:00Z',
        },
      ],
      total: 1,
    })

    const { result } = renderHook(
      () => useEntityAttribution('release', 100),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data).toEqual({
      user_name: 'Anonymous',
      user_username: null,
      created_at: '2026-05-01T00:00:00Z',
    })
  })

  it('is disabled (does not fetch) when options.enabled is false', () => {
    const { result } = renderHook(
      () => useEntityAttribution('artist', 42, { enabled: false }),
      { wrapper: createWrapper() }
    )

    expect(result.current.fetchStatus).toBe('idle')
    expect(mockApiRequest).not.toHaveBeenCalled()
  })

  it('surfaces an error when the revisions request fails', async () => {
    mockApiRequest.mockRejectedValueOnce(new Error('500'))

    const { result } = renderHook(
      () => useEntityAttribution('label', 7),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect(result.current.error).toBeInstanceOf(Error)
  })
})
