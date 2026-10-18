import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import {
  createTestQueryClient,
  createWrapperWithClient,
} from '@/test/utils'
import { deferred } from '@/test/inflightOrdering'
import {
  BUILD_CONSENT_VERSIONS,
  BUILD_VERSIONS_BODY,
  NEWER_VERSIONS_BODY,
  jsonResponse,
  publishedVersions,
} from '@/test/consentVersions'
import { CURRENT_PRIVACY_VERSION, CURRENT_TERMS_VERSION } from '@/lib/legal'
import { queryKeys } from '@/lib/queryClient'
import {
  CONSENT_VERSIONS_WAIT_MS,
  ConsentVersionsChangedError,
  ConsentVersionsUnavailableError,
  isConsentRefusal,
  matchesBuildVersions,
  useConsentVersions,
  useLegalVersions,
} from './useLegalVersions'

const PUBLISHED = BUILD_VERSIONS_BODY
const NEWER = {
  termsVersion: NEWER_VERSIONS_BODY.terms_version,
  privacyVersion: NEWER_VERSIONS_BODY.privacy_version,
}

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

  it('answers with the versions the route publishes when they are this build\'s', async () => {
    fetchMock.mockResolvedValue(publishedVersions(PUBLISHED))
    const { reader } = renderReader()

    await expect(reader.read()).resolves.toEqual(BUILD_CONSENT_VERSIONS)
    const [url, init] = fetchMock.mock.calls[0]
    expect(new URL(url).pathname).toBe('/legal/versions')
    expect((init as RequestInit).method).toBe('GET')
  })

  it('refuses versions other than this build\'s, and keeps them for the form', async () => {
    fetchMock.mockResolvedValue(publishedVersions(NEWER_VERSIONS_BODY))
    const { reader, queryClient } = renderReader()

    await expect(reader.read()).rejects.toBeInstanceOf(ConsentVersionsChangedError)
    expect(queryClient.getQueryData(queryKeys.legal.versions)).toEqual(NEWER)
  })

  it.each([
    ['this build\'s versions', BUILD_CONSENT_VERSIONS, true],
    ['a newer terms version', { ...BUILD_CONSENT_VERSIONS, termsVersion: '2031-04-01' }, false],
    ['a newer privacy version', { ...BUILD_CONSENT_VERSIONS, privacyVersion: '2031-04-02' }, false],
  ])('matches %s against the build: %s', (_label, versions, matches) => {
    expect(matchesBuildVersions(versions)).toBe(matches)
    expect(BUILD_CONSENT_VERSIONS).toEqual({
      termsVersion: CURRENT_TERMS_VERSION,
      privacyVersion: CURRENT_PRIVACY_VERSION,
    })
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
    ['a body without the terms version', () => jsonResponse({ privacy_version: PUBLISHED.privacy_version })],
    ['an empty version', () => publishedVersions({ ...PUBLISHED, terms_version: '' })],
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
    it('reads past the browser cache, even with a fresh answer cached', async () => {
      fetchMock.mockImplementation(() => Promise.resolve(publishedVersions(PUBLISHED)))
      const { reader } = renderReader()
      await reader.read()

      await expect(reader.refresh()).resolves.toEqual(BUILD_CONSENT_VERSIONS)

      expect(fetchMock).toHaveBeenCalledTimes(2)
      expect((fetchMock.mock.calls[0][1] as RequestInit).cache).toBeUndefined()
      expect((fetchMock.mock.calls[1][1] as RequestInit).cache).toBe('no-cache')
    })

    it('refuses, and caches, versions that changed since the last read', async () => {
      fetchMock
        .mockResolvedValueOnce(publishedVersions(PUBLISHED))
        .mockResolvedValueOnce(publishedVersions(NEWER_VERSIONS_BODY))
      const { reader, queryClient } = renderReader()
      await reader.read()

      await expect(reader.refresh()).rejects.toBeInstanceOf(ConsentVersionsChangedError)
      expect(queryClient.getQueryData(queryKeys.legal.versions)).toEqual(NEWER)
    })

    it('starts its own read instead of joining one already in flight', async () => {
      // The in-flight answer would be refused; the refresh's own is not.
      const inFlight = deferred<Response>()
      fetchMock
        .mockReturnValueOnce(inFlight.promise)
        .mockResolvedValueOnce(publishedVersions(PUBLISHED))
      const { reader } = renderReader()
      const firstRead = reader.read().catch(() => undefined)
      await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1))

      const refreshed = reader.refresh()
      inFlight.resolve(publishedVersions(NEWER_VERSIONS_BODY))

      await expect(refreshed).resolves.toEqual(BUILD_CONSENT_VERSIONS)
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
