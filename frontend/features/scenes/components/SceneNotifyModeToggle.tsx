'use client'

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import { queryKeys } from '@/lib/queryClient'
import { useAuthContext } from '@/lib/context/AuthContext'
import { useFollowStatus } from '@/lib/hooks/common/useFollow'
import { InfoTooltip } from '@/components/shared/InfoTooltip'
import { AlertChipRadioGroup } from '@/components/shared/AlertChipRadioGroup'

const MODES = [
  { value: 'off', label: 'Off' },
  { value: 'followed_bands_only', label: 'Bands I follow' },
  { value: 'all', label: 'All shows' },
] as const

type SceneNotifyMode = (typeof MODES)[number]['value']

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
  const current = (data.notify_mode || 'all') as SceneNotifyMode

  return (
    <div className="flex flex-wrap items-center gap-1 text-xs">
      {/* Shared with the artist/venue follow's scope reveal (PSY-1905). The
          two were byte-identical copies of the same control; the markup and
          the ARIA contract now live in one place, while each keeps its own
          storage and optimistic-update path, which genuinely differ. */}
      <AlertChipRadioGroup
        ariaLabel="New-show alerts"
        label="Alerts:"
        options={MODES}
        value={current}
        onChange={mode => setMode.mutate(mode)}
        pending={setMode.isPending}
      />
      <InfoTooltip
        label="What do these alerts control?"
        copy="Controls immediate alerts when a new show is added to this scene. It doesn't change the separate weekly Scene digest email."
      />
    </div>
  )
}
