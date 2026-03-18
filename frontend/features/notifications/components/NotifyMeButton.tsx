'use client'

import { useState } from 'react'
import { useRouter } from 'next/navigation'
import { Bell, BellRing, Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { useAuthContext } from '@/lib/context/AuthContext'
import {
  useNotificationFilterCheck,
  useQuickCreateFilter,
  useDeleteFilter,
} from '../hooks'
import type { NotifyEntityType } from '../types'
import { cn } from '@/lib/utils'

interface NotifyMeButtonProps {
  entityType: NotifyEntityType
  entityId: number
  entityName: string
  /** Compact mode for tighter layouts */
  compact?: boolean
}

const entityLabels: Record<NotifyEntityType, string> = {
  artist: 'Notify me about',
  venue: 'Notify me about shows at',
  label: 'Notify me about',
  tag: 'Notify me about',
}

export function NotifyMeButton({
  entityType,
  entityId,
  entityName,
  compact = false,
}: NotifyMeButtonProps) {
  const router = useRouter()
  const { isAuthenticated } = useAuthContext()
  const [isHovering, setIsHovering] = useState(false)

  const { data: matchingFilter, hasFilter, isLoading: checkLoading } =
    useNotificationFilterCheck(entityType, entityId)

  const quickCreate = useQuickCreateFilter()
  const deleteFilter = useDeleteFilter()

  const isMutating = quickCreate.isPending || deleteFilter.isPending

  const handleClick = (e: React.MouseEvent) => {
    e.preventDefault()
    e.stopPropagation()

    if (!isAuthenticated) {
      router.push('/auth')
      return
    }

    if (isMutating) return

    if (hasFilter && matchingFilter) {
      deleteFilter.mutate(matchingFilter.id)
    } else {
      quickCreate.mutate({ entityType, entityId })
    }
  }

  // Don't render for unauthenticated users in loading state
  if (!isAuthenticated) {
    if (compact) {
      return (
        <Button
          variant="ghost"
          size="sm"
          onClick={() => router.push('/auth')}
          className="h-7 px-2 gap-1 text-xs"
          title="Sign in to get notifications"
        >
          <Bell className="h-3.5 w-3.5" />
        </Button>
      )
    }
    return (
      <Button
        variant="outline"
        size="sm"
        onClick={() => router.push('/auth')}
        className="gap-1.5"
      >
        <Bell className="h-4 w-4" />
        <span>Notify me</span>
      </Button>
    )
  }

  if (checkLoading) {
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
        <span>Notify me</span>
      </Button>
    )
  }

  const showRemove = hasFilter && isHovering

  if (compact) {
    return (
      <Button
        variant={hasFilter ? 'secondary' : 'ghost'}
        size="sm"
        onClick={handleClick}
        onMouseEnter={() => setIsHovering(true)}
        onMouseLeave={() => setIsHovering(false)}
        disabled={isMutating}
        className={cn(
          'h-7 px-2 gap-1 text-xs',
          showRemove && 'text-destructive hover:text-destructive'
        )}
        title={
          hasFilter
            ? `Notifications on for ${entityName}`
            : `${entityLabels[entityType]} ${entityName}`
        }
        aria-label={hasFilter ? 'Remove notification' : 'Notify me'}
      >
        {isMutating ? (
          <Loader2 className="h-3.5 w-3.5 animate-spin" />
        ) : hasFilter ? (
          <BellRing className="h-3.5 w-3.5" />
        ) : (
          <Bell className="h-3.5 w-3.5" />
        )}
      </Button>
    )
  }

  return (
    <Button
      variant={hasFilter ? (showRemove ? 'destructive' : 'secondary') : 'outline'}
      size="sm"
      onClick={handleClick}
      onMouseEnter={() => setIsHovering(true)}
      onMouseLeave={() => setIsHovering(false)}
      disabled={isMutating}
      className="gap-1.5"
    >
      {isMutating ? (
        <Loader2 className="h-4 w-4 animate-spin" />
      ) : hasFilter ? (
        <BellRing className="h-4 w-4" />
      ) : (
        <Bell className="h-4 w-4" />
      )}
      <span>
        {showRemove
          ? 'Remove notification'
          : hasFilter
            ? 'Notifications on'
            : 'Notify me'}
      </span>
    </Button>
  )
}
