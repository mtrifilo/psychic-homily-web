'use client'

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import { queryKeys } from '@/lib/queryClient'
import { useFollowStatus } from '@/lib/hooks/common/useFollow'
import { cn } from '@/lib/utils'

const MODES = [
  { value: 'all', label: 'All shows' },
  { value: 'followed_bands_only', label: 'Bands I follow' },
] as const

/**
 * Scene-follow notification mode (PSY-1341): once following, choose between
 * every new show in the metro or only shows featuring bands you already
 * follow (the maintainer-decided semantics from the PSY-1314 spike). Renders
 * nothing until the scene is followed — the mode is meaningless before then.
 * Re-POSTing the follow with a mode updates it (the endpoint is idempotent).
 */
export function SceneNotifyModeToggle({ slug }: { slug: string }) {
  const queryClient = useQueryClient()
  const { data } = useFollowStatus('scenes', slug)

  const setMode = useMutation({
    mutationFn: async (mode: string) =>
      apiRequest(API_ENDPOINTS.FOLLOW.ENTITY('scenes', slug), {
        method: 'POST',
        body: JSON.stringify({ notify_mode: mode }),
      }),
    // Optimistic: without this the radio snaps back to the stale cached mode
    // between the POST resolving and the invalidation refetch landing, which
    // reads as "the click didn't take" (review-caught).
    onMutate: async (mode: string) => {
      const key = queryKeys.follows.entity('scenes', slug)
      await queryClient.cancelQueries({ queryKey: key })
      const previous = queryClient.getQueryData(key)
      queryClient.setQueryData(key, (old: unknown) =>
        old ? { ...(old as object), notify_mode: mode } : old,
      )
      return { previous }
    },
    onError: (_err, _mode, context) => {
      if (context?.previous !== undefined) {
        queryClient.setQueryData(
          queryKeys.follows.entity('scenes', slug),
          context.previous,
        )
      }
    },
    onSettled: () => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.follows.entity('scenes', slug),
      })
    },
  })

  if (!data?.is_following) return null
  const current = data.notify_mode || 'all'

  return (
    <div
      role="radiogroup"
      aria-label="New-show notifications"
      className="flex items-center gap-1 text-xs"
    >
      <span className="text-muted-foreground">Notify:</span>
      {MODES.map((m) => (
        <button
          key={m.value}
          type="button"
          role="radio"
          aria-checked={current === m.value}
          disabled={setMode.isPending}
          onClick={() => {
            if (current !== m.value) setMode.mutate(m.value)
          }}
          className={cn(
            'rounded-full border px-2 py-0.5 transition-colors',
            current === m.value
              ? 'border-primary text-foreground'
              : 'border-border text-muted-foreground hover:border-primary/60 hover:text-foreground',
          )}
        >
          {m.label}
        </button>
      ))}
    </div>
  )
}
