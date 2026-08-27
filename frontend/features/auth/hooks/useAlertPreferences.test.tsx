import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'

// PSY-1907's account-level alert preferences: home area + the resolved matrix.

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    AUTH: {
      ALERT_PREFERENCES: 'http://localhost:8080/auth/preferences/alerts',
      HOME_METRO: 'http://localhost:8080/auth/preferences/home-metro',
      ALERT_DEFAULTS: 'http://localhost:8080/auth/preferences/alert-defaults',
    },
  },
}))

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({ isAuthenticated: true, user: { id: 1 } }),
}))

import {
  useAlertPreferences,
  useSetAlertDefaults,
  useSetHomeMetro,
  type AlertPreferences,
} from './useAlertPreferences'
import { queryKeys } from '@/lib/queryClient'

const preferences: AlertPreferences = {
  success: true,
  home_metro: '38060',
  alert_defaults: {
    shows: { in_app: true, email: false },
    releases: { in_app: true, email: false },
  },
}

let queryClient: QueryClient

const wrapper = ({ children }: { children: ReactNode }) => (
  <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
)

beforeEach(() => {
  mockApiRequest.mockReset()
  queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: Infinity },
      mutations: { retry: false },
    },
  })
})

describe('useAlertPreferences', () => {
  it('reads home area and the resolved matrix in one request', async () => {
    mockApiRequest.mockResolvedValue(preferences)
    const { result } = renderHook(() => useAlertPreferences(), { wrapper })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      'http://localhost:8080/auth/preferences/alerts',
      { method: 'GET' }
    )
    expect(result.current.data?.home_metro).toBe('38060')
    expect(result.current.data?.alert_defaults.shows.email).toBe(false)
  })

  it('keys by viewer, like every other per-user preference', async () => {
    mockApiRequest.mockResolvedValue(preferences)
    const { result } = renderHook(() => useAlertPreferences(), { wrapper })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(queryClient.getQueryData(queryKeys.auth.alertPreferences(1))).toEqual(
      preferences
    )
  })
})

describe('useSetHomeMetro', () => {
  it('PUTs the new code and seeds the cache from the response', async () => {
    const updated = { ...preferences, home_metro: '16980' }
    mockApiRequest.mockResolvedValue(updated)

    const { result } = renderHook(() => useSetHomeMetro(), { wrapper })
    result.current.mutate('16980')

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      'http://localhost:8080/auth/preferences/home-metro',
      { method: 'PUT', body: JSON.stringify({ metro: '16980' }) }
    )
    expect(queryClient.getQueryData(queryKeys.auth.alertPreferences(1))).toEqual(
      updated
    )
  })

  it('sends an explicit null to clear the area', async () => {
    mockApiRequest.mockResolvedValue({ ...preferences, home_metro: null })

    const { result } = renderHook(() => useSetHomeMetro(), { wrapper })
    result.current.mutate(null)

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      'http://localhost:8080/auth/preferences/home-metro',
      { method: 'PUT', body: JSON.stringify({ metro: null }) }
    )
  })

  // Every follow's near-me scope resolves against this value, so the
  // per-follow subscriptions the server resolved before the write are stale.
  it('stales every per-follow subscription', async () => {
    const key = queryKeys.follows.alerts('artists', 7, 1)
    queryClient.setQueryData(key, { entity_type: 'artist', entity_id: 7 })
    mockApiRequest.mockResolvedValue(preferences)

    const { result } = renderHook(() => useSetHomeMetro(), { wrapper })
    result.current.mutate('16980')

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(queryClient.getQueryState(key)?.isInvalidated).toBe(true)
  })
})

describe('useSetAlertDefaults', () => {
  it('PATCHes only the channel the caller flipped', async () => {
    mockApiRequest.mockResolvedValue(preferences)

    const { result } = renderHook(() => useSetAlertDefaults(), { wrapper })
    result.current.mutate({ shows: { email: true } })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      'http://localhost:8080/auth/preferences/alert-defaults',
      { method: 'PATCH', body: JSON.stringify({ shows: { email: true } }) }
    )
  })

  // The matrix is the layer every un-overridden follow inherits from.
  it('stales the Library rows, which carry resolved subscriptions', async () => {
    const key = queryKeys.follows.libraryFollowingRoot(1)
    queryClient.setQueryData([...key, 'artist'], { following: [] })
    mockApiRequest.mockResolvedValue(preferences)

    const { result } = renderHook(() => useSetAlertDefaults(), { wrapper })
    result.current.mutate({ shows: { email: true } })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(
      queryClient.getQueryState([...key, 'artist'])?.isInvalidated
    ).toBe(true)
  })
})
