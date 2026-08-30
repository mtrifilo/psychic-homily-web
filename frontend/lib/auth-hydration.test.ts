import { describe, it, expect, vi, beforeEach } from 'vitest'
import { QueryClient } from '@tanstack/react-query'

// The module is server-only by virtue of importing `next/headers`, so the
// cookie store is the one thing that has to be faked to exercise it at all.
const mockGet = vi.fn()
vi.mock('next/headers', () => ({
  cookies: async () => ({ get: mockGet }),
}))

vi.mock('@sentry/nextjs', () => ({
  captureMessage: vi.fn(),
  captureException: vi.fn(),
}))

// `React.cache` memoizes per request; in tests there is no request scope, so
// pass the function through and let each test drive it directly.
vi.mock('react', async () => {
  const actual = await vi.importActual<typeof import('react')>('react')
  return { ...actual, cache: <T,>(fn: T) => fn }
})

// A fresh client per call, which is what `getQueryClient` does ON THE SERVER,
// the environment this server-only module actually runs in. Its real
// implementation branches on `typeof window === 'undefined'`, and under jsdom
// `window` exists, so the unmocked version would hand every test in this file
// the same browser singleton. An entry seeded by an earlier case would then
// still be present for the "seeds NOTHING" cases and fail them for a reason
// that cannot occur in production.
vi.mock('./queryClient', async () => {
  const actual =
    await vi.importActual<typeof import('./queryClient')>('./queryClient')
  return { ...actual, getQueryClient: () => new QueryClient() }
})

import { prefetchAuthProfile, getAuthenticatedNavMode } from './auth-hydration'
import { queryKeys } from './queryClient'

const PROFILE_KEY = JSON.stringify(queryKeys.auth.profile)

interface SeededEntry {
  queryKey: unknown
  state: { data?: { success?: boolean; user?: { id?: string } } }
}

function seededProfileEntry(state: {
  queries: Array<{ queryKey: unknown }>
}): SeededEntry | undefined {
  return state.queries.find(
    q => JSON.stringify(q.queryKey) === PROFILE_KEY
  ) as SeededEntry | undefined
}

describe('prefetchAuthProfile', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.unstubAllGlobals()
  })

  it('seeds the unauthenticated sentinel when there is no cookie', async () => {
    // A definitive answer that needs no backend: no cookie means no session.
    mockGet.mockReturnValue(undefined)

    const state = await prefetchAuthProfile()
    const entry = seededProfileEntry(state as never)

    expect(entry).toBeDefined()
    expect(entry?.state.data?.success).toBe(false)
  })

  it('seeds the real profile on a 200', async () => {
    mockGet.mockReturnValue({ value: 'token' })
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => ({
        ok: true,
        status: 200,
        json: async () => ({ success: true, user: { id: 'u1' } }),
      }))
    )

    const state = await prefetchAuthProfile()
    const entry = seededProfileEntry(state as never)

    expect(entry?.state.data?.user?.id).toBe('u1')
  })

  it('seeds the sentinel on a 401, which IS the backend answering', async () => {
    mockGet.mockReturnValue({ value: 'stale-token' })
    vi.stubGlobal('fetch', vi.fn(async () => ({ ok: false, status: 401 })))

    const state = await prefetchAuthProfile()

    expect(seededProfileEntry(state as never)).toBeDefined()
  })

  // The regression this file exists for. A 5xx used to seed the SAME
  // unauthenticated sentinel a real 401 does, which made a settled "anonymous"
  // forgeable by any transient backend failure, and it did not self-correct:
  // because production runs with `refetchOnWindowFocus: false` and
  // `AuthProvider` mounts once in the root layout.
  it('seeds NOTHING on a 5xx, so the client query mounts pending and asks again', async () => {
    mockGet.mockReturnValue({ value: 'good-token' })
    vi.stubGlobal('fetch', vi.fn(async () => ({ ok: false, status: 503 })))

    const state = await prefetchAuthProfile()

    expect(seededProfileEntry(state as never)).toBeUndefined()
  })

  it('seeds NOTHING when the backend is unreachable', async () => {
    mockGet.mockReturnValue({ value: 'good-token' })
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => {
        throw new Error('ECONNREFUSED')
      })
    )

    const state = await prefetchAuthProfile()

    expect(seededProfileEntry(state as never)).toBeUndefined()
  })
})

describe('getAuthenticatedNavMode', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.unstubAllGlobals()
  })

  it('returns the saved mode for a signed-in viewer', async () => {
    mockGet.mockReturnValue({ value: 'token' })
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => ({
        ok: true,
        status: 200,
        json: async () => ({
          success: true,
          user: { id: 'u1', nav_mode: 'compact' },
        }),
      }))
    )

    await expect(getAuthenticatedNavMode()).resolves.toBe('compact')
  })

  // Unchanged behavior: a failed read already collapsed to undefined here, and
  // the caller's default is the right fallback when there is no answer.
  it('returns undefined when the profile read is indeterminate', async () => {
    mockGet.mockReturnValue({ value: 'token' })
    vi.stubGlobal('fetch', vi.fn(async () => ({ ok: false, status: 500 })))

    await expect(getAuthenticatedNavMode()).resolves.toBeUndefined()
  })
})
