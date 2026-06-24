import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { createWrapper } from '@/test/utils'
import type {
  ExtractShowRequest,
  ExtractShowResponse,
  ExtractedShowData,
} from '@/lib/types/extraction'

import { useShowExtraction } from './useShowExtraction'

const extractedData: ExtractedShowData = {
  artists: [{ name: 'The National', is_headliner: true }],
  venue: { name: 'Valley Bar', city: 'Phoenix', state: 'AZ' },
  date: '2025-06-15',
  time: '20:00',
  cost: '$25',
  ages: '21+',
}

function jsonResponse(
  body: ExtractShowResponse,
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

describe('useShowExtraction', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn())
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    vi.clearAllMocks()
  })

  it('extracts show info from text and returns parsed data', async () => {
    const response: ExtractShowResponse = { success: true, data: extractedData }
    vi.mocked(fetch).mockResolvedValueOnce(jsonResponse(response, true, 200))

    const { result } = renderHook(() => useShowExtraction(), {
      wrapper: createWrapper(),
    })

    const request: ExtractShowRequest = {
      type: 'text',
      text: 'The National at Valley Bar 6/15',
    }

    await act(async () => {
      result.current.mutate(request)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(result.current.data).toEqual(response)
    expect(result.current.data?.data?.artists[0].name).toBe('The National')
  })

  it('calls the extraction endpoint with the request body', async () => {
    vi.mocked(fetch).mockResolvedValueOnce(
      jsonResponse({ success: true, data: extractedData }, true, 200)
    )

    const { result } = renderHook(() => useShowExtraction(), {
      wrapper: createWrapper(),
    })

    const request: ExtractShowRequest = {
      type: 'image',
      image_data: 'base64data',
      media_type: 'image/jpeg',
    }

    await act(async () => {
      result.current.mutate(request)
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(fetch).toHaveBeenCalledWith(
      '/api/ai/extract-show',
      expect.objectContaining({
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(request),
      })
    )
  })

  it('reflects loading state while the request is in flight', async () => {
    let resolveFetch: ((value: Response) => void) | undefined
    vi.mocked(fetch).mockReturnValueOnce(
      new Promise<Response>((resolve) => {
        resolveFetch = resolve
      })
    )

    const { result } = renderHook(() => useShowExtraction(), {
      wrapper: createWrapper(),
    })

    act(() => {
      result.current.mutate({ type: 'text', text: 'pending' })
    })

    await waitFor(() => expect(result.current.isPending).toBe(true))

    await act(async () => {
      resolveFetch?.(jsonResponse({ success: true, data: extractedData }, true, 200))
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
  })

  // A 200 OK with success=false is a real failure (e.g. AI could not parse the
  // flyer). The hook must surface it as an error, never swallow it.
  it('surfaces success=false as an error even when response.ok is true', async () => {
    const response: ExtractShowResponse = {
      success: false,
      error: 'Could not extract show information from the image',
    }
    vi.mocked(fetch).mockResolvedValueOnce(jsonResponse(response, true, 200))

    const { result } = renderHook(() => useShowExtraction(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ type: 'text', text: 'gibberish' })
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect(result.current.data).toBeUndefined()
    expect((result.current.error as Error).message).toBe(
      'Could not extract show information from the image'
    )
  })

  it('falls back to a generic message when success=false omits an error', async () => {
    vi.mocked(fetch).mockResolvedValueOnce(
      jsonResponse({ success: false }, true, 200)
    )

    const { result } = renderHook(() => useShowExtraction(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ type: 'text', text: 'gibberish' })
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect((result.current.error as Error).message).toBe(
      'Failed to extract show information'
    )
  })

  it('surfaces a non-ok HTTP response as an error', async () => {
    vi.mocked(fetch).mockResolvedValueOnce(
      jsonResponse({ success: false, error: 'Internal server error' }, false, 500)
    )

    const { result } = renderHook(() => useShowExtraction(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ type: 'text', text: 'boom' })
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect((result.current.error as Error).message).toBe('Internal server error')
  })

  // PSY-857 defensive parse: an upstream HTML 502 (gateway error page) makes
  // response.json() reject. The hook must catch that and throw a friendly
  // status-based message, NOT the opaque "Unexpected token '<'" SyntaxError.
  it('throws a status-based message when the body is not JSON (HTML 502)', async () => {
    vi.mocked(fetch).mockResolvedValueOnce({
      ok: false,
      status: 502,
      json: () => Promise.reject(new SyntaxError("Unexpected token '<'")),
      headers: new Headers(),
    } as unknown as Response)

    const { result } = renderHook(() => useShowExtraction(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ type: 'text', text: 'gateway down' })
    })

    await waitFor(() => expect(result.current.isError).toBe(true))

    expect((result.current.error as Error).message).toBe(
      'AI service error (HTTP 502)'
    )
  })

  // PSY-855: a 429 rate-limit response carries a human-readable retry hint in
  // `error` ("Rate limit exceeded. Try again in N minutes."). The hook must
  // surface that exact message so the AIFormFiller error banner shows it.
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

    const { result } = renderHook(() => useShowExtraction(), {
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

  it('allows retrying after a failure and succeeds on the second attempt', async () => {
    vi.mocked(fetch)
      .mockResolvedValueOnce(
        jsonResponse({ success: false, error: 'Temporary failure' }, false, 503)
      )
      .mockResolvedValueOnce(
        jsonResponse({ success: true, data: extractedData }, true, 200)
      )

    const { result } = renderHook(() => useShowExtraction(), {
      wrapper: createWrapper(),
    })

    await act(async () => {
      result.current.mutate({ type: 'text', text: 'retry me' })
    })
    await waitFor(() => expect(result.current.isError).toBe(true))

    await act(async () => {
      result.current.mutate({ type: 'text', text: 'retry me' })
    })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(result.current.data?.success).toBe(true)
    expect(fetch).toHaveBeenCalledTimes(2)
  })
})
