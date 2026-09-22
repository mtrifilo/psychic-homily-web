import { describe, it, expect, vi, beforeEach } from 'vitest'
import { act, renderHook, waitFor } from '@testing-library/react'
import { QueryClient } from '@tanstack/react-query'
import { createWrapperWithClient } from '@/test/utils'
import { queryKeys } from '@/lib/queryClient'
import { useHomeLayout, useWriteHomeLayout } from './useHomeLayout'
import {
  moveHomeSection,
  resolveHomeLayout,
  toHomeLayoutDocument,
  type HomeLayoutDocument,
} from '../sections'

const apiRequest = vi.fn()
vi.mock('@/lib/api', async importOriginal => ({
  ...(await importOriginal<object>()),
  apiRequest: (...args: unknown[]) => apiRequest(...args),
}))

let profile: unknown
vi.mock('@/features/auth/hooks/useAuth', () => ({
  useProfile: () => ({ data: profile }),
}))

const CUSTOM: HomeLayoutDocument = toHomeLayoutDocument(
  moveHomeSection(resolveHomeLayout(null), 'radio_shows', 'up')!
)

const CUSTOM_TWO_STEPS: HomeLayoutDocument = toHomeLayoutDocument(
  moveHomeSection(
    moveHomeSection(resolveHomeLayout(null), 'radio_shows', 'up')!,
    'radio_shows',
    'up'
  )!
)

/**
 * A client whose profile entry SURVIVES having no observer. The shared test
 * client sets `gcTime: 0`, which collects the entry these hooks write into the
 * moment `cancelQueries` touches it, so an optimistic write would read back as
 * undefined for reasons that have nothing to do with the hook.
 */
function createClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  })
}

/** The document hangs off `user.preferences`, where the backend stores it
 *  beside the viewer's other preferences. */
function profilePayload(home_layout: HomeLayoutDocument | null | undefined) {
  return {
    success: true,
    user: { id: 7, preferences: { favorite_cities: [], home_layout } },
  }
}

beforeEach(() => {
  apiRequest.mockReset()
  // Both endpoints echo the document they stored; the hook reconciles from it.
  apiRequest.mockImplementation(
    async (_endpoint: string, options: { method: string; body?: string }) => ({
      success: true,
      message: 'ok',
      ...(options.method === 'PUT'
        ? { home_layout: JSON.parse(options.body ?? 'null') }
        : {}),
    })
  )
  profile = undefined
})

describe('useHomeLayout', () => {
  it('uses the server-read document until the profile query answers', () => {
    const { result } = renderHook(() => useHomeLayout(CUSTOM), {
      wrapper: createWrapperWithClient(createClient()),
    })

    expect(result.current.sections.map(section => section.id)).toEqual([
      'saved_shows',
      'nearby_shows',
      'community_stats',
      'radio_shows',
      'city_graph',
    ])
  })

  it('prefers the profile payload once it names a viewer', () => {
    profile = profilePayload(null)

    const { result } = renderHook(() => useHomeLayout(CUSTOM), {
      wrapper: createWrapperWithClient(createClient()),
    })

    expect(result.current.sections.map(section => section.id)).toEqual(
      resolveHomeLayout(null).map(s => s.id)
    )
  })

  it('treats an absent home_layout field on a named viewer as "no layout"', () => {
    profile = profilePayload(undefined)

    const { result } = renderHook(() => useHomeLayout(CUSTOM), {
      wrapper: createWrapperWithClient(createClient()),
    })

    expect(result.current.sections.map(section => section.id)).toEqual(
      resolveHomeLayout(null).map(s => s.id)
    )
  })
})

describe('useWriteHomeLayout', () => {
  it('PUTs the whole document and writes it into the profile cache first', async () => {
    const queryClient = createClient()
    queryClient.setQueryData(queryKeys.auth.profile, profilePayload(null))

    const { result } = renderHook(() => useWriteHomeLayout(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    result.current.mutate(CUSTOM)

    await waitFor(() => {
      const cached = queryClient.getQueryData(
        queryKeys.auth.profile
      ) as ReturnType<typeof profilePayload>
      expect(cached.user.preferences.home_layout).toEqual(CUSTOM)
    })

    await waitFor(() => expect(apiRequest).toHaveBeenCalled())
    const [endpoint, options] = apiRequest.mock.calls[0]
    expect(endpoint).toContain('/auth/preferences/home-layout')
    expect(options).toMatchObject({ method: 'PUT' })
    expect(JSON.parse(options.body)).toEqual(CUSTOM)
  })

  it('snaps the cache back when the write fails', async () => {
    apiRequest.mockRejectedValue(new Error('nope'))
    const queryClient = createClient()
    queryClient.setQueryData(queryKeys.auth.profile, profilePayload(null))

    const { result } = renderHook(() => useWriteHomeLayout(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    result.current.mutate(CUSTOM)

    await waitFor(() => expect(result.current.isError).toBe(true))
    const cached = queryClient.getQueryData(
      queryKeys.auth.profile
    ) as ReturnType<typeof profilePayload>
    expect(cached.user.preferences.home_layout).toBeNull()
  })

  it('leaves the viewer\'s other preferences untouched', async () => {
    const queryClient = createClient()
    queryClient.setQueryData(queryKeys.auth.profile, profilePayload(null))

    const { result } = renderHook(() => useWriteHomeLayout(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    result.current.mutate(CUSTOM)

    await waitFor(() => {
      const cached = queryClient.getQueryData(
        queryKeys.auth.profile
      ) as ReturnType<typeof profilePayload>
      expect(cached.user.preferences.home_layout).toEqual(CUSTOM)
    })
    const cached = queryClient.getQueryData(
      queryKeys.auth.profile
    ) as ReturnType<typeof profilePayload>
    expect(cached.user.preferences.favorite_cities).toEqual([])
    expect(cached.user.id).toBe(7)
  })

  it('leaves a cache entry that names no viewer alone', async () => {
    const queryClient = createClient()
    const anonymous = { success: false }
    queryClient.setQueryData(queryKeys.auth.profile, anonymous)

    const { result } = renderHook(() => useWriteHomeLayout(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    result.current.mutate(CUSTOM)

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(queryClient.getQueryData(queryKeys.auth.profile)).toEqual(anonymous)
  })
})

describe('useWriteHomeLayout, reset', () => {
  it('DELETEs on a null document and clears the stored one optimistically', async () => {
    const queryClient = createClient()
    queryClient.setQueryData(queryKeys.auth.profile, profilePayload(CUSTOM))

    const { result } = renderHook(() => useWriteHomeLayout(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    result.current.mutate(null)

    await waitFor(() => {
      const cached = queryClient.getQueryData(
        queryKeys.auth.profile
      ) as ReturnType<typeof profilePayload>
      expect(cached.user.preferences.home_layout).toBeNull()
    })

    const [endpoint, options] = apiRequest.mock.calls[0]
    expect(endpoint).toContain('/auth/preferences/home-layout')
    expect(options).toMatchObject({ method: 'DELETE' })
  })

  it('restores the previous document when the reset fails', async () => {
    apiRequest.mockRejectedValue(new Error('nope'))
    const queryClient = createClient()
    queryClient.setQueryData(queryKeys.auth.profile, profilePayload(CUSTOM))

    const { result } = renderHook(() => useWriteHomeLayout(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    result.current.mutate(null)

    await waitFor(() => expect(result.current.isError).toBe(true))
    const cached = queryClient.getQueryData(
      queryKeys.auth.profile
    ) as ReturnType<typeof profilePayload>
    expect(cached.user.preferences.home_layout).toEqual(CUSTOM)
  })
})

describe('the server echo', () => {
  it('reconciles the cache from the response rather than refetching the profile', async () => {
    const queryClient = createClient()
    queryClient.setQueryData(queryKeys.auth.profile, profilePayload(null))
    const refetch = vi.spyOn(queryClient, 'refetchQueries')

    const { result } = renderHook(() => useWriteHomeLayout(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    result.current.mutate(CUSTOM)
    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    const cached = queryClient.getQueryData(
      queryKeys.auth.profile
    ) as ReturnType<typeof profilePayload>
    expect(cached.user.preferences.home_layout).toEqual(CUSTOM)
    // One PUT, and no second trip for the profile it already answered with.
    expect(apiRequest).toHaveBeenCalledTimes(1)
    expect(refetch).not.toHaveBeenCalled()
  })

  it('takes an absent home_layout in the echo as the shipped default', async () => {
    const queryClient = createClient()
    queryClient.setQueryData(queryKeys.auth.profile, profilePayload(CUSTOM))

    const { result } = renderHook(() => useWriteHomeLayout(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    result.current.mutate(null)
    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    const cached = queryClient.getQueryData(
      queryKeys.auth.profile
    ) as ReturnType<typeof profilePayload>
    expect(cached.user.preferences.home_layout).toBeNull()
  })
})

describe('concurrent writes', () => {
  /** A PUT whose response the test releases by hand. */
  function deferredApi() {
    const pending: Array<{
      body: unknown
      resolve: (value: unknown) => void
      reject: (error: Error) => void
    }> = []
    apiRequest.mockImplementation(
      (_endpoint: string, options: { method: string; body?: string }) =>
        new Promise((resolve, reject) => {
          pending.push({
            body: options.body ? JSON.parse(options.body) : null,
            resolve,
            reject,
          })
        })
    )
    return pending
  }

  // Moving a section three slots is three clicks, so two writes in flight is
  // the ordinary path. Only the LAST ISSUED write may touch the cache when it
  // settles; otherwise a slow response from an earlier click reverts a gesture
  // the viewer already watched apply.
  it('ignores an earlier write that answers after a later one', async () => {
    const pending = deferredApi()
    const queryClient = createClient()
    queryClient.setQueryData(queryKeys.auth.profile, profilePayload(null))

    const { result } = renderHook(() => useWriteHomeLayout(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    const first = toHomeLayoutDocument(
      moveHomeSection(resolveHomeLayout(null), 'radio_shows', 'up')!
    )
    act(() => result.current.mutate(first))
    await waitFor(() => expect(pending).toHaveLength(1))
    act(() => result.current.mutate(CUSTOM_TWO_STEPS))
    await waitFor(() => expect(pending).toHaveLength(2))

    // The LATER write answers first, then the earlier one.
    await act(async () => {
      pending[1].resolve({ success: true, message: 'ok', home_layout: CUSTOM_TWO_STEPS })
      pending[0].resolve({ success: true, message: 'ok', home_layout: first })
    })

    const cached = queryClient.getQueryData(
      queryKeys.auth.profile
    ) as ReturnType<typeof profilePayload>
    expect(cached.user.preferences.home_layout).toEqual(CUSTOM_TWO_STEPS)
  })

  // Symmetrically: an earlier FAILURE must not roll back to a snapshot that
  // predates a later write the viewer has already seen applied.
  it('ignores an earlier failure that lands after a later write', async () => {
    const pending = deferredApi()
    const queryClient = createClient()
    queryClient.setQueryData(queryKeys.auth.profile, profilePayload(null))

    const { result } = renderHook(() => useWriteHomeLayout(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    act(() => result.current.mutate(CUSTOM))
    await waitFor(() => expect(pending).toHaveLength(1))
    act(() => result.current.mutate(CUSTOM_TWO_STEPS))
    await waitFor(() => expect(pending).toHaveLength(2))

    await act(async () => {
      pending[1].resolve({ success: true, message: 'ok', home_layout: CUSTOM_TWO_STEPS })
      pending[0].reject(new Error('nope'))
    })

    const cached = queryClient.getQueryData(
      queryKeys.auth.profile
    ) as ReturnType<typeof profilePayload>
    expect(cached.user.preferences.home_layout).toEqual(CUSTOM_TWO_STEPS)
  })
})

describe('a write that outlives its session', () => {
  // `logout()` clears the query cache without cancelling in-flight requests.
  // Restoring a whole profile snapshot in onError would REBUILD the cleared
  // entry and repaint the signed-out viewer as signed in.
  it('does not put the previous viewer back into a cleared cache', async () => {
    apiRequest.mockRejectedValue(new Error('401'))
    const queryClient = createClient()
    queryClient.setQueryData(queryKeys.auth.profile, profilePayload(null))

    const { result } = renderHook(() => useWriteHomeLayout(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    result.current.mutate(CUSTOM)
    // The viewer signs out while the PUT is in flight.
    queryClient.clear()

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect(queryClient.getQueryData(queryKeys.auth.profile)).toBeUndefined()
  })
})
