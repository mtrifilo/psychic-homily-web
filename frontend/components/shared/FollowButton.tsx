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
  // This gate is only as trustworthy as the signal under it, and an earlier
  // draft of this comment described a gap here as harmless and inherited. It
  // was neither. When the SSR profile prefetch failed, `prefetchAuthProfile`
  // seeded the unauthenticated sentinel for a viewer holding a valid cookie,
  // so `authStatus` settled 'anonymous' for someone signed in, and this skip then
  // shipped them an enabled, replay-covered bracket that bounced them to /auth
  // on a pre-hydration click, and painted [Follow] for something they follow.
  // The fix belongs at the source and lives there now: an SSR profile read that
  // cannot reach the backend seeds NOTHING, so the viewer stays 'pending' and
  // this gate does not fire. See lib/auth-hydration.ts.
  // Co-owned, not local: this skip only holds if no SIBLING observer keeps the
  // same query key alive for the same viewer. `useFollowStatus` keys on
  // (entityType, entityId, viewerId), and viewerId is `undefined` for every
  // anonymous viewer, so a second component beside this one asking for the same
  // entity refills the key, which both spends the request and, because all
  // observers of a query share one `fetchStatus`, drives THIS disabled observer
  // to report `isLoading` and grey the bracket out mid-hydration.
  //
  // State the rule by VARIANT, not by page, because the page list drifts: any
  // component that shares `queryKeys.follows.entity` with a `variant="bracket"`
  // FollowButton has to gate itself the same way. Today the brackets are the
  // show venue module, artist, label and festival pages, and the only sibling
  // observer that lands beside one is `<FollowAlertsReveal>` on the artist page
  // (it also appears on the venue page, but that page renders the BUTTON
  // variant, which never skips, so nothing there depends on the gate).
  // `SceneNotifyModeToggle` is a third observer of this key family and is
  // deliberately ungated: scene pages render the button variant too.
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

    // Never route an unsettled viewer. `isAuthenticated` is false both for a
    // viewer who has no session and for one whose profile has not arrived, and
    // the redirect below cannot tell them apart, so without this line a
    // signed-in viewer clicking during the pending window is sent to /auth.
    //
    // Deliberately the FIRST statement, ahead of the redirect and ahead of the
    // `isDisabled` bail. Until now the pending window was survivable only
    // because every path that could reach this function happened to render a
    // disabled control first; that made the invariant depend on three other
    // files (the loading render branch, BracketLink's `pointer-events-none`,
    // and the replay helper's disabled check) and it did not actually hold on
    // the `followData` path. Doing nothing is right for a click we cannot yet
    // interpret: the viewer can click again a moment later.
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
    // Unsettled auth disables the bracket REGARDLESS of where its data came
    // from. The `followData` clause below is not a substitute for this, and
    // that was a real hole: on the charts pages `useBatchFollowStatus` is
    // handed an empty id list while `isAuthenticated` is false, so the batch
    // query is disabled, its `isLoading` reads false, and the caller falls back
    // to a truthy zeroed `followData`. Every one of those brackets therefore
    // shipped ENABLED during the pending window, and a replayed pre-hydration
    // click on one sent a signed-in viewer to /auth: the exact failure this
    // ticket exists to close, on the one path the fetch gate never touched.
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
