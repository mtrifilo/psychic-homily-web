import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient } from '@tanstack/react-query'
import { createWrapperWithClient } from '@/test/utils'
import { queryKeys } from '@/lib/queryClient'
import {
  useHomeLayout,
  useResetHomeLayout,
  useUpdateHomeLayout,
} from './useHomeLayout'
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
  moveHomeSection(resolveHomeLayout(null), 'radio_shows', 'up')
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

function profilePayload(home_layout: HomeLayoutDocument | null | undefined) {
  return { success: true, user: { id: 7, home_layout } }
}

beforeEach(() => {
  apiRequest.mockReset()
  apiRequest.mockResolvedValue({ success: true, message: 'ok' })
  profile = undefined
})

describe('useHomeLayout', () => {
  it('uses the server-read document until the profile query answers', () => {
    const { result } = renderHook(() => useHomeLayout(CUSTOM), {
      wrapper: createWrapperWithClient(createClient()),
    })

    expect(result.current.map(s => s.id)).toEqual([
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

    expect(result.current.map(s => s.id)).toEqual(
      resolveHomeLayout(null).map(s => s.id)
    )
  })

  it('treats an absent home_layout field on a named viewer as "no layout"', () => {
    profile = profilePayload(undefined)

    const { result } = renderHook(() => useHomeLayout(CUSTOM), {
      wrapper: createWrapperWithClient(createClient()),
    })

    expect(result.current.map(s => s.id)).toEqual(
      resolveHomeLayout(null).map(s => s.id)
    )
  })
})

describe('useUpdateHomeLayout', () => {
  it('PUTs the whole document and writes it into the profile cache first', async () => {
    const queryClient = createClient()
    queryClient.setQueryData(queryKeys.auth.profile, profilePayload(null))

    const { result } = renderHook(() => useUpdateHomeLayout(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    result.current.mutate(CUSTOM)

    await waitFor(() => {
      const cached = queryClient.getQueryData(
        queryKeys.auth.profile
      ) as ReturnType<typeof profilePayload>
      expect(cached.user.home_layout).toEqual(CUSTOM)
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

    const { result } = renderHook(() => useUpdateHomeLayout(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    result.current.mutate(CUSTOM)

    await waitFor(() => expect(result.current.isError).toBe(true))
    const cached = queryClient.getQueryData(
      queryKeys.auth.profile
    ) as ReturnType<typeof profilePayload>
    expect(cached.user.home_layout).toBeNull()
  })

  it('leaves a cache entry that names no viewer alone', async () => {
    const queryClient = createClient()
    const anonymous = { success: false }
    queryClient.setQueryData(queryKeys.auth.profile, anonymous)

    const { result } = renderHook(() => useUpdateHomeLayout(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    result.current.mutate(CUSTOM)

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(queryClient.getQueryData(queryKeys.auth.profile)).toEqual(anonymous)
  })
})

describe('useResetHomeLayout', () => {
  it('DELETEs and clears the stored document optimistically', async () => {
    const queryClient = createClient()
    queryClient.setQueryData(queryKeys.auth.profile, profilePayload(CUSTOM))

    const { result } = renderHook(() => useResetHomeLayout(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    result.current.mutate()

    await waitFor(() => {
      const cached = queryClient.getQueryData(
        queryKeys.auth.profile
      ) as ReturnType<typeof profilePayload>
      expect(cached.user.home_layout).toBeNull()
    })

    const [endpoint, options] = apiRequest.mock.calls[0]
    expect(endpoint).toContain('/auth/preferences/home-layout')
    expect(options).toMatchObject({ method: 'DELETE' })
  })

  it('restores the previous document when the delete fails', async () => {
    apiRequest.mockRejectedValue(new Error('nope'))
    const queryClient = createClient()
    queryClient.setQueryData(queryKeys.auth.profile, profilePayload(CUSTOM))

    const { result } = renderHook(() => useResetHomeLayout(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    result.current.mutate()

    await waitFor(() => expect(result.current.isError).toBe(true))
    const cached = queryClient.getQueryData(
      queryKeys.auth.profile
    ) as ReturnType<typeof profilePayload>
    expect(cached.user.home_layout).toEqual(CUSTOM)
  })
})
