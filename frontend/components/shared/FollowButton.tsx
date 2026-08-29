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

  // Skip the status request for a bracket shown to an anonymous viewer: the
  // bracket paints no follower count, and an anonymous viewer's `is_following`
  // is false by definition, so the whole response is discarded.
  //
  // The gate is `authStatus === 'anonymous'` and NOT `!isAuthenticated`, and
  // that difference is the entire point. PSY-1686 tried the latter and reverted
  // it: `isAuthenticated` is false for a SIGNED-IN viewer until their profile
  // round-trip lands, and `isLoading` is false before that fetch even starts, so
  // both misread "signed in, profile pending" as "anonymous". The cost of the
  // misread is not a wasted request — it is an ENABLED bracket in that window,
  // whose replayed pre-hydration click bounces an already-logged-in user to
  // /auth. `authStatus` only reports 'anonymous' once the profile query has
  // resolved to "no user"; while it is 'pending' this stays enabled and the
  // bracket stays disabled, which is PSY-1615's posture unchanged.
  //
  // Known gap, inherited rather than introduced: if the SSR profile prefetch
  // fails (backend 5xx), `prefetchAuthProfile` seeds the anonymous sentinel for
  // a signed-in viewer, so `authStatus` settles 'anonymous' wrongly until
  // staleTime elapses. That viewer is already treated as logged out by every
  // other auth-gated control on the page; this makes the bracket agree with them
  // rather than fetch a status it would then contradict.
  // Co-owned, not local: this skip only holds if no SIBLING observer keeps the
  // same query key alive for the same viewer. `useFollowStatus` keys on
  // (entityType, entityId, viewerId), and viewerId is `undefined` for every
  // anonymous viewer, so a second component beside this one asking for the same
  // entity refills the key, which both spends the request and, because all
  // observers of a query share one `fetchStatus`, drives THIS disabled observer
  // to report `isLoading` and grey the bracket out mid-hydration.
  // `<FollowAlertsReveal>` sits next to the bracket on artist and venue pages
  // and is gated for that reason; anything new that lands in that row needs the
  // same treatment.
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
  // window, so nothing is buffered and replay never applies: for a SIGNED-IN
  // viewer that variant is protected by its own loading state, not by this
  // primitive.
  //
  // "Signed-in" is now a real qualifier, not a redundant one. Since PSY-1867 a
  // SETTLED-ANONYMOUS viewer skips the status query, so their bracket ships
  // ENABLED in server HTML and IS covered by replay (BracketLink carries
  // `replayOnHydrate`). That is the intended outcome: replaying their click
  // reaches `handleClick`, which sends an anonymous viewer to
  // /auth?returnTo=… — exactly what clicking Follow should do for them, and
  // now it survives the hydration window instead of being dropped. The
  // dangerous version of this render is the one PSY-1686 shipped, where the
  // same enabled bracket was handed to a signed-in viewer whose profile had
  // not landed; `authStatus` is what separates the two.
  //
  // The Button variants below do opt in, and are genuinely covered wherever a
  // caller passes `followData` (the charts pages), because that skips the
  // loading branch and renders an enabled control at first paint.

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
