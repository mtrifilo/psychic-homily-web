'use client'

import { useCallback, useMemo, useRef } from 'react'
import {
  useMutation,
  useMutationState,
  useQueryClient,
} from '@tanstack/react-query'
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
  user?: {
    id?: string | number
    preferences?: ProfilePreferences | null
  } | null
}

/** Shared key so every surface's write lands in one identifiable group. */
const HOME_LAYOUT_MUTATION_KEY = ['auth', 'home-layout'] as const

interface HomeLayoutResponse {
  success: boolean
  message: string
  /** Absent means the shipped default, which is what DELETE restores. */
  home_layout?: HomeLayoutDocument | null
}

export interface HomeLayoutState {
  sections: ResolvedHomeSection[]
  /**
   * Whether the layout above is this VIEWER's or merely the shipped default
   * standing in until their profile answers.
   *
   * Load-bearing, not cosmetic: every write PUTs the whole document built from
   * `sections`, so a write issued while this is false would persist the
   * default over the layout the viewer actually stored.
   */
  isReady: boolean
  /** Whether a document is STORED for this viewer. Not the same as "differs
   *  from the default": a viewer who customized and changed their mind back
   *  still holds a row, and only DELETE clears it. */
  hasStoredLayout: boolean
}

/**
 * This viewer's home layout, merged with the registry.
 *
 * `initialLayout` is the document the SERVER read for this request, and
 * `undefined` means the server had no answer to give (the Settings page, or a
 * home render whose profile read failed). A server answer is what lets the
 * first paint carry the viewer's own order instead of reflowing into it after
 * hydration; once the profile query holds a payload, that payload wins, which
 * is also how an optimistic write reaches the page.
 */
export function useHomeLayout(
  initialLayout?: HomeLayoutDocument | null
): HomeLayoutState {
  const { data } = useProfile()
  const profile = data as ProfileWithHomeLayout | undefined
  // A payload that NAMES a viewer is authoritative, absent field included: the
  // backend omits `home_layout` for a viewer who has none, and reading that as
  // "no answer" would fall back to the server document and undo a reset.
  const stored = profile?.user
    ? (profile.user.preferences?.home_layout ?? null)
    : undefined
  const answered = stored === undefined ? initialLayout : stored

  return useMemo(
    () => ({
      sections: resolveHomeLayout(answered),
      isReady: answered !== undefined,
      hasStoredLayout: answered != null,
    }),
    [answered]
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
 * it.
 *
 * CONCURRENCY. Reordering a section by three slots is three clicks, so two or
 * more writes in flight is the ordinary path, not an edge case. Writes are
 * therefore stamped with an issue order and only the LAST ISSUED one is
 * allowed to touch the cache when it settles. Without that, a slow response
 * from an earlier click lands after a later one and reverts a gesture the
 * viewer already saw applied, and an earlier failure rolls back to a snapshot
 * that predates a later success. Serializing the requests instead would fix
 * the ordering but delay the optimistic paint behind the previous round trip,
 * which is the one thing this feature may not do.
 */
export function useWriteHomeLayout() {
  const queryClient = useQueryClient()
  const issued = useRef(0)

  return useMutation<
    HomeLayoutResponse,
    Error,
    HomeLayoutDocument | null,
    { issue: number; previousDocument: HomeLayoutDocument | null; viewerId: unknown }
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
      const cached = queryClient.getQueryData(queryKeys.auth.profile)
      const context = {
        issue: ++issued.current,
        // The layout FIELD, not the whole payload: restoring a whole profile
        // snapshot would put a signed-out viewer's identity back into a cache
        // that a logout had already cleared.
        previousDocument: readStoredLayout(cached),
        viewerId: readViewerId(cached),
      }
      queryClient.setQueryData(queryKeys.auth.profile, (old: unknown) =>
        withHomeLayout(old, document)
      )
      return context
    },
    onSuccess: (response: HomeLayoutResponse, _document, context) => {
      // Both endpoints echo the document they stored, so the cache reconciles
      // from the response instead of refetching the whole profile after every
      // click. Only the last-issued write may do so.
      if (!context || context.issue !== issued.current) return
      queryClient.setQueryData(queryKeys.auth.profile, (old: unknown) =>
        withHomeLayout(old, response.home_layout ?? null)
      )
    },
    onError: (_error: Error, _document, context) => {
      if (!context || context.issue !== issued.current) return
      // Guarded on the viewer still being the one whose layout this was. A
      // write can outlive its session: `logout()` clears the cache without
      // cancelling in-flight requests, and an unguarded restore would rebuild
      // the viewer-less profile entry and repaint the signed-out viewer as
      // signed in.
      queryClient.setQueryData(queryKeys.auth.profile, (old: unknown) =>
        readViewerId(old) === context.viewerId
          ? withHomeLayout(old, context.previousDocument)
          : old
      )
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

/**
 * Whether the most recent layout write FAILED, read from the shared mutation
 * cache rather than from one component's observer.
 *
 * The popover unmounts when it closes, taking its observer with it, but the
 * request outlives it. Reading the cache is what lets the always-present
 * toolbar report a failure the viewer would otherwise only see as their change
 * silently undoing itself.
 */
export function useHomeLayoutWriteFailed(): boolean {
  const statuses = useMutationState({
    filters: { mutationKey: HOME_LAYOUT_MUTATION_KEY },
    select: mutation => mutation.state.status,
  })
  return statuses.at(-1) === 'error'
}

function readViewerId(cached: unknown): unknown {
  return (cached as ProfileWithHomeLayout | undefined)?.user?.id
}

function readStoredLayout(cached: unknown): HomeLayoutDocument | null {
  const profile = cached as ProfileWithHomeLayout | undefined
  return profile?.user?.preferences?.home_layout ?? null
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
  const { mutate, isPending, variables } = useWriteHomeLayout()
  const reset = useCallback(() => mutate(null), [mutate])

  return {
    persist: mutate,
    reset,
    /** `variables === null` is the reset call; a reorder in flight must not
     *  disable the reset control. */
    isResetting: isPending && variables === null,
  }
}
