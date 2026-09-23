import { describe, it, expect, vi, beforeEach } from 'vitest'
import { act, renderHook, waitFor } from '@testing-library/react'
import { QueryClient } from '@tanstack/react-query'
import { createWrapperWithClient } from '@/test/utils'
import { queryKeys } from '@/lib/queryClient'
import {
  shouldOpenFirstSaveHint,
  useDismissFirstSaveHint,
} from './useFirstSaveHint'

const apiRequest = vi.fn()
vi.mock('@/lib/api', async importOriginal => ({
  ...(await importOriginal<object>()),
  apiRequest: (...args: unknown[]) => apiRequest(...args),
}))

/** A client whose profile entry survives having no observer (the shared test
 *  client's `gcTime: 0` would collect it the moment `cancelQueries` runs). */
function createClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  })
}

/** The flag hangs off `user.preferences`, beside the other preferences. */
function profilePayload(
  first_save_hint_dismissed_at: string | null | undefined,
  userId: number = 7
) {
  return {
    success: true,
    user: {
      id: userId,
      preferences: { favorite_cities: [], first_save_hint_dismissed_at },
    },
  }
}

function readFlag(client: QueryClient): unknown {
  const cached = client.getQueryData(queryKeys.auth.profile) as
    | ReturnType<typeof profilePayload>
    | undefined
  return cached?.user?.preferences?.first_save_hint_dismissed_at
}

function savedTotal(total: number) {
  return { shows: [], total, limit: 1, offset: 0 }
}

beforeEach(() => {
  apiRequest.mockReset()
})

describe('shouldOpenFirstSaveHint', () => {
  it('opens when the flag is unset and the save made exactly one', async () => {
    const client = createClient()
    client.setQueryData(queryKeys.auth.profile, profilePayload(null))
    apiRequest.mockResolvedValue(savedTotal(1))

    await expect(shouldOpenFirstSaveHint(client)).resolves.toBe(true)
    const [endpoint, options] = apiRequest.mock.calls[0]
    expect(endpoint).toMatch(/\/saved-shows\?limit=1&offset=0$/)
    expect(options).toEqual({ method: 'GET' })
  })

  // A viewer with no preferences row has no `preferences` key at all; that
  // is the never-dismissed state, not an unknown one.
  it('treats absent preferences as never dismissed', async () => {
    const client = createClient()
    client.setQueryData(queryKeys.auth.profile, {
      success: true,
      user: { id: 7 },
    })
    apiRequest.mockResolvedValue(savedTotal(1))

    await expect(shouldOpenFirstSaveHint(client)).resolves.toBe(true)
  })

  it('stays closed on a second save', async () => {
    const client = createClient()
    client.setQueryData(queryKeys.auth.profile, profilePayload(null))
    apiRequest.mockResolvedValue(savedTotal(2))

    await expect(shouldOpenFirstSaveHint(client)).resolves.toBe(false)
  })

  it('stays closed, without asking the count, once dismissed', async () => {
    const client = createClient()
    client.setQueryData(
      queryKeys.auth.profile,
      profilePayload('2026-09-01T12:00:00Z')
    )

    await expect(shouldOpenFirstSaveHint(client)).resolves.toBe(false)
    expect(apiRequest).not.toHaveBeenCalled()
  })

  it('stays closed when no viewer is known', async () => {
    const client = createClient()

    await expect(shouldOpenFirstSaveHint(client)).resolves.toBe(false)
    expect(apiRequest).not.toHaveBeenCalled()
  })
})

describe('useDismissFirstSaveHint', () => {
  it('stamps the cache at once and reconciles to the stored time', async () => {
    const client = createClient()
    client.setQueryData(queryKeys.auth.profile, profilePayload(null))
    let resolve!: (value: unknown) => void
    apiRequest.mockImplementation(
      () => new Promise(r => (resolve = r))
    )

    const { result } = renderHook(() => useDismissFirstSaveHint(), {
      wrapper: createWrapperWithClient(client),
    })
    act(() => result.current.mutate())

    // Optimistic: set before the server answers, so no other Save control can
    // open the hint while the request is in flight.
    await waitFor(() => expect(readFlag(client)).toEqual(expect.any(String)))
    expect(apiRequest).toHaveBeenCalledWith(
      expect.stringMatching(/\/auth\/preferences\/first-save-hint$/),
      { method: 'PUT' }
    )

    await act(async () => {
      resolve({
        success: true,
        first_save_hint_dismissed_at: '2026-09-01T12:00:00Z',
      })
    })
    await waitFor(() => expect(readFlag(client)).toBe('2026-09-01T12:00:00Z'))
    // The rest of the payload is untouched.
    expect(
      (client.getQueryData(queryKeys.auth.profile) as ReturnType<
        typeof profilePayload
      >).user.preferences.favorite_cities
    ).toEqual([])
  })

  it('restores the flag when the write fails', async () => {
    const client = createClient()
    client.setQueryData(queryKeys.auth.profile, profilePayload(null))
    apiRequest.mockRejectedValue(new Error('500'))

    const { result } = renderHook(() => useDismissFirstSaveHint(), {
      wrapper: createWrapperWithClient(client),
    })
    act(() => result.current.mutate())

    await waitFor(() => expect(result.current.isError).toBe(true))
    expect(readFlag(client)).toBeNull()
  })

  // A write can outlive its session: the next account's profile must not
  // inherit this viewer's dismissal.
  it("leaves a different viewer's profile alone when it settles", async () => {
    const client = createClient()
    client.setQueryData(queryKeys.auth.profile, profilePayload(null, 7))
    let resolve!: (value: unknown) => void
    apiRequest.mockImplementation(
      () => new Promise(r => (resolve = r))
    )

    const { result } = renderHook(() => useDismissFirstSaveHint(), {
      wrapper: createWrapperWithClient(client),
    })
    act(() => result.current.mutate())
    await waitFor(() => expect(readFlag(client)).toEqual(expect.any(String)))

    client.setQueryData(queryKeys.auth.profile, profilePayload(null, 8))
    await act(async () => {
      resolve({
        success: true,
        first_save_hint_dismissed_at: '2026-09-01T12:00:00Z',
      })
    })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(readFlag(client)).toBeNull()
  })
})
