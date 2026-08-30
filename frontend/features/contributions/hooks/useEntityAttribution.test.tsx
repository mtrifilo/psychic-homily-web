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
      total: 1,
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

  // PSY-1940: an omitted user_name is the backend declining to name the author
  // (hidden contributions, or an email-only name), not a gap to paper over. It
  // must arrive as null so consumers render no byline; the old 'Anonymous'
  // fallback turned "we may not say" into a claim about a person.
  it('maps an omitted user_name to null rather than "Anonymous"', async () => {
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
      user_name: null,
      user_username: null,
      created_at: '2026-05-01T00:00:00Z',
      total: 1,
    })
  })

  // The revision itself still resolves: the edit and its date are facts, only
  // the person is withheld. A consumer that treated a nameless revision as "no
  // revision" would drop the edit count and the "updated" date with it.
  it('still returns the revision when the author is unnamed', async () => {
    mockApiRequest.mockResolvedValueOnce({
      revisions: [
        {
          id: 1,
          user_id: 5,
          user_username: null,
          created_at: '2026-05-01T00:00:00Z',
        },
      ],
      total: 12,
    })

    const { result } = renderHook(
      () => useEntityAttribution('venue', 3),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data).not.toBeNull()
    expect(result.current.data?.total).toBe(12)
    expect(result.current.data?.created_at).toBe('2026-05-01T00:00:00Z')
  })

  // The count is passed through untouched: the contract declares it
  // required, and masking a backend regression with a floor would trade a
  // loud "0 edits beside a revision" for a quiet wrong number.
  it('passes the reported total through as-is', async () => {
    mockApiRequest.mockResolvedValueOnce({
      revisions: [
        {
          id: 2,
          user_id: 5,
          user_name: 'Alice',
          user_username: 'alice',
          created_at: '2026-05-01T00:00:00Z',
        },
      ],
      total: 40,
    })

    const { result } = renderHook(
      () => useEntityAttribution('show', 12),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data?.total).toBe(40)
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
