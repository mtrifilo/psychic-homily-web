'use client'

import { useIsMutating } from '@tanstack/react-query'
import { BracketLink } from './BracketLink'
import { InfoTooltip } from './InfoTooltip'
import { useAuthContext } from '@/lib/context/AuthContext'
import {
  followMutationKey,
  useFollowStatus,
} from '@/lib/hooks/common/useFollow'
import {
  useFollowAlerts,
  useUpdateFollowAlerts,
} from '@/lib/hooks/common/useFollowAlerts'
import { useAlertPreferences } from '@/features/auth/hooks/useAlertPreferences'
import {
  followAlertChoice,
  followAlertOptions,
  followAlertUpdateFor,
  isAlertCapableFollowType,
  FOLLOW_ALERTS_PENDING_NOTE,
  type FollowAlertChoice,
} from './followAlertChoices'
import { cn } from '@/lib/utils'

/** Where a viewer with no home area goes to set one. */
export const ALERTS_AREA_HREF = '/profile?tab=settings#alerts-area'

const ARTIST_TOOLTIP = `Chooses which of this artist's new shows count as an alert for you. New releases are never geography-scoped, so this does not affect them. ${FOLLOW_ALERTS_PENDING_NOTE}`

const VENUE_TOOLTIP = `Turns alerts on or off for shows this venue adds. A venue sits in one place, so there is nothing to scope. ${FOLLOW_ALERTS_PENDING_NOTE}`

interface FollowAlertsRevealProps {
  /** PLURAL follow path segment: "artists" or "venues". */
  entityType: string
  entityId: number
  /** Used for the control's accessible name, so several on a page stay distinct. */
  entityName: string
  className?: string
}

/**
 * The post-follow scope reveal of the merged Follow control (PSY-1905).
 *
 * Following an artist or a venue IS subscribing to its alerts (PSY-1893), so
 * there is no second button to discover. What the follow could not express is
 * WHICH shows count, and that question only exists once the follow does, so
 * this renders nothing until then: the first click stays one click.
 *
 * Sits beside `<FollowButton>` rather than inside it. The bracket is the
 * follow toggle and keeps its own accessible name; wrapping the two would put
 * a scope control inside the thing that turns the subscription on and off.
 */
export function FollowAlertsReveal({
  entityType,
  entityId,
  entityName,
  className,
}: FollowAlertsRevealProps) {
  const { isAuthenticated } = useAuthContext()
  const { data: followStatus } = useFollowStatus(entityType, entityId)
  const isFollowing = followStatus?.is_following ?? false
  const supported = isAlertCapableFollowType(entityType)

  // `is_following` flips optimistically, so it reads true before the POST that
  // creates the follow has landed. The alerts endpoint is a SUB-resource of
  // that follow and 404s until it exists, so asking on the optimistic flag
  // spends a guaranteed-failing request and a logged error on the happy path.
  // Waiting for the write to settle costs one render; the follow mutation
  // invalidates this query on settle, so nothing has to poll for it.
  const isFollowSettling =
    useIsMutating({ mutationKey: followMutationKey }) > 0

  const { data: preferences } = useAlertPreferences()
  const { data: alerts } = useFollowAlerts(
    entityType,
    entityId,
    supported && isFollowing && !isFollowSettling
  )
  const updateAlerts = useUpdateFollowAlerts()

  const hasHomeMetro = Boolean(preferences?.home_metro)
  const current = followAlertChoice(alerts, { entityType, hasHomeMetro })

  // Nothing to reveal: not following yet, a follow type with no alert
  // subscription, or the resolved subscription has not landed.
  if (!isAuthenticated || !supported || !isFollowing || !current) return null

  const options = followAlertOptions({ entityType, hasHomeMetro })
  const isVenue = entityType === 'venues'

  const choose = (choice: FollowAlertChoice) => {
    if (choice === current || updateAlerts.isPending) return
    updateAlerts.mutate({
      entityType,
      entityId,
      update: followAlertUpdateFor(choice),
    })
  }

  return (
    <div
      className={cn('flex flex-wrap items-center gap-1 text-xs', className)}
    >
      <div
        role="radiogroup"
        aria-label={`Alerts for ${entityName}`}
        className="flex flex-wrap items-center gap-1"
      >
        <span className="text-muted-foreground">Alerts:</span>
        {options.map(option => (
          <button
            key={option.value}
            type="button"
            role="radio"
            aria-checked={current === option.value}
            disabled={updateAlerts.isPending}
            onClick={() => choose(option.value)}
            className={cn(
              'rounded-full border px-2 py-0.5 transition-colors',
              current === option.value
                ? 'border-primary text-foreground'
                : 'border-border text-muted-foreground hover:border-primary/60 hover:text-foreground',
              updateAlerts.isPending && 'opacity-60'
            )}
          >
            {option.label}
          </button>
        ))}
      </div>

      {/* Near me is not offered until an area exists, so the way to get one
          has to sit right where it is missing. */}
      {!isVenue && !hasHomeMetro && (
        <BracketLink
          label="set your area"
          href={ALERTS_AREA_HREF}
          className="font-mono text-[11px]"
        />
      )}

      <InfoTooltip
        label="What these alerts cover"
        copy={isVenue ? VENUE_TOOLTIP : ARTIST_TOOLTIP}
      />


      {updateAlerts.isError && (
        <span className="text-destructive" role="alert">
          Couldn&apos;t save that. Try again.
        </span>
      )}
    </div>
  )
}
