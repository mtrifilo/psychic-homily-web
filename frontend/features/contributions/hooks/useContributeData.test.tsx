import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { createWrapper } from '@/test/utils'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_BASE_URL: 'http://localhost:8080',
}))

// Import hooks after mocks are wired.
import { useContributeOpportunities, useContributeCategory } from './useContributeData'

describe('useContributeOpportunities', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches the contribution-opportunities summary', async () => {
    const summary = {
      categories: [
        {
          key: 'missing_bio',
          label: 'Missing Bio',
          entity_type: 'artist',
          count: 12,
          description: 'Artists with no description',
        },
      ],
      total_items: 12,
    }
    mockApiRequest.mockResolvedValueOnce(summary)

    const { result } = renderHook(() => useContributeOpportunities(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      'http://localhost:8080/contribute/opportunities'
    )
    expect(result.current.data).toEqual(summary)
  })

  it('surfaces an error when the summary request fails', async () => {
    mockApiRequest.mockRejectedValueOnce(new Error('boom'))

    const { result } = renderHook(() => useContributeOpportunities(), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect(result.current.error).toBeInstanceOf(Error)
  })
})

describe('useContributeCategory', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches the items for a given category with limit=20', async () => {
    const payload = {
      items: [
        {
          entity_type: 'artist',
          entity_id: 7,
          name: 'Phantogram',
          slug: 'phantogram',
          reason: 'missing bio',
          show_count: 3,
        },
      ],
      total: 1,
    }
    mockApiRequest.mockResolvedValueOnce(payload)

    const { result } = renderHook(() => useContributeCategory('missing_bio'), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      'http://localhost:8080/contribute/opportunities/missing_bio?limit=20'
    )
    expect(result.current.data).toEqual(payload)
  })

  it('is disabled (does not fetch) when the category is an empty string', async () => {
    const { result } = renderHook(() => useContributeCategory(''), {
      wrapper: createWrapper(),
    })

    // enabled: !!category — empty string keeps the query idle (no fetch in
    // flight) while still pending (no data resolved yet).
    expect(result.current.fetchStatus).toBe('idle')
    expect(result.current.isPending).toBe(true)
    expect(mockApiRequest).not.toHaveBeenCalled()
  })

  it('surfaces an error when the category request fails', async () => {
    mockApiRequest.mockRejectedValueOnce(new Error('not found'))

    const { result } = renderHook(() => useContributeCategory('bad_category'), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect(result.current.error).toBeInstanceOf(Error)
  })
})
