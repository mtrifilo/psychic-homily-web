'use client'

import { useState } from 'react'
import { useRouter, usePathname } from 'next/navigation'
import { AlertCircle, Bell, BellRing, Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { BracketLink } from '@/components/shared/BracketLink'
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
  /**
   * Rendering style. `button` (default) is the shadcn Button. `bracket`
   * renders a `<BracketLink>` for dense entity-page header linkboxes
   * (PSY-641) — `[Notify me]` toggling to `[Notifications on]`.
   */
  variant?: 'button' | 'bracket'
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
  variant = 'button',
}: NotifyMeButtonProps) {
  const router = useRouter()
  const pathname = usePathname()
  const { isAuthenticated, authStatus } = useAuthContext()
  const [isHovering, setIsHovering] = useState(false)

  // No fetch-side guard is needed here: `useNotificationFilters` enables on
  // `isAuthenticated`, which is true only for a SETTLED authenticated viewer,
  // so neither an anonymous viewer nor the unsettled window issues a request.
  const { data: matchingFilter, hasFilter, isLoading: checkLoading } =
    useNotificationFilterCheck(entityType, entityId)

  const quickCreate = useQuickCreateFilter()
  const deleteFilter = useDeleteFilter()

  const isMutating = quickCreate.isPending || deleteFilter.isPending
  const isUnsettled = authStatus === 'pending'

  const handleClick = (e: React.MouseEvent) => {
    e.preventDefault()
    e.stopPropagation()

    // Unreachable while the control renders disabled; defence in depth for the
    // redirect below, which cannot tell "no session" from "profile in flight".
    if (isUnsettled) return

    if (!isAuthenticated) {
      router.push(`/auth?returnTo=${encodeURIComponent(pathname)}`)
      return
    }

    if (isMutating) return

    if (hasFilter && matchingFilter) {
      deleteFilter.mutate(matchingFilter.id)
    } else {
      quickCreate.mutate({ entityType, entityId })
    }
  }

  // Bracket variant — dense header linkbox. handleClick already handles the
  // unauthenticated → /auth redirect, so a single BracketLink covers all states.
  if (variant === 'bracket') {
    // `disabled` also sets pointer-events-none, which is what keeps the
    // pre-hydration click BracketLink replays from landing on a viewer whose
    // identity is not settled.
    if (isUnsettled || (isAuthenticated && checkLoading)) {
      return <BracketLink label="Notify me" disabled />
    }
    return (
      <BracketLink
        label={hasFilter ? 'Notifications on' : 'Notify me'}
        active={hasFilter}
        onClick={handleClick}
        disabled={isMutating}
      />
    )
  }

  // The sign-in affordance, shared by the settled-anonymous viewer it is FOR and
  // by the unsettled one, who gets the same shape inert: this branch is a bare
  // router push to /auth, and `!isAuthenticated` reads true for a signed-in
  // viewer whose profile has not arrived. One branch rather than two so the two
  // states cannot drift into different buttons.
  if (isUnsettled || !isAuthenticated) {
    const goToAuth = isUnsettled
      ? undefined
      : () => router.push(`/auth?returnTo=${encodeURIComponent(pathname)}`)
    if (compact) {
      return (
        <Button
          variant="ghost"
          size="sm"
          onClick={goToAuth}
          disabled={isUnsettled}
          className="h-7 px-2 gap-1 text-xs"
          title={isUnsettled ? undefined : 'Sign in to get notifications'}
        >
          <Bell className="h-3.5 w-3.5" />
        </Button>
      )
    }
    return (
      <Button
        variant="outline"
        size="sm"
        onClick={goToAuth}
        disabled={isUnsettled}
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
  const mutationError = quickCreate.isError || deleteFilter.isError
  const errorMessage =
    quickCreate.error?.message ||
    deleteFilter.error?.message ||
    'Failed to update notification. Please try again.'

  if (compact) {
    return (
      <div className="inline-flex flex-col items-start gap-1">
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
        {mutationError && (
          <span className="text-xs text-destructive flex items-center gap-1" role="alert">
            <AlertCircle className="h-3 w-3 shrink-0" />
            {errorMessage}
          </span>
        )}
      </div>
    )
  }

  return (
    <div className="inline-flex flex-col items-start gap-1">
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
      {mutationError && (
        <span className="text-xs text-destructive flex items-center gap-1" role="alert">
          <AlertCircle className="h-3 w-3 shrink-0" />
          {errorMessage}
        </span>
      )}
    </div>
  )
}
