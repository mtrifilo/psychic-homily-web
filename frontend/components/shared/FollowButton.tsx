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
  const { isAuthenticated } = useAuthContext()
  const [isHovering, setIsHovering] = useState(false)

  // Fetch follow status only if not provided via props.
  //
  // A tempting optimization was probed here and REJECTED: skipping this
  // fetch for anonymous bracket viewers (the bracket shows no count, and an
  // anonymous viewer's is_following is false by definition). It cannot be
  // gated safely with what this context exposes — `isAuthenticated` is
  // false for a LOGGED-IN viewer until the profile round-trip lands, and
  // `isLoading` is false before that fetch even starts, so every available
  // predicate misreads "signed in, profile pending" as "anonymous". Acting
  // on that misreading renders an enabled bracket whose replayed
  // pre-hydration click bounces a signed-in user to /auth. Revisit only
  // with a settled-auth signal (tracked separately).
  const { data: fetchedData, isLoading: statusLoading } = useFollowStatus(
    entityType,
    entityId,
    !followData
  )

  const follow = useFollow()
  const unfollow = useUnfollow()

  // Use pre-fetched data if available, otherwise use query data
  const data = followData ?? fetchedData
  const isFollowing = data?.is_following ?? false
  const followerCount = data?.follower_count ?? 0
  const isMutating = follow.isPending || unfollow.isPending
  const isDisabled = disabled || isMutating

  // Replay coverage here is narrower than it looks, so state it precisely.
  //
  // PSY-1610 inferred this control's exposure from SaveButton's rather than
  // clicking it. Checking the actual server HTML of an authenticated artist
  // page (production build) showed the BRACKET variant ships from the
  // `statusLoading` branch below — rendered `disabled`, hence
  // `pointer-events-none`. No click can land on it during the pre-hydration
  // window, so nothing is buffered and replay never applies: on entity pages
  // that variant is protected by its own loading state, not by this primitive.
  //
  // The Button variants below do opt in, and are genuinely covered wherever a
  // caller passes `followData` (the charts pages), because that skips the
  // loading branch and renders an enabled control at first paint.
  //
  // Making the bracket render enabled at first paint would move it under replay
  // too — `handleClick` already guards on `isDisabled` — but that changes what
  // the control shows before its status is known, so it is deliberately left
  // out of PSY-1615's scope. (See also the rejected anonymous-skip above: an
  // enabled first-paint bracket is exactly the render that turns the
  // profile-pending window into a wrong /auth redirect.)

  const handleClick = (e: React.MouseEvent) => {
    e.preventDefault()
    e.stopPropagation()

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
    if (!followData && statusLoading) {
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
