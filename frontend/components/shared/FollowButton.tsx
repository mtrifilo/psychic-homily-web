'use client'

import { useState } from 'react'
import { useRouter, usePathname } from 'next/navigation'
import { UserPlus, UserCheck, UserMinus, Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { BracketLink } from './BracketLink'
import { useAuthContext } from '@/lib/context/AuthContext'
import {
  useFollowStatus,
  useFollow,
  useUnfollow,
} from '@/lib/hooks/common/useFollow'
import { replayOnHydrate } from '@/lib/hydration/clickReplay'
import { cn } from '@/lib/utils'

interface FollowButtonProps {
  /** Entity type for URL path (plural: "artists", "venues", "labels", "festivals", "tags", "scenes") */
  entityType: string
  /** Numeric id, or the scene SLUG for entityType "scenes" (PSY-1340). */
  entityId: number | string
  /** true for cards (icon + count only), false for detail pages (icon + text + count) */
  compact?: boolean
  /** Pre-fetched data from batch endpoint, avoids extra request */
  followData?: { follower_count: number; is_following: boolean }
  /**
   * Rendering style. `button` (default) is the shadcn Button used everywhere
   * today. `bracket` renders a `<BracketLink>` for dense entity-page header
   * linkboxes (PSY-641) — `[Follow]` toggling to `[Following]`.
   */
  variant?: 'button' | 'bracket'
  /**
   * Bracket-variant label for the NOT-following state, when the surrounding
   * context does not already say what would be followed (the show page's
   * venue module renders `[Follow venue]` so it cannot be read as following
   * the show). The followed state stays `[Following]` — by then the toggle
   * itself is the antecedent. Ignored by the button variant.
   */
  bracketLabel?: string
  className?: string
  disabled?: boolean
}

export function FollowButton({
  entityType,
  entityId,
  compact = false,
  followData,
  variant = 'button',
  bracketLabel = 'Follow',
  className,
  disabled = false,
}: FollowButtonProps) {
  const router = useRouter()
  const pathname = usePathname()
  const { isAuthenticated, authStatus } = useAuthContext()
  const [isHovering, setIsHovering] = useState(false)

  // The bracket paints no follower count, and a settled-anonymous viewer's
  // `is_following` is false by definition, so the whole response is discarded.
  //
  // The gate is `authStatus === 'anonymous'`, never `!isAuthenticated` or
  // `!isLoading`: both of those read "anonymous" for a signed-in viewer whose
  // profile has not landed, and acting on that misread ships an ENABLED bracket
  // whose replayed pre-hydration click bounces them to /auth.
  //
  // Co-owned: any component observing `queryKeys.follows.entity` for the same
  // entity must not hold that query open for a viewer this one skips it for,
  // or the request moves to the sibling and the shared `fetchStatus` reports
  // `isLoading` here. `<FollowAlertsReveal>` is the only such sibling and gates
  // on the same fact.
  const skipAnonymousStatusFetch =
    variant === 'bracket' && authStatus === 'anonymous'

  // Fetch follow status only if not provided via props.
  const { data: fetchedData, isLoading: statusLoading } = useFollowStatus(
    entityType,
    entityId,
    !followData && !skipAnonymousStatusFetch
  )

  const follow = useFollow()
  const unfollow = useUnfollow()

  // Reading `fetchedData` under the skip is safe because `useFollowStatus`
  // holds the viewer-less key to anonymous data (see its `enabled`). Guarding
  // the read here instead would cover this component and this variant only.
  const data = followData ?? fetchedData
  const isFollowing = data?.is_following ?? false
  const followerCount = data?.follower_count ?? 0
  const isMutating = follow.isPending || unfollow.isPending
  // Cells: pending => disabled for EVERY variant. A control that cannot act
  // must not render actionable; guarding only the click leaves the Button
  // variants enabled and silently inert.
  const isDisabled = disabled || isMutating || authStatus === 'pending'

  // Server-HTML cells, verified on a production build:
  //   signed-in  -> bracket ships from the `statusLoading` branch, `disabled`,
  //                 `pointer-events-none`; no pre-hydration click can land.
  //   anonymous  -> bracket ships ENABLED and carries `replayOnHydrate`, so a
  //                 pre-hydration click replays into `handleClick` and routes
  //                 to /auth. Intended: that is what Follow does for them.
  //   pending    -> disabled (see `isDisabled`).

  const handleClick = (e: React.MouseEvent) => {
    e.preventDefault()
    e.stopPropagation()

    // Unreachable while `isDisabled` includes 'pending'; kept because the
    // redirect below cannot distinguish "no session" from "profile in flight",
    // and it sits ahead of the `isDisabled` bail, which runs after the
    // redirect.
    if (authStatus === 'pending') return

    if (!isAuthenticated) {
      const returnTo = `${pathname}${window.location.search}`
      router.push(`/auth?returnTo=${encodeURIComponent(returnTo)}`)
      return
    }

    if (isDisabled) return

    if (isFollowing) {
      unfollow.mutate({ entityType, entityId })
    } else {
      follow.mutate({ entityType, entityId })
    }
  }

  // Bracket variant — dense header linkbox. Toggles [Follow] ↔ [Following];
  // ignores `compact` (brackets are already maximally compact).
  if (variant === 'bracket') {
    // 'pending' disables regardless of where the data came from. `followData`
    // does not imply settled: the charts pages pass a truthy zeroed fallback
    // while their batch query is disabled, so `statusLoading` is false there
    // and this is the only clause that covers that path.
    if (authStatus === 'pending' || (!followData && statusLoading)) {
      return <BracketLink label={bracketLabel} disabled className={className} />
    }
    return (
      <BracketLink
        label={isFollowing ? 'Following' : bracketLabel}
        active={isFollowing}
        onClick={handleClick}
        disabled={isDisabled}
        className={className}
      />
    )
  }

  // Don't show loading spinner for pre-fetched data
  if (!followData && statusLoading) {
    if (compact) {
      return (
        <Button variant="ghost" size="sm" disabled className="h-7 px-2 gap-1">
          <Loader2 className="h-3.5 w-3.5 animate-spin" />
        </Button>
      )
    }
    return (
      <Button variant="outline" size="sm" disabled className="gap-1.5">
        <Loader2 className="h-4 w-4 animate-spin" />
        <span>Follow</span>
      </Button>
    )
  }

  // Determine the display state
  const showUnfollow = isFollowing && isHovering

  if (compact) {
    return (
      <Button
        {...replayOnHydrate}
        variant={isFollowing ? 'secondary' : 'ghost'}
        size="sm"
        onClick={handleClick}
        onMouseEnter={() => setIsHovering(true)}
        onMouseLeave={() => setIsHovering(false)}
        disabled={isDisabled}
        className={cn(
          'h-7 px-2 gap-1 text-xs',
          showUnfollow && 'text-destructive hover:text-destructive',
          className
        )}
        title={isFollowing ? 'Unfollow' : 'Follow'}
        aria-label={isFollowing ? 'Unfollow' : 'Follow'}
      >
        {isMutating ? (
          <Loader2 className="h-3.5 w-3.5 animate-spin" />
        ) : showUnfollow ? (
          <UserMinus className="h-3.5 w-3.5" />
        ) : isFollowing ? (
          <UserCheck className="h-3.5 w-3.5" />
        ) : (
          <UserPlus className="h-3.5 w-3.5" />
        )}
        {followerCount > 0 && (
          <span className="tabular-nums">{followerCount}</span>
        )}
      </Button>
    )
  }

  return (
    <Button
      {...replayOnHydrate}
      variant={
        isFollowing ? (showUnfollow ? 'destructive' : 'secondary') : 'outline'
      }
      size="sm"
      onClick={handleClick}
      onMouseEnter={() => setIsHovering(true)}
      onMouseLeave={() => setIsHovering(false)}
      disabled={isDisabled}
      className={cn('gap-1.5', className)}
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
      {followerCount > 0 && (
        <span className="text-muted-foreground tabular-nums">
          {followerCount}
        </span>
      )}
    </Button>
  )
}
