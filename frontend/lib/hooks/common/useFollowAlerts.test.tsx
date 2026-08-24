import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import type { FollowAlertSettings } from '@/lib/types/follow'

// PSY-1893's per-follow alert subscription, read and written by the merged
// Follow control.

const mockApiRequest = vi.fn()

vi.mock('@/lib/api', () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
  API_ENDPOINTS: {
    FOLLOW: {
      ALERTS: (entityType: string, entityId: number | string) =>
        `http://localhost:8080/${entityType}/${entityId}/follow/alerts`,
    },
  },
}))

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({ isAuthenticated: true, user: { id: 1 } }),
}))

import { useFollowAlerts, useUpdateFollowAlerts } from './useFollowAlerts'
import { queryKeys } from '@/lib/queryClient'

const settings: FollowAlertSettings = {
  entity_type: 'artist',
  entity_id: 7,
  shows: { enabled: true, in_app: true, email: false, scope: 'near_me' },
  releases: { enabled: true, in_app: true, email: false },
}

let queryClient: QueryClient

const wrapper = ({ children }: { children: ReactNode }) => (
  <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
)

describe('useFollowAlerts', () => {
  beforeEach(() => {
    mockApiRequest.mockReset()
    queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false, gcTime: Infinity },
        mutations: { retry: false },
      },
    })
  })

  it('reads the resolved subscription from the follow sub-resource', async () => {
    mockApiRequest.mockResolvedValue(settings)

    const { result } = renderHook(() => useFollowAlerts('artists', 7), {
      wrapper,
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      'http://localhost:8080/artists/7/follow/alerts',
      { method: 'GET' }
    )
    expect(result.current.data?.shows.scope).toBe('near_me')
  })

  // Callers gate this on follow state: the endpoint 404s when the follow that
  // IS the subscription does not exist.
  it('does not fire when the caller says the entity is not followed', () => {
    renderHook(() => useFollowAlerts('artists', 7, false), { wrapper })
    expect(mockApiRequest).not.toHaveBeenCalled()
  })

  it('keys by viewer so a logout cannot leak the previous viewer’s subscription', async () => {
    mockApiRequest.mockResolvedValue(settings)
    const { result } = renderHook(() => useFollowAlerts('artists', 7), {
      wrapper,
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(
      queryClient.getQueryData(queryKeys.follows.alerts('artists', 7, 1))
    ).toEqual(settings)
  })
})

describe('useUpdateFollowAlerts', () => {
  beforeEach(() => {
    mockApiRequest.mockReset()
    queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false, gcTime: Infinity },
        mutations: { retry: false },
      },
    })
  })

  it('PATCHes only the axes the caller pinned', async () => {
    mockApiRequest.mockResolvedValue(settings)
    const { result } = renderHook(() => useUpdateFollowAlerts(), { wrapper })

    result.current.mutate({
      entityType: 'artists',
      entityId: 7,
      update: { shows: { enabled: false } },
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(mockApiRequest).toHaveBeenCalledWith(
      'http://localhost:8080/artists/7/follow/alerts',
      { method: 'PATCH', body: JSON.stringify({ shows: { enabled: false } }) }
    )
  })

  // Without the optimistic write the chip snaps back to the pre-click value
  // between the PATCH resolving and the refetch landing.
  it('applies the change to the cache before the request resolves', async () => {
    const key = queryKeys.follows.alerts('artists', 7, 1)
    queryClient.setQueryData(key, settings)
    let resolve: (value: FollowAlertSettings) => void = () => {}
    mockApiRequest.mockReturnValue(
      new Promise<FollowAlertSettings>(r => {
        resolve = r
      })
    )

    const { result } = renderHook(() => useUpdateFollowAlerts(), { wrapper })
    result.current.mutate({
      entityType: 'artists',
      entityId: 7,
      update: { shows: { scope: 'everywhere' } },
    })

    await waitFor(() =>
      expect(
        queryClient.getQueryData<FollowAlertSettings>(key)?.shows.scope
      ).toBe('everywhere')
    )
    // The un-pinned axes survive the optimistic merge.
    expect(queryClient.getQueryData<FollowAlertSettings>(key)?.shows.in_app).toBe(
      true
    )

    resolve({ ...settings, shows: { ...settings.shows, scope: 'everywhere' } })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
  })

  it('rolls the cache back when the write fails', async () => {
    const key = queryKeys.follows.alerts('artists', 7, 1)
    queryClient.setQueryData(key, settings)
    mockApiRequest.mockRejectedValue(new Error('boom'))

    const { result } = renderHook(() => useUpdateFollowAlerts(), { wrapper })
    result.current.mutate({
      entityType: 'artists',
      entityId: 7,
      update: { shows: { scope: 'everywhere' } },
    })

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect(queryClient.getQueryData<FollowAlertSettings>(key)?.shows.scope).toBe(
      'near_me'
    )
  })

  // The server read the merged state back; re-fetching would only re-derive it.
  it('seeds the cache from the response rather than re-fetching', async () => {
    const merged: FollowAlertSettings = {
      ...settings,
      shows: { ...settings.shows, scope: 'everywhere' },
    }
    mockApiRequest.mockResolvedValue(merged)

    const { result } = renderHook(() => useUpdateFollowAlerts(), { wrapper })
    result.current.mutate({
      entityType: 'artists',
      entityId: 7,
      update: { shows: { scope: 'everywhere' } },
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(
      queryClient.getQueryData(queryKeys.follows.alerts('artists', 7, 1))
    ).toEqual(merged)
    expect(mockApiRequest).toHaveBeenCalledTimes(1)
  })
})
