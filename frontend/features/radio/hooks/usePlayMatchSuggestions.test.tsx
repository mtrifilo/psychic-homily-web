import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { createWrapper } from '@/test/utils'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_BASE_URL: 'http://localhost:8080',
}))

import {
  useOwnPlayMatchSuggestion,
  useCreatePlayMatchSuggestion,
  playMatchSuggestionQueryKeys,
} from './usePlayMatchSuggestions'

describe('playMatchSuggestionQueryKeys', () => {
  it('keys mine queries by play id', () => {
    expect(playMatchSuggestionQueryKeys.mine(42)).toEqual([
      'radio',
      'plays',
      42,
      'match-suggestions',
      'mine',
    ])
  })
})

describe('useOwnPlayMatchSuggestion', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('does not fetch when disabled', () => {
    const { result } = renderHook(
      () => useOwnPlayMatchSuggestion(10, false),
      { wrapper: createWrapper() }
    )
    expect(result.current.fetchStatus).toBe('idle')
    expect(mockApiRequest).not.toHaveBeenCalled()
  })

  it('GETs the mine endpoint and returns the suggestion', async () => {
    const entry = { id: 1, play_id: 10, status: 'pending' }
    mockApiRequest.mockResolvedValueOnce({ suggestion: entry })

    const { result } = renderHook(
      () => useOwnPlayMatchSuggestion(10, true),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data).toEqual(entry)
    expect(mockApiRequest).toHaveBeenCalledWith(
      'http://localhost:8080/radio/plays/10/match-suggestions/mine'
    )
  })
})

describe('useCreatePlayMatchSuggestion', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApiRequest.mockReset()
  })

  it('POSTs artist_id and optional note', async () => {
    const entry = {
      id: 5,
      play_id: 10,
      status: 'pending',
      suggested_artist_id: 42,
    }
    mockApiRequest.mockResolvedValueOnce(entry)

    const { result } = renderHook(() => useCreatePlayMatchSuggestion(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({
        playId: 10,
        artistId: 42,
        note: 'same band',
      })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data).toEqual(entry)
    expect(mockApiRequest).toHaveBeenCalledWith(
      'http://localhost:8080/radio/plays/10/match-suggestions',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ artist_id: 42, note: 'same band' }),
      })
    )
  })

  it('omits empty note from the body', async () => {
    mockApiRequest.mockResolvedValueOnce({
      id: 5,
      play_id: 10,
      status: 'pending',
    })

    const { result } = renderHook(() => useCreatePlayMatchSuggestion(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ playId: 10, artistId: 42, note: '   ' })
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      'http://localhost:8080/radio/plays/10/match-suggestions',
      expect.objectContaining({
        body: JSON.stringify({ artist_id: 42 }),
      })
    )
  })
})
