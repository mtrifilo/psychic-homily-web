/**
 * PSY-1857 — viewer-tier query caches across an auth-state change.
 *
 * The revision-history cache key carries no viewer dimension, so an admin who
 * opened a History panel while signed out would keep the MASKED payload for
 * the whole 15-minute `staleTime` after logging in, rendered next to a
 * Rollback button that acts on the real stored value. These tests drive the
 * real `useEntityRevisions` observer through the real `useLogin` / `useLogout`
 * mutations against one shared QueryClient, mocking only the HTTP layer.
 */

import { describe, it, expect, vi, beforeEach } from 'vitest'
import type { ReactNode } from 'react'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    AUTH: {
      LOGIN: '/auth/login',
      LOGOUT: '/auth/logout',
      PROFILE: '/auth/profile',
    },
    REVISIONS: {
      ENTITY_HISTORY: (entityType: string, entityId: string | number) =>
        `/revisions/${entityType}/${entityId}`,
      DETAIL: (id: number) => `/revisions/${id}`,
      USER_REVISIONS: (id: string | number) => `/users/${id}/revisions`,
      ROLLBACK: (id: number) => `/revisions/${id}/rollback`,
    },
  },
  API_BASE_URL: 'http://localhost:8080',
}))

vi.mock('@/lib/utils/authLogger', () => ({
  authLogger: {
    debug: vi.fn(),
    info: vi.fn(),
    warn: vi.fn(),
    error: vi.fn(),
    loginAttempt: vi.fn(),
    loginSuccess: vi.fn(),
    loginFailed: vi.fn(),
    logout: vi.fn(),
    profileFetch: vi.fn(),
    tokenRefresh: vi.fn(),
  },
}))

import { useLogin, useLogout } from './useAuth'
import { useEntityRevisions } from '@/lib/hooks/common/useRevisions'
import { refreshViewerTierQueries } from '@/lib/queryClient'

const MASKED_HISTORY = {
  revisions: [
    {
      id: 1,
      entity_type: 'venue',
      entity_id: 7,
      user_id: 3,
      user_name: 'alice',
      user_username: 'alice',
      changes: [
        { field: 'address', old_value: '(hidden)', new_value: '(hidden)' },
      ],
      created_at: '2026-08-01T00:00:00Z',
    },
  ],
  total: 1,
}

const UNMASKED_HISTORY = {
  revisions: [
    {
      id: 1,
      entity_type: 'venue',
      entity_id: 7,
      user_id: 3,
      user_name: 'alice',
      user_username: 'alice',
      changes: [
        {
          field: 'address',
          old_value: '100 Old St',
          new_value: '200 New Ave',
        },
      ],
      summary: 'Fixed the address',
      created_at: '2026-08-01T00:00:00Z',
    },
  ],
  total: 1,
}

/**
 * One QueryClient shared by the revisions observer and the auth mutations,
 * with the production 15-minute staleTime so the test actually exercises the
 * window the bug lived in. Without an explicit invalidation a stale-time-fresh
 * query is never refetched, which is the whole point.
 */
function createClient() {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 30 * 60 * 1000, staleTime: 15 * 60 * 1000 },
      mutations: { retry: false },
    },
  })
}

function wrapperFor(client: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={client}>{children}</QueryClientProvider>
  }
}

/** Serves the anonymous payload until `promote()` flips it to the admin one. */
function historyEndpointStub() {
  let masked = true
  mockApiRequest.mockImplementation(async (url: string) => {
    if (url.startsWith('/revisions/venue/')) {
      return masked ? MASKED_HISTORY : UNMASKED_HISTORY
    }
    if (url === '/auth/login') {
      return { success: true, message: 'ok', user: { id: '3', email: 'a@b.c' } }
    }
    if (url === '/auth/logout') {
      return { success: true, message: 'ok' }
    }
    if (url === '/auth/profile') {
      return {
        success: !masked,
        message: 'ok',
        user: masked ? undefined : { id: '3', email: 'a@b.c', is_admin: true },
      }
    }
    throw new Error(`unexpected request: ${url}`)
  })
  return {
    promote: () => {
      masked = false
    },
    demote: () => {
      masked = true
    },
  }
}

beforeEach(() => {
  mockApiRequest.mockReset()
})

describe('viewer-tier caches across an auth-state change', () => {
  it('login refreshes an already-open revision history to the unmasked view', async () => {
    const client = createClient()
    const wrapper = wrapperFor(client)
    const backend = historyEndpointStub()

    // The admin opens the History panel while signed out.
    const history = renderHook(
      () => useEntityRevisions('venue', 7, { enabled: true }),
      { wrapper }
    )
    await waitFor(() =>
      expect(history.result.current.data).toEqual(MASKED_HISTORY)
    )

    // ...then signs in, without touching the panel or reloading the page.
    const login = renderHook(() => useLogin(), { wrapper })
    backend.promote()
    await act(async () => {
      await login.result.current.mutateAsync({
        email: 'a@b.c',
        password: 'pw',
      })
    })

    await waitFor(() =>
      expect(history.result.current.data).toEqual(UNMASKED_HISTORY)
    )
    expect(
      history.result.current.data?.revisions[0].changes[0].new_value
    ).toBe('200 New Ave')
  })

  it('logout re-masks an already-open revision history', async () => {
    const client = createClient()
    const wrapper = wrapperFor(client)
    const backend = historyEndpointStub()
    backend.promote()

    const history = renderHook(
      () => useEntityRevisions('venue', 7, { enabled: true }),
      { wrapper }
    )
    await waitFor(() =>
      expect(history.result.current.data).toEqual(UNMASKED_HISTORY)
    )

    const logout = renderHook(() => useLogout(), { wrapper })
    backend.demote()
    await act(async () => {
      await logout.result.current.mutateAsync()
    })

    // `queryClient.clear()` drops the privileged payload; the re-render that
    // the logout state change causes rebuilds the query and refetches it as
    // anonymous. Re-render explicitly here because this hook has no auth
    // consumer above it, unlike the entity pages that host the real panel.
    history.rerender()

    await waitFor(() =>
      expect(history.result.current.data).toEqual(MASKED_HISTORY)
    )
  })

  it('a login that fails leaves the anonymous cache untouched', async () => {
    const client = createClient()
    const wrapper = wrapperFor(client)
    historyEndpointStub()

    const history = renderHook(
      () => useEntityRevisions('venue', 7, { enabled: true }),
      { wrapper }
    )
    await waitFor(() =>
      expect(history.result.current.data).toEqual(MASKED_HISTORY)
    )
    const callsBefore = mockApiRequest.mock.calls.length

    mockApiRequest.mockImplementationOnce(async () => ({
      success: false,
      message: 'nope',
      error_code: 'INVALID_CREDENTIALS',
    }))
    const login = renderHook(() => useLogin(), { wrapper })
    await act(async () => {
      await login.result.current
        .mutateAsync({ email: 'a@b.c', password: 'bad' })
        .catch(() => undefined)
    })

    // Only the failed login request itself; no viewer-tier refetch storm.
    expect(mockApiRequest.mock.calls.length).toBe(callsBefore + 1)
    expect(history.result.current.data).toEqual(MASKED_HISTORY)
  })
})

describe('refreshViewerTierQueries', () => {
  it('leaves families that already carry a viewer key segment alone', async () => {
    const client = createClient()
    // savedShows.count and follows.entity encode the viewer in the key, so an
    // auth change re-keys them and they must not be refetched on top of that.
    client.setQueryData(['savedShows', 'count', false, null, 7], { count: 1 })
    client.setQueryData(['follows', 'venue', null, 7], { is_following: false })
    client.setQueryData(['venues', 'detail', 'the-rebel-lounge'], { id: 7 })
    client.setQueryData(['revisions', 'entity', 'venue', '7'], MASKED_HISTORY)

    await refreshViewerTierQueries(client)

    const stale = (key: unknown[]) => client.getQueryState(key)?.isInvalidated
    expect(stale(['revisions', 'entity', 'venue', '7'])).toBe(true)
    expect(stale(['savedShows', 'count', false, null, 7])).toBe(false)
    expect(stale(['follows', 'venue', null, 7])).toBe(false)
    expect(stale(['venues', 'detail', 'the-rebel-lounge'])).toBe(false)
  })

  it('covers every audited viewer-tier family', async () => {
    const client = createClient()
    const audited: [string, unknown[]][] = [
      ['revisions', ['revisions', 'entity', 'venue', '7']],
      ['comments', ['comments', 'venue', 7]],
      ['entity tags', ['tags', 'entityTags', 'venue', 7]],
      ['collections', ['collections', 'detail', 'best-of-2026']],
      ['contributor profile', ['contributor', 'profile', 'alice']],
      ['requests', ['requests', 'list', undefined]],
      ['leaderboard', ['community', 'leaderboard', 'edits', 'month', 10]],
      ['contribute', ['contribute', 'opportunities']],
    ]
    for (const [, key] of audited) client.setQueryData(key, { seeded: true })

    await refreshViewerTierQueries(client)

    for (const [name, key] of audited) {
      expect(
        client.getQueryState(key)?.isInvalidated,
        `${name} should be invalidated on an auth change`
      ).toBe(true)
    }
  })

  it('does not touch the unauthed families that cannot vary by viewer', async () => {
    const client = createClient()
    const exempt: [string, unknown[]][] = [
      ['tag detail', ['tags', 'detail', 'shoegaze']],
      ['public charts', ['charts', 'artists', 'month']],
      ['artist detail', ['artists', 'detail', 'sundressed']],
      ['scene shows', ['scenes', 'shows', 'phoenix', 7, 20]],
    ]
    for (const [, key] of exempt) client.setQueryData(key, { seeded: true })

    await refreshViewerTierQueries(client)

    for (const [name, key] of exempt) {
      expect(
        client.getQueryState(key)?.isInvalidated,
        `${name} is served by an unauthed route and should stay fresh`
      ).toBe(false)
    }
  })
})
