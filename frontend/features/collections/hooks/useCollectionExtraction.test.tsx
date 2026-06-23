import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { createWrapper } from '@/test/utils'
import type {
  ExtractCollectionRequest,
  ExtractCollectionResponse,
  ExtractedCollectionData,
} from '@/lib/types/extraction'

import { useCollectionExtraction } from './useCollectionExtraction'

const extractedData: ExtractedCollectionData = {
  source: "Pitchfork's 200 Best Albums of the 2010s",
  items: [
    {
      artist_name: 'Kendrick Lamar',
      release_title: 'To Pimp a Butterfly',
      matched_artist_id: 7,
      matched_artist_name: 'Kendrick Lamar',
      matched_artist_slug: 'kendrick-lamar',
    },
  ],
}

function jsonResponse(
  body: ExtractCollectionResponse,
  ok: boolean,
  status: number
): Response {
  return {
    ok,
    status,
    json: () => Promise.resolve(body),
    headers: new Headers(),
  } as Response
}

describe('useCollectionExtraction', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn())
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    vi.clearAllMocks()
  })

  it('extracts collection items and returns parsed data', async () => {
    const response: ExtractCollectionResponse = {
      success: true,
      data: extractedData,
    }
    vi.mocked(fetch).mockResolvedValueOnce(jsonResponse(response, true, 200))

    const { result } = renderHook(() => useCollectionExtraction(), {
      wrapper: createWrapper(),
    })

    const request: ExtractCollectionRequest = {
      type: 'text',
      text: 'Pitchfork best albums...',
    }

    await act(async () => {
      result.current.mutate(request)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(result.current.data).toEqual(response)
    expect(result.current.data?.data?.items[0].artist_name).toBe(
      'Kendrick Lamar'
    )
  })

  it('calls the extraction endpoint with the request body', async () => {
    vi.mocked(fetch).mockResolvedValueOnce(
      jsonResponse({ success: true, data: extractedData }, true, 200)
    )

    const { result } = renderHook(() => useCollectionExtraction(), {
      wrapper: createWrapper(),
    })

    const request: ExtractCollectionRequest = {
      type: 'text',
      text: 'list',
    }

    await act(async () => {
      result.current.mutate(request)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(fetch).toHaveBeenCalledWith(
      '/api/ai/extract-collection',
      expect.objectContaining({
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(request),
      })
    )
  })

  // A 200 OK with success=false is a real failure (e.g. AI could not parse the
  // input). The hook must surface it as an error, never swallow it.
  it('surfaces success=false as an error even when response.ok is true', async () => {
    const response: ExtractCollectionResponse = {
      success: false,
      error: 'Could not extract any items from the input',
    }
    vi.mocked(fetch).mockResolvedValueOnce(jsonResponse(response, true, 200))

    const { result } = renderHook(() => useCollectionExtraction(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ type: 'text', text: 'gibberish' })
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect((result.current.error as Error).message).toBe(
      'Could not extract any items from the input'
    )
  })

  it('falls back to a generic message when success=false omits an error', async () => {
    vi.mocked(fetch).mockResolvedValueOnce(
      jsonResponse({ success: false }, true, 200)
    )

    const { result } = renderHook(() => useCollectionExtraction(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ type: 'text', text: 'gibberish' })
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect((result.current.error as Error).message).toBe(
      'Failed to extract collection items'
    )
  })

  // PSY-855: a 429 rate-limit response carries a human-readable retry hint in
  // `error` ("Rate limit exceeded. Try again in N minutes."). The hook must
  // surface that exact message so the AICollectionFiller error banner shows it.
  it('surfaces the 429 rate-limit hint as the error message', async () => {
    vi.mocked(fetch).mockResolvedValueOnce(
      jsonResponse(
        {
          success: false,
          error: 'Rate limit exceeded. Try again in 42 minutes.',
          retry_after: 2520,
        },
        false,
        429
      )
    )

    const { result } = renderHook(() => useCollectionExtraction(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ type: 'text', text: 'too many' })
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect((result.current.error as Error).message).toBe(
      'Rate limit exceeded. Try again in 42 minutes.'
    )
  })
})
