'use client'

import { useCallback, useMemo } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import { queryKeys } from '@/lib/queryClient'
// Concrete module path, not the `@/features/auth` barrel: that barrel is
// mocked wholesale by sibling suites and pulling it in here would ship the
// whole auth surface into the home chunk. See sharedChunkBarrelGuard.test.ts.
import { useProfile } from '@/features/auth/hooks/useAuth'
import {
  resolveHomeLayout,
  type HomeLayoutDocument,
  type ResolvedHomeSection,
} from '../sections'

/**
 * The subset of the profile payload these hooks read and write.
 *
 * The document hangs off `user.preferences`, beside the other stored
 * preferences, NOT off the user. Reading it one level too high resolves to
 * `undefined` for every viewer, which looks exactly like "has no layout".
 */
interface ProfilePreferences {
  home_layout?: HomeLayoutDocument | null
}

interface ProfileWithHomeLayout {
  user?: { preferences?: ProfilePreferences | null } | null
}

/** Shared key so a write can tell whether another write is still in flight. */
const HOME_LAYOUT_MUTATION_KEY = ['auth', 'home-layout'] as const

interface HomeLayoutResponse {
  success: boolean
  message: string
  /** Absent means the shipped default, which is what DELETE restores. */
  home_layout?: HomeLayoutDocument | null
}

/**
 * This viewer's home layout, merged with the registry.
 *
 * `initialLayout` is the document the SERVER read for this request. It is what
 * the first paint renders from, so a viewer with a custom order never sees the
 * shipped order reflow into theirs after hydration. Once the profile query
 * holds a payload (it is hydrated from the same request) that payload wins,
 * which is also how an optimistic write reaches the page.
 */
export function useHomeLayout(
  initialLayout?: HomeLayoutDocument | null
): ResolvedHomeSection[] {
  const { data } = useProfile()
  const profile = data as ProfileWithHomeLayout | undefined
  // A payload that NAMES a viewer is authoritative, absent field included: the
  // backend omits `home_layout` for a viewer who has none, and reading that as
  // "no answer" would fall back to the server document and undo a reset.
  const stored = profile?.user
    ? (profile.user.preferences?.home_layout ?? null)
    : undefined
  return useMemo(
    () => resolveHomeLayout(stored === undefined ? initialLayout : stored),
    [stored, initialLayout]
  )
}

/**
 * Write the whole document, optimistically. `null` resets to the shipped
 * layout, which is a DELETE rather than a PUT of the default: the stored
 * absence is what lets a future default change reach viewers who never
 * customized.
 *
 * The page reads its order from the profile cache, so writing the new document
 * there IS the immediate apply the owner asked for; the request only confirms
 * it. A failure restores the exact snapshot taken before the write, so the rows
 * snap back to what the server still holds.
 */
export function useWriteHomeLayout() {
  const queryClient = useQueryClient()

  return useMutation<
    HomeLayoutResponse,
    Error,
    HomeLayoutDocument | null,
    { previous: unknown }
  >({
    mutationKey: HOME_LAYOUT_MUTATION_KEY,
    mutationFn: (document: HomeLayoutDocument | null) =>
      apiRequest<HomeLayoutResponse>(
        API_ENDPOINTS.AUTH.HOME_LAYOUT,
        document
          ? { method: 'PUT', body: JSON.stringify(document) }
          : { method: 'DELETE' }
      ),
    onMutate: async (document: HomeLayoutDocument | null) => {
      await queryClient.cancelQueries({ queryKey: queryKeys.auth.profile })
      const previous = queryClient.getQueryData(queryKeys.auth.profile)
      queryClient.setQueryData(queryKeys.auth.profile, (old: unknown) =>
        withHomeLayout(old, document)
      )
      return { previous }
    },
    onSuccess: (response: HomeLayoutResponse) => {
      // Both endpoints echo the document they stored, so the cache reconciles
      // from the response instead of refetching the whole profile after every
      // click. An echo can only be trusted when it is the last word: a slower
      // response from an earlier click would otherwise overwrite a later one.
      if (
        queryClient.isMutating({ mutationKey: HOME_LAYOUT_MUTATION_KEY }) > 1
      ) {
        return
      }
      queryClient.setQueryData(queryKeys.auth.profile, (old: unknown) =>
        withHomeLayout(old, response.home_layout ?? null)
      )
    },
    onError: (
      _error: Error,
      _document: HomeLayoutDocument | null,
      context: { previous: unknown } | undefined
    ) => {
      if (context) {
        queryClient.setQueryData(queryKeys.auth.profile, context.previous)
      }
    },
    onSettled: () => {
      // Marked stale, not refetched: the entry is already reconciled, and the
      // next natural read (a remount, a focus) re-validates it for free.
      queryClient.invalidateQueries({
        queryKey: queryKeys.auth.profile,
        refetchType: 'none',
      })
    },
  })
}

/** Write `home_layout` into a cached profile payload without disturbing the
 *  rest of it. A cache entry that names no user is left alone: there is no
 *  viewer to hold a layout. */
function withHomeLayout(
  cached: unknown,
  document: HomeLayoutDocument | null
): unknown {
  const profile = cached as ProfileWithHomeLayout | undefined
  if (!profile?.user) return cached
  return {
    ...profile,
    user: {
      ...profile.user,
      preferences: { ...profile.user.preferences, home_layout: document },
    },
  }
}

/**
 * The one write surface the popover and the settings card share: takes the
 * list the viewer just produced and persists it.
 */
export function usePersistHomeLayout() {
  const { mutate, isPending, isError, variables } = useWriteHomeLayout()
  const reset = useCallback(() => mutate(null), [mutate])

  return {
    persist: mutate,
    reset,
    /** `variables === null` is the reset call; a reorder in flight must not
     *  disable the reset control. */
    isResetting: isPending && variables === null,
    /** One inline line covers every write: the viewer made one gesture and
     *  wants to know whether it stuck. */
    hasError: isError,
  }
}
