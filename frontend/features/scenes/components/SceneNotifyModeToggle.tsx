'use client'

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import { queryKeys } from '@/lib/queryClient'
import { useAuthContext } from '@/lib/context/AuthContext'
import { useFollowStatus } from '@/lib/hooks/common/useFollow'
import { InfoTooltip } from '@/components/shared/InfoTooltip'
import { cn } from '@/lib/utils'

const MODES = [
  { value: 'off', label: 'Off' },
  { value: 'followed_bands_only', label: 'Bands I follow' },
  { value: 'all', label: 'All shows' },
] as const

/**
 * Scene-follow immediate new-show alert mode (PSY-1341; `off` added in
 * PSY-1466/PSY-1468): once following, choose whether to get alerted on every
 * new show in the metro, only shows featuring bands you already follow (the
 * maintainer-decided semantics from the PSY-1314 spike), or nothing at all.
 * This is scoped to immediate alerts ONLY — it's independent of the separate
 * weekly Scene digest opt-in (account notification settings); muting alerts
 * here does not touch the digest subscription, and vice versa. Renders
 * nothing until the scene is followed — the mode is meaningless before then.
 * Re-POSTing the follow with a mode updates it (the endpoint is idempotent).
 */
export function SceneNotifyModeToggle({ slug }: { slug: string }) {
  const queryClient = useQueryClient()
  const { user } = useAuthContext()
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
      const key = queryKeys.follows.entity('scenes', slug, user?.id)
      await queryClient.cancelQueries({ queryKey: key })
      const previous = queryClient.getQueryData(key)
      queryClient.setQueryData(key, (old: unknown) =>
        old ? { ...(old as object), notify_mode: mode } : old
      )
      return { previous }
    },
    onError: (_err, _mode, context) => {
      if (context?.previous !== undefined) {
        queryClient.setQueryData(
          queryKeys.follows.entity('scenes', slug, user?.id),
          context.previous
        )
      }
    },
    onSettled: () => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.follows.entity('scenes', slug, user?.id),
      })
    },
  })

  if (!data?.is_following) return null
  const current = data.notify_mode || 'all'

  return (
    <div className="flex flex-wrap items-center gap-1 text-xs">
      <div
        role="radiogroup"
        aria-label="New-show alerts"
        className="flex flex-wrap items-center gap-1"
      >
        <span className="text-muted-foreground">Alerts:</span>
        {MODES.map(m => (
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
                : 'border-border text-muted-foreground hover:border-primary/60 hover:text-foreground'
            )}
          >
            {m.label}
          </button>
        ))}
      </div>
      <InfoTooltip
        label="What do these alerts control?"
        copy="Controls immediate alerts when a new show is added to this scene. It doesn't change the separate weekly Scene digest email."
      />
    </div>
  )
}
