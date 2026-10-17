import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import {
  createTestQueryClient,
  createWrapperWithClient,
} from '@/test/utils'
import { deferred } from '@/test/inflightOrdering'
import { jsonResponse, publishedVersions } from '@/test/consentVersions'
import { CURRENT_PRIVACY_VERSION, CURRENT_TERMS_VERSION } from '@/lib/legal'
import { queryKeys } from '@/lib/queryClient'
import {
  CONSENT_VERSIONS_WAIT_MS,
  ConsentVersionsUnavailableError,
  isConsentRefusal,
  useConsentVersions,
  useLegalVersions,
} from './useLegalVersions'

// Versions no build names, so a test can tell the route's answer from the
// constants in lib/legal.ts.
const PUBLISHED = { terms_version: '2031-03-01', privacy_version: '2031-03-02' }
const REPUBLISHED = { terms_version: '2031-04-01', privacy_version: '2031-04-02' }

const fetchMock = vi.fn()

function renderReader() {
  const queryClient = createTestQueryClient()
  const { result } = renderHook(() => useConsentVersions(), {
    wrapper: createWrapperWithClient(queryClient),
  })
  return { reader: result.current, queryClient }
}

describe('useConsentVersions', () => {
  beforeEach(() => {
    fetchMock.mockReset()
    vi.stubGlobal('fetch', fetchMock)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    vi.useRealTimers()
  })

  it('reads the versions the route publishes, not the build constants', async () => {
    fetchMock.mockResolvedValue(publishedVersions(PUBLISHED))
    const { reader } = renderReader()

    await expect(reader.read()).resolves.toEqual({
      termsVersion: '2031-03-01',
      privacyVersion: '2031-03-02',
    })
    expect(PUBLISHED.terms_version).not.toBe(CURRENT_TERMS_VERSION)
    expect(PUBLISHED.privacy_version).not.toBe(CURRENT_PRIVACY_VERSION)
    const [url, init] = fetchMock.mock.calls[0]
    expect(new URL(url).pathname).toBe('/legal/versions')
    expect((init as RequestInit).method).toBe('GET')
  })

  it('does not fetch on mount', () => {
    renderReader()

    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('reuses a fresh answer for the next submit', async () => {
    fetchMock.mockImplementation(() => Promise.resolve(publishedVersions(PUBLISHED)))
    const { reader, queryClient } = renderReader()
    // The query an open form holds, which keeps the answer cached.
    renderHook(() => useLegalVersions(), {
      wrapper: createWrapperWithClient(queryClient),
    })
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1))

    await reader.read()
    await reader.read()

    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it.each([
    ['an error status', () => jsonResponse({ detail: 'boom' }, 500)],
    ['a body without the terms version', () => jsonResponse({ privacy_version: '2031-03-02' })],
    ['an empty version', () => publishedVersions({ terms_version: '', privacy_version: '2031-03-02' })],
  ])('refuses %s instead of answering with the constants', async (_label, response) => {
    fetchMock.mockImplementation(() => Promise.resolve(response()))
    const { reader } = renderReader()

    await expect(reader.read()).rejects.toBeInstanceOf(
      ConsentVersionsUnavailableError
    )
  })

  it('refuses a network failure', async () => {
    fetchMock.mockRejectedValue(new TypeError('Failed to fetch'))
    const { reader } = renderReader()

    await expect(reader.read()).rejects.toBeInstanceOf(
      ConsentVersionsUnavailableError
    )
  })

  it(`gives up after ${CONSENT_VERSIONS_WAIT_MS} ms without an answer`, async () => {
    vi.useFakeTimers()
    fetchMock.mockImplementation(() => new Promise(() => {}))
    const { reader } = renderReader()

    const read = reader.read()
    const settled = vi.fn()
    read.then(settled, settled)

    await vi.advanceTimersByTimeAsync(CONSENT_VERSIONS_WAIT_MS - 1)
    expect(settled).not.toHaveBeenCalled()

    await vi.advanceTimersByTimeAsync(1)
    await expect(read).rejects.toBeInstanceOf(ConsentVersionsUnavailableError)
    // Aborted, so the next submit reads again instead of joining it.
    expect((fetchMock.mock.calls[0][1] as RequestInit).signal?.aborted).toBe(true)
  })

  describe('refresh', () => {
    it('reads past the browser cache and replaces a fresh answer', async () => {
      fetchMock
        .mockResolvedValueOnce(publishedVersions(PUBLISHED))
        .mockResolvedValueOnce(publishedVersions(REPUBLISHED))
      const { reader, queryClient } = renderReader()
      await reader.read()

      await expect(reader.refresh()).resolves.toEqual({
        termsVersion: '2031-04-01',
        privacyVersion: '2031-04-02',
      })

      expect((fetchMock.mock.calls[0][1] as RequestInit).cache).toBeUndefined()
      expect((fetchMock.mock.calls[1][1] as RequestInit).cache).toBe('no-cache')
      expect(queryClient.getQueryData(queryKeys.legal.versions)).toEqual({
        termsVersion: '2031-04-01',
        privacyVersion: '2031-04-02',
      })
    })

    it('starts its own read instead of joining one already in flight', async () => {
      const inFlight = deferred<Response>()
      fetchMock
        .mockReturnValueOnce(inFlight.promise)
        .mockResolvedValueOnce(publishedVersions(REPUBLISHED))
      const { reader } = renderReader()
      const firstRead = reader.read().catch(() => undefined)
      await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1))

      const refreshed = reader.refresh()
      inFlight.resolve(publishedVersions(PUBLISHED))

      await expect(refreshed).resolves.toEqual({
        termsVersion: '2031-04-01',
        privacyVersion: '2031-04-02',
      })
      expect(fetchMock).toHaveBeenCalledTimes(2)
      expect((fetchMock.mock.calls[0][1] as RequestInit).signal?.aborted).toBe(true)
      expect((fetchMock.mock.calls[1][1] as RequestInit).cache).toBe('no-cache')
      await firstRead
    })

    it('refuses a failed read', async () => {
      fetchMock.mockResolvedValue(jsonResponse({ detail: 'boom' }, 503))
      const { reader } = renderReader()

      await expect(reader.refresh()).rejects.toBeInstanceOf(
        ConsentVersionsUnavailableError
      )
    })
  })
})

describe('isConsentRefusal', () => {
  it('names only the consent refusal', () => {
    expect(isConsentRefusal('TERMS_ACCEPTANCE_REQUIRED')).toBe(true)
    expect(isConsentRefusal('AGE_CONFIRMATION_REQUIRED')).toBe(false)
    expect(isConsentRefusal(undefined)).toBe(false)
  })
})
