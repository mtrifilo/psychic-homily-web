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
  const { isAuthenticated } = useAuthContext()
  const [isHovering, setIsHovering] = useState(false)
  const [errorAction, setErrorAction] = useState<'follow' | 'unfollow' | null>(
    null
  )

  const { data, isLoading: statusLoading } = useUserFollowStatus(username)
  const follow = useUserFollow()
  const unfollow = useUserUnfollow()

  const isFollowing = data?.is_following ?? false
  const isMutating = follow.isPending || unfollow.isPending
  const isDisabled = isMutating

  const handleClick = () => {
    if (!isAuthenticated) {
      const returnTo = `${pathname}${window.location.search}`
      router.push(`/auth?returnTo=${encodeURIComponent(returnTo)}`)
      return
    }

    if (isDisabled) return

    setErrorAction(null)
    const action = isFollowing ? 'unfollow' : 'follow'
    const onError = () => {
      setErrorAction(action)
      setTimeout(() => setErrorAction(null), 3000)
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
        aria-label={
          !isAuthenticated
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
