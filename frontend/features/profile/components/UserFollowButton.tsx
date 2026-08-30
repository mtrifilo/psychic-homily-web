'use client'

import { useState } from 'react'
import { useRouter, usePathname } from 'next/navigation'
import { UserPlus, UserCheck, UserMinus, Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { useAuthContext } from '@/lib/context/AuthContext'
import {
  useUserFollowStatus,
  useUserFollow,
  useUserUnfollow,
} from '@/lib/hooks/common/useUserFollow'
import { cn } from '@/lib/utils'
import { replayOnHydrate } from '@/lib/hydration/clickReplay'
import { useAutoDismissBanner } from '@/lib/hooks/common'

// How long a follow/unfollow failure stays on screen before auto-hiding.
const ERROR_DISMISS_MS = 3000

interface UserFollowButtonProps {
  username: string
  className?: string
}

/**
 * Visitor Follow / Following toggle for public profile headers.
 * Username-addressed (POST/DELETE /users/{username}/follow) — not the
 * entity follow routes. Logged-out click sends the viewer to sign-in;
 * owners never render this (PublicProfile gates on !isOwner).
 */
export function UserFollowButton({
  username,
  className,
}: UserFollowButtonProps) {
  const router = useRouter()
  const pathname = usePathname()
  const { isAuthenticated, authStatus } = useAuthContext()
  const [isHovering, setIsHovering] = useState(false)
  // Shared auto-dismiss primitive rather than a hand-rolled timer, which must
  // not outlive unmount. See useAutoDismissBanner / useDismissTimer (PSY-1664).
  const {
    value: errorAction,
    show: showErrorAction,
    clear: clearErrorAction,
  } = useAutoDismissBanner<'follow' | 'unfollow'>(ERROR_DISMISS_MS)

  // This control paints no follower count, and a settled-anonymous viewer's
  // `is_following` is false by definition, so the whole response is discarded
  // for them. The gate is `authStatus === 'anonymous'`, never `!isAuthenticated`
  // or `!isLoading`: both read "anonymous" for a signed-in viewer whose profile
  // has not landed, and acting on that misread skips the request that decides
  // whether this reads Follow or Following.
  const { data, isLoading: statusLoading } = useUserFollowStatus(
    username,
    authStatus !== 'anonymous'
  )
  const follow = useUserFollow()
  const unfollow = useUserUnfollow()

  const isFollowing = data?.is_following ?? false
  const isMutating = follow.isPending || unfollow.isPending
  // Disabled while unsettled, as every control in this class is: it ships
  // ENABLED in server HTML with pre-hydration click replay, and its handler
  // routes on `!isAuthenticated`, which reads false for a signed-in viewer whose
  // profile has not landed. See AuthStatus in lib/context/AuthContext.
  const isDisabled = isMutating || authStatus === 'pending'

  const handleClick = () => {
    // Unreachable while the control renders disabled; defence in depth for the
    // redirect below, which cannot tell "no session" from "profile in flight".
    if (authStatus === 'pending') return

    if (!isAuthenticated) {
      const returnTo = `${pathname}${window.location.search}`
      router.push(`/auth?returnTo=${encodeURIComponent(returnTo)}`)
      return
    }

    if (isDisabled) return

    clearErrorAction()
    const action = isFollowing ? 'unfollow' : 'follow'
    const onError = () => {
      showErrorAction(action)
    }

    if (isFollowing) {
      unfollow.mutate(username, { onError })
    } else {
      follow.mutate(username, { onError })
    }
  }

  if (statusLoading) {
    return (
      <Button variant="outline" size="sm" disabled className="gap-1.5">
        <Loader2 className="h-4 w-4 animate-spin" />
        <span>Follow</span>
      </Button>
    )
  }

  const showUnfollow = isFollowing && isHovering

  return (
    <div className="relative">
      <Button
        {...replayOnHydrate}
        type="button"
        variant={
          isFollowing ? (showUnfollow ? 'destructive' : 'secondary') : 'outline'
        }
        size="sm"
        onClick={handleClick}
        onMouseEnter={() => setIsHovering(true)}
        onMouseLeave={() => setIsHovering(false)}
        disabled={isDisabled}
        className={cn('gap-1.5', className)}
        // `authStatus === 'anonymous'`, not `!isAuthenticated`: the sign-in
        // wording is a claim about the viewer, and the unsettled window is not
        // yet entitled to make it.
        aria-label={
          authStatus === 'anonymous'
            ? 'Sign in to follow'
            : showUnfollow
              ? 'Unfollow'
              : isFollowing
                ? 'Following'
                : 'Follow'
        }
      >
        {isMutating ? (
          <Loader2 className="h-4 w-4 animate-spin" />
        ) : showUnfollow ? (
          <UserMinus className="h-4 w-4" />
        ) : isFollowing ? (
          <UserCheck className="h-4 w-4" />
        ) : (
          <UserPlus className="h-4 w-4" />
        )}
        <span>
          {showUnfollow ? 'Unfollow' : isFollowing ? 'Following' : 'Follow'}
        </span>
      </Button>
      {errorAction ? (
        <div className="absolute left-1/2 top-full z-50 mt-2 -translate-x-1/2 whitespace-nowrap rounded-md bg-destructive px-3 py-1.5 text-xs text-destructive-foreground shadow-sm">
          Failed to {errorAction}
        </div>
      ) : null}
    </div>
  )
}
