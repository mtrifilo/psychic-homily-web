'use client'

import { useIsMutating } from '@tanstack/react-query'
import { AlertChipRadioGroup } from './AlertChipRadioGroup'
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
  ALERTS_AREA_HREF,
  followAlertChoice,
  followAlertOptions,
  followAlertUpdateFor,
  isAlertCapableFollowType,
  FOLLOW_ALERTS_PENDING_NOTE,
  type FollowAlertChoice,
} from './followAlertChoices'
import { cn } from '@/lib/utils'

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
  const isVenue = entityType === 'venues'

  // `is_following` flips optimistically, so it reads true before the POST that
  // creates the follow has landed. The alerts endpoint is a SUB-resource of
  // that follow and 404s until it exists, so asking on the optimistic flag
  // spends a guaranteed-failing request and a logged error on the happy path.
  // Scoped to THIS entity's write: a page that grows a second follow control
  // must not have one control's click suspend the other's query.
  const isFollowSettling =
    useIsMutating({
      mutationKey: followMutationKey,
      predicate: mutation => {
        const variables = mutation.state.variables as
          | { entityType?: string; entityId?: number | string }
          | undefined
        return (
          variables?.entityType === entityType &&
          variables?.entityId === entityId
        )
      },
    }) > 0

  const wants = supported && isFollowing && !isFollowSettling

  // Only an artist follow has a geographic scope, so only an artist page needs
  // to know whether a home area exists. Fetching it on a venue page would buy
  // a value every code path below provably discards.
  const { data: preferences } = useAlertPreferences(wants && !isVenue)
  const { data: alerts } = useFollowAlerts(entityType, entityId, wants)
  const updateAlerts = useUpdateFollowAlerts()

  const hasHomeMetro = Boolean(preferences?.home_metro)
  const current = followAlertChoice(alerts, { entityType, hasHomeMetro })

  // Nothing to reveal: not following yet, a follow type with no alert
  // subscription, or the resolved subscription has not landed.
  if (!isAuthenticated || !supported || !isFollowing || !current) return null

  const options = followAlertOptions({ entityType, hasHomeMetro })

  const choose = (choice: FollowAlertChoice) => {
    if (updateAlerts.isPending) return
    updateAlerts.mutate({
      entityType,
      entityId,
      update: followAlertUpdateFor(choice),
    })
  }

  return (
    <div className={cn('flex flex-wrap items-center gap-1 text-xs', className)}>
      <AlertChipRadioGroup
        ariaLabel={`Alerts for ${entityName}`}
        label="Alerts:"
        options={options}
        value={current}
        onChange={choose}
        pending={updateAlerts.isPending}
      />

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
