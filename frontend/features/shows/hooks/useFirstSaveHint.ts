'use client'

import { useCallback } from 'react'
import {
  useMutation,
  useQueryClient,
  type QueryClient,
} from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import { queryKeys } from '@/lib/queryClient'
import type { components } from '@/types/api'
import type { SavedShowsListResponse } from '../types'

type DismissFirstSaveHintResponse =
  components['schemas']['DismissFirstSaveHintResponseBody']

/**
 * The subset of the cached profile payload the hint reads and writes. The
 * flag hangs off `user.preferences`, not the user: one level too high resolves
 * to `undefined` for every viewer, which reads as "never dismissed".
 */
interface ProfileWithFirstSaveHint {
  user?: {
    id?: string | number
    preferences?: { first_save_hint_dismissed_at?: string | null } | null
  } | null
}

const FIRST_SAVE_HINT_MUTATION_KEY = ['auth', 'first-save-hint'] as const

/**
 * Whether the viewer the cached profile names has NOT dismissed the hint. A
 * cache entry that names no viewer answers false: there is nobody to show it
 * to, and an unknown answer must not render a once-ever message.
 */
function isHintUndismissed(cached: unknown): boolean {
  const profile = cached as ProfileWithFirstSaveHint | undefined
  if (!profile?.user) return false
  return profile.user.preferences?.first_save_hint_dismissed_at == null
}

/**
 * Called after a save SUCCEEDS: whether that save should open the first-save
 * hint. True only when the viewer has not dismissed the hint and the save
 * took them from zero saved shows to exactly one.
 *
 * The flag is read from the profile cache at call time rather than through a
 * subscription, so the dozens of Save controls on a list page do not each
 * re-render on every profile change. The count is asked only while the flag
 * is unset, so a viewer who has dismissed the hint never pays for the request.
 */
export async function shouldOpenFirstSaveHint(
  queryClient: QueryClient
): Promise<boolean> {
  if (!isHintUndismissed(queryClient.getQueryData(queryKeys.auth.profile))) {
    return false
  }
  const params = new URLSearchParams({ limit: '1', offset: '0' })
  const page = await apiRequest<SavedShowsListResponse>(
    `${API_ENDPOINTS.SAVED_SHOWS.LIST}?${params.toString()}`,
    { method: 'GET' }
  )
  return page.total === 1
}

/** `shouldOpenFirstSaveHint` bound to this tree's query client. */
export function useShouldOpenFirstSaveHint(): () => Promise<boolean> {
  const queryClient = useQueryClient()
  return useCallback(() => shouldOpenFirstSaveHint(queryClient), [queryClient])
}

/**
 * Stamp the hint dismissed on the account, optimistically: the profile cache
 * carries the flag at once, so no other Save control can open the hint again
 * while the request is in flight.
 *
 * A failed write restores the flag and is not surfaced. The hint has already
 * closed at the viewer's request, and the only cost of the failure is that the
 * hint may open again at a later first save.
 */
export function useDismissFirstSaveHint() {
  const queryClient = useQueryClient()

  return useMutation<
    DismissFirstSaveHintResponse,
    Error,
    void,
    { viewerId: unknown; previous: string | null }
  >({
    mutationKey: FIRST_SAVE_HINT_MUTATION_KEY,
    mutationFn: () =>
      apiRequest<DismissFirstSaveHintResponse>(
        API_ENDPOINTS.AUTH.FIRST_SAVE_HINT,
        { method: 'PUT' }
      ),
    onMutate: async () => {
      await queryClient.cancelQueries({ queryKey: queryKeys.auth.profile })
      const cached = queryClient.getQueryData(queryKeys.auth.profile)
      const context = {
        viewerId: readViewerId(cached),
        previous: readDismissedAt(cached),
      }
      queryClient.setQueryData(queryKeys.auth.profile, (old: unknown) =>
        withDismissedAt(old, context.previous ?? new Date().toISOString())
      )
      return context
    },
    onSuccess: (response, _vars, context) => {
      // The server keeps the FIRST dismissal, so its answer can differ from
      // the optimistic stamp. Guarded on the viewer: a write can outlive its
      // session, and the next account's profile must not inherit this flag.
      queryClient.setQueryData(queryKeys.auth.profile, (old: unknown) =>
        readViewerId(old) === context?.viewerId
          ? withDismissedAt(old, response.first_save_hint_dismissed_at)
          : old
      )
    },
    onError: (_error, _vars, context) => {
      if (!context) return
      queryClient.setQueryData(queryKeys.auth.profile, (old: unknown) =>
        readViewerId(old) === context.viewerId
          ? withDismissedAt(old, context.previous)
          : old
      )
    },
  })
}

function readViewerId(cached: unknown): unknown {
  return (cached as ProfileWithFirstSaveHint | undefined)?.user?.id
}

function readDismissedAt(cached: unknown): string | null {
  const profile = cached as ProfileWithFirstSaveHint | undefined
  return profile?.user?.preferences?.first_save_hint_dismissed_at ?? null
}

/** Write the flag into a cached profile payload without disturbing the rest
 *  of it. An entry that names no user is left alone. */
function withDismissedAt(cached: unknown, value: string | null): unknown {
  const profile = cached as ProfileWithFirstSaveHint | undefined
  if (!profile?.user) return cached
  return {
    ...profile,
    user: {
      ...profile.user,
      preferences: {
        ...profile.user.preferences,
        first_save_hint_dismissed_at: value,
      },
    },
  }
}
