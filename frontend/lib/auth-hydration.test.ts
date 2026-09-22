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

import {
  prefetchAuthProfile,
  getAuthenticatedHomeLayout,
  getAuthenticatedNavMode,
  isAuthenticatedViewer,
  prefetchHomeSavedShows,
  resolveHomeViewer,
} from './auth-hydration'
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

// The single switch that decides which homepage a viewer gets (PSY-2103), so
// its fallback direction is worth pinning: it must fail CLOSED, serving the
// anonymous page to any viewer the backend did not positively name.
describe('isAuthenticatedViewer', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.unstubAllGlobals()
  })

  it('is true only when the backend names a user', async () => {
    mockGet.mockReturnValue({ value: 'token' })
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => ({
        ok: true,
        status: 200,
        json: async () => ({ success: true, user: { id: 'u1' } }),
      }))
    )

    await expect(isAuthenticatedViewer()).resolves.toBe(true)
  })

  it('is false with no cookie, without asking the backend', async () => {
    mockGet.mockReturnValue(undefined)
    const fetchSpy = vi.fn()
    vi.stubGlobal('fetch', fetchSpy)

    await expect(isAuthenticatedViewer()).resolves.toBe(false)
    expect(fetchSpy).not.toHaveBeenCalled()
  })

  it('is false when the backend answers that the cookie names nobody', async () => {
    mockGet.mockReturnValue({ value: 'stale' })
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => ({
        ok: false,
        status: 401,
        json: async () => ({ error_code: 'TOKEN_MISSING' }),
      }))
    )

    await expect(isAuthenticatedViewer()).resolves.toBe(false)
  })

  it('fails closed when the read is indeterminate', async () => {
    mockGet.mockReturnValue({ value: 'token' })
    vi.stubGlobal('fetch', vi.fn(async () => ({ ok: false, status: 500 })))

    await expect(isAuthenticatedViewer()).resolves.toBe(false)
  })

  it('fails closed on a 2xx whose body is not a profile', async () => {
    mockGet.mockReturnValue({ value: 'token' })
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => ({
        ok: true,
        status: 200,
        json: async () => ({}),
      }))
    )

    await expect(isAuthenticatedViewer()).resolves.toBe(false)
  })

  it('is false when the backend reports success but names no user', async () => {
    mockGet.mockReturnValue({ value: 'token' })
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => ({
        ok: true,
        status: 200,
        json: async () => ({ success: true }),
      }))
    )

    await expect(isAuthenticatedViewer()).resolves.toBe(false)
  })
})

// The three-way read behind the homepage's variant slot (PSY-2103): only an
// answer the backend could not give falls through to the client-side switch.
describe('resolveHomeViewer', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.unstubAllGlobals()
  })

  it('names a viewer the backend named', async () => {
    mockGet.mockReturnValue({ value: 'token' })
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => ({
        ok: true,
        status: 200,
        json: async () => ({ success: true, user: { id: 'u1' } }),
      }))
    )

    await expect(resolveHomeViewer()).resolves.toBe('authenticated')
  })

  it('is anonymous with no cookie', async () => {
    mockGet.mockReturnValue(undefined)
    vi.stubGlobal('fetch', vi.fn())

    await expect(resolveHomeViewer()).resolves.toBe('anonymous')
  })

  it('is indeterminate when the backend could not answer', async () => {
    mockGet.mockReturnValue({ value: 'token' })
    vi.stubGlobal('fetch', vi.fn(async () => ({ ok: false, status: 500 })))

    await expect(resolveHomeViewer()).resolves.toBe('indeterminate')
  })
})

describe('prefetchHomeSavedShows', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.unstubAllGlobals()
  })

  const savedKey = JSON.stringify(
    queryKeys.savedShows.list('u1', 4, 0, 'upcoming')
  )
  const seeded = (state: { queries: SeededEntry[] }) =>
    state.queries.find(q => JSON.stringify(q.queryKey) === savedKey)

  it('seeds the module\'s exact key from the viewer\'s cookie', async () => {
    mockGet.mockReturnValue({ value: 'token' })
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string) =>
        url.endsWith('/auth/profile')
          ? {
              ok: true,
              status: 200,
              json: async () => ({ success: true, user: { id: 'u1' } }),
            }
          : {
              ok: true,
              status: 200,
              json: async () => ({ shows: [{ id: 7 }], total: 1 }),
            }
      )
    )

    const state = await prefetchHomeSavedShows()

    const entry = seeded(state as { queries: SeededEntry[] })
    expect(entry).toBeDefined()
    expect(entry?.state.data).toEqual({ shows: [{ id: 7 }], total: 1 })
    const calls = (fetch as unknown as { mock: { calls: unknown[][] } }).mock.calls
    expect(String(calls[1][0])).toContain(
      '/saved-shows?limit=4&offset=0&time_filter=upcoming'
    )
  })

  it('seeds nothing when the read fails, leaving the client query to run', async () => {
    mockGet.mockReturnValue({ value: 'token' })
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string) =>
        url.endsWith('/auth/profile')
          ? {
              ok: true,
              status: 200,
              json: async () => ({ success: true, user: { id: 'u1' } }),
            }
          : { ok: false, status: 503 }
      )
    )

    const state = await prefetchHomeSavedShows()

    expect(seeded(state as { queries: SeededEntry[] })).toBeUndefined()
  })

  it('seeds nothing and reads nothing for a viewer with no session', async () => {
    mockGet.mockReturnValue(undefined)
    const fetchSpy = vi.fn()
    vi.stubGlobal('fetch', fetchSpy)

    const state = await prefetchHomeSavedShows()

    expect((state as { queries: SeededEntry[] }).queries).toHaveLength(0)
    expect(fetchSpy).not.toHaveBeenCalled()
  })
})

describe('getAuthenticatedHomeLayout', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.unstubAllGlobals()
  })

  function stubProfile(user: unknown) {
    mockGet.mockReturnValue({ value: 'token' })
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => ({
        ok: true,
        status: 200,
        json: async () => ({ success: true, user }),
      }))
    )
  }

  const document = {
    version: 1,
    sections: [
      { id: 'radio_shows', visible: true },
      { id: 'saved_shows', visible: false },
    ],
  }

  // The shape the backend actually serves: the document hangs off
  // `preferences`, beside favorite_cities and the rest. A reader that looks on
  // the user resolves undefined for every viewer and silently serves the
  // default layout, which is indistinguishable from "has not customized".
  it('reads the document out of user.preferences', async () => {
    stubProfile({ id: 'u1', preferences: { favorite_cities: [], home_layout: document } })

    await expect(getAuthenticatedHomeLayout()).resolves.toEqual(document)
  })

  it('answers null for a viewer who has not customized', async () => {
    stubProfile({ id: 'u1', preferences: { favorite_cities: [] } })

    await expect(getAuthenticatedHomeLayout()).resolves.toBeNull()
  })

  it('answers null for a payload with no preferences at all', async () => {
    stubProfile({ id: 'u1' })

    await expect(getAuthenticatedHomeLayout()).resolves.toBeNull()
  })

  it('answers null rather than a shape the client would have to guess at', async () => {
    stubProfile({ id: 'u1', preferences: { home_layout: 'not-a-document' } })

    await expect(getAuthenticatedHomeLayout()).resolves.toBeNull()
  })

  it('answers null when there is no session', async () => {
    mockGet.mockReturnValue(undefined)

    await expect(getAuthenticatedHomeLayout()).resolves.toBeNull()
  })
})
