import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { createWrapper } from '@/test/utils'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_BASE_URL: 'http://localhost:8080',
}))

// Import hook after mocks are wired.
import { useDataGaps } from './useDataGaps'

describe('useDataGaps', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('fetches data gaps for an entity by type and slug', async () => {
    const response = {
      gaps: [
        { field: 'description', label: 'Description', priority: 1 },
        { field: 'image_url', label: 'Image', priority: 2 },
      ],
    }
    mockApiRequest.mockResolvedValueOnce(response)

    const { result } = renderHook(
      () => useDataGaps('artist', 'phantogram'),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      'http://localhost:8080/entities/artist/phantogram/data-gaps'
    )
    expect(result.current.data).toEqual(response)
  })

  it('is disabled (does not fetch) when entitySlug is empty', () => {
    const { result } = renderHook(() => useDataGaps('artist', ''), {
      wrapper: createWrapper(),
    })

    expect(result.current.fetchStatus).toBe('idle')
    expect(mockApiRequest).not.toHaveBeenCalled()
  })

  it('is disabled when options.enabled is false, even with a valid slug', () => {
    const { result } = renderHook(
      () => useDataGaps('venue', 'the-rebel-lounge', { enabled: false }),
      { wrapper: createWrapper() }
    )

    expect(result.current.fetchStatus).toBe('idle')
    expect(mockApiRequest).not.toHaveBeenCalled()
  })

  it('fetches when options.enabled is explicitly true', async () => {
    mockApiRequest.mockResolvedValueOnce({ gaps: [] })

    const { result } = renderHook(
      () => useDataGaps('venue', 'the-rebel-lounge', { enabled: true }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockApiRequest).toHaveBeenCalledWith(
      'http://localhost:8080/entities/venue/the-rebel-lounge/data-gaps'
    )
  })

  it('surfaces an error when the request fails', async () => {
    mockApiRequest.mockRejectedValueOnce(new Error('500'))

    const { result } = renderHook(
      () => useDataGaps('label', 'sub-pop'),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect(result.current.error).toBeInstanceOf(Error)
  })
})
