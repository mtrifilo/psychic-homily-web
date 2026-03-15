'use client'

import { useState } from 'react'
import { useRouter } from 'next/navigation'
import { UserPlus, UserCheck, UserMinus, Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { useAuthContext } from '@/lib/context/AuthContext'
import {
  useFollowStatus,
  useFollow,
  useUnfollow,
} from '@/lib/hooks/common/useFollow'
import { cn } from '@/lib/utils'

interface FollowButtonProps {
  /** Entity type for URL path (plural: "artists", "venues", "labels", "festivals") */
  entityType: string
  entityId: number
  /** true for cards (icon + count only), false for detail pages (icon + text + count) */
  compact?: boolean
  /** Pre-fetched data from batch endpoint, avoids extra request */
  followData?: { follower_count: number; is_following: boolean }
}

export function FollowButton({
  entityType,
  entityId,
  compact = false,
  followData,
}: FollowButtonProps) {
  const router = useRouter()
  const { isAuthenticated } = useAuthContext()
  const [isHovering, setIsHovering] = useState(false)

  // Fetch follow status only if not provided via props
  const { data: fetchedData, isLoading: statusLoading } = useFollowStatus(
    entityType,
    entityId
  )

  const follow = useFollow()
  const unfollow = useUnfollow()

  // Use pre-fetched data if available, otherwise use query data
  const data = followData ?? fetchedData
  const isFollowing = data?.is_following ?? false
  const followerCount = data?.follower_count ?? 0
  const isMutating = follow.isPending || unfollow.isPending

  const handleClick = (e: React.MouseEvent) => {
    e.preventDefault()
    e.stopPropagation()

    if (!isAuthenticated) {
      router.push('/auth')
      return
    }

    if (isMutating) return

    if (isFollowing) {
      unfollow.mutate({ entityType, entityId })
    } else {
      follow.mutate({ entityType, entityId })
    }
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
        variant={isFollowing ? 'secondary' : 'ghost'}
        size="sm"
        onClick={handleClick}
        onMouseEnter={() => setIsHovering(true)}
        onMouseLeave={() => setIsHovering(false)}
        disabled={isMutating}
        className={cn(
          'h-7 px-2 gap-1 text-xs',
          showUnfollow && 'text-destructive hover:text-destructive'
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
      variant={isFollowing ? (showUnfollow ? 'destructive' : 'secondary') : 'outline'}
      size="sm"
      onClick={handleClick}
      onMouseEnter={() => setIsHovering(true)}
      onMouseLeave={() => setIsHovering(false)}
      disabled={isMutating}
      className="gap-1.5"
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
