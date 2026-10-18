'use client'

import { useMemo } from 'react'
import {
  useQuery,
  useQueryClient,
  type QueryClient,
} from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import { AuthErrorCode } from '@/lib/errors'
import { CURRENT_PRIVACY_VERSION, CURRENT_TERMS_VERSION } from '@/lib/legal'
import { queryKeys } from '@/lib/queryClient'
import type { components } from '@/types/api'

type LegalVersionsResponse = components['schemas']['LegalVersionsResponseBody']

/** The Terms of Service and Privacy Policy versions one sign-up sends. */
export interface ConsentVersions {
  termsVersion: string
  privacyVersion: string
}

/**
 * How long a read of the published versions is reused. It matches the
 * `max-age` the backend sends with them.
 */
export const LEGAL_VERSIONS_STALE_TIME_MS = 5 * 60 * 1000

/** How long a sign-up waits for the published versions before it gives up. */
export const CONSENT_VERSIONS_WAIT_MS = 10_000

/**
 * The published versions could not be read in time. A sign-up that meets it
 * is not sent: the versions it would carry exist only in the backend's answer,
 * never in this build.
 */
export class ConsentVersionsUnavailableError extends Error {
  constructor(options?: { cause?: unknown }) {
    super('The current consent versions could not be read', options)
    this.name = 'ConsentVersionsUnavailableError'
  }
}

/**
 * The backend publishes versions other than the ones this build's /terms and
 * /privacy pages show. A sign-up that meets it is not sent, so no account
 * records a version whose text the reader was not shown; a reload brings the
 * build that shows it.
 */
export class ConsentVersionsChangedError extends Error {
  constructor() {
    super('The published consent versions differ from this build')
    this.name = 'ConsentVersionsChangedError'
  }
}

/**
 * Whether published versions are the ones this build's legal pages show
 * (CURRENT_TERMS_VERSION and CURRENT_PRIVACY_VERSION in lib/legal.ts).
 */
export function matchesBuildVersions(versions: ConsentVersions): boolean {
  return (
    versions.termsVersion === CURRENT_TERMS_VERSION &&
    versions.privacyVersion === CURRENT_PRIVACY_VERSION
  )
}

/**
 * Reads the versions every sign-up path sends at the moment it submits.
 */
export interface ConsentVersionsReader {
  /**
   * The versions to send now: the last answer while it is fresh, otherwise a
   * new read. Rejects with ConsentVersionsUnavailableError when no answer
   * arrives within CONSENT_VERSIONS_WAIT_MS or the answer is unusable, and
   * with ConsentVersionsChangedError when the answer is not this build's
   * versions (the answer is still cached, for the form to show why).
   */
  read: () => Promise<ConsentVersions>
  /**
   * A new read that bypasses every cache, browser HTTP cache included, for a
   * sign-up the backend refused with versions it had published. Rejects like
   * `read`.
   */
  refresh: () => Promise<ConsentVersions>
}

// Every read shares one cache entry and one freshness, so a submit joins the
// form's early read. No retries: a submit made while the early read retried
// would wait out the backoff, and the next submit reads again anyway.
const legalVersionsQuery = {
  queryKey: queryKeys.legal.versions,
  queryFn: ({ signal }: { signal: AbortSignal }) => fetchConsentVersions(signal),
  staleTime: LEGAL_VERSIONS_STALE_TIME_MS,
  retry: false,
}

/**
 * Starts reading the published versions as soon as a sign-up form shows, so a
 * submit rarely has to wait for them.
 */
export function useLegalVersions() {
  return useQuery(legalVersionsQuery)
}

/** The reader for the shared query client. It never fetches on mount. */
export function useConsentVersions(): ConsentVersionsReader {
  const queryClient = useQueryClient()
  return useMemo(() => consentVersionsReader(queryClient), [queryClient])
}

/**
 * Whether a sign-up was refused for its consent. Every sign-up form sends
 * `terms_accepted` with the published versions, so for them this refusal
 * means the backend no longer accepts the versions they read.
 */
export function isConsentRefusal(code: string | null | undefined): boolean {
  return code === AuthErrorCode.TERMS_ACCEPTANCE_REQUIRED
}

function consentVersionsReader(queryClient: QueryClient): ConsentVersionsReader {
  return {
    read: () =>
      sendable(
        withinConsentWait(queryClient, queryClient.fetchQuery(legalVersionsQuery))
      ),
    refresh: () =>
      sendable(withinConsentWait(queryClient, readPastCaches(queryClient))),
  }
}

async function readPastCaches(
  queryClient: QueryClient
): Promise<ConsentVersions> {
  // A fetch already in flight may be answered from the HTTP cache, and
  // fetchQuery would hand that fetch back instead of starting this one.
  await queryClient.cancelQueries({ queryKey: queryKeys.legal.versions })
  return queryClient.fetchQuery({
    ...legalVersionsQuery,
    queryFn: ({ signal }) => fetchConsentVersions(signal, 'no-cache'),
    staleTime: 0,
  })
}

// Only this build's versions are ever sent; see ConsentVersionsChangedError.
async function sendable(
  read: Promise<ConsentVersions>
): Promise<ConsentVersions> {
  const versions = await read
  if (!matchesBuildVersions(versions)) {
    throw new ConsentVersionsChangedError()
  }
  return versions
}

async function fetchConsentVersions(
  signal: AbortSignal,
  cache?: RequestCache
): Promise<ConsentVersions> {
  const body = await apiRequest<LegalVersionsResponse>(
    API_ENDPOINTS.LEGAL_VERSIONS,
    { method: 'GET', signal, ...(cache && { cache }) }
  )
  if (!isVersion(body?.terms_version) || !isVersion(body?.privacy_version)) {
    throw new Error('GET /legal/versions answered without both versions')
  }
  return {
    termsVersion: body.terms_version,
    privacyVersion: body.privacy_version,
  }
}

function isVersion(value: unknown): value is string {
  return typeof value === 'string' && value.length > 0
}

// A read that misses the wait is aborted, so the next submit starts a read of
// its own instead of joining the stalled one.
async function withinConsentWait(
  queryClient: QueryClient,
  read: Promise<ConsentVersions>
): Promise<ConsentVersions> {
  let timer: ReturnType<typeof setTimeout> | undefined
  const deadline = new Promise<never>((_, reject) => {
    timer = setTimeout(() => {
      void queryClient.cancelQueries({ queryKey: queryKeys.legal.versions })
      reject(new ConsentVersionsUnavailableError())
    }, CONSENT_VERSIONS_WAIT_MS)
  })
  try {
    return await Promise.race([read, deadline])
  } catch (error) {
    if (error instanceof ConsentVersionsUnavailableError) throw error
    throw new ConsentVersionsUnavailableError({ cause: error })
  } finally {
    clearTimeout(timer)
  }
}
