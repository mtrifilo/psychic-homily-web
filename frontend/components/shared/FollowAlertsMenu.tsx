'use client'

import { BracketLink } from './BracketLink'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { useUpdateFollowAlerts } from '@/lib/hooks/common/useFollowAlerts'
import {
  followAlertChoice,
  followAlertOptions,
  followAlertSummaryFor,
  followAlertUpdateFor,
} from './followAlertChoices'
import type { FollowAlertSettings } from '@/lib/types/follow'
import { cn } from '@/lib/utils'

interface FollowAlertsMenuProps {
  /** PLURAL follow path segment: "artists" or "venues". */
  entityType: string
  entityId: number
  entityName: string
  /**
   * The subscription the row was served with. Library rows carry their own
   * resolved copy (PSY-1893) precisely so this control costs no request per
   * row, so it is a prop rather than a fetch.
   */
  alerts: FollowAlertSettings | undefined
  /** Whether the viewer has a home area, which decides if "Near me" is offered. */
  hasHomeMetro: boolean
  className?: string
}

/**
 * A Library row's `[ alerts: … ]` bracket (PSY-1905).
 *
 * A menu rather than a toggle, because the artist axis has three positions and
 * cycling through them by clicking would make "off" reachable only by passing
 * through a state that briefly subscribes. It writes the same field the entity
 * page's reveal writes; both read `followAlertChoices` so the option sets
 * cannot drift apart.
 */
export function FollowAlertsMenu({
  entityType,
  entityId,
  entityName,
  alerts,
  hasHomeMetro,
  className,
}: FollowAlertsMenuProps) {
  const updateAlerts = useUpdateFollowAlerts()

  const current = followAlertChoice(alerts, { entityType, hasHomeMetro })

  // The SERVER decides which follow types carry a subscription, and it says so
  // per row by populating `alerts` or leaving it out. Reading that beats
  // re-asserting a hardcoded list the backend would have to be kept in step
  // with by hand. A row without one renders no bracket at all, rather than a
  // disabled one implying it could be switched on.
  if (!current) return null

  const options = followAlertOptions({ entityType, hasHomeMetro })
  const summary = followAlertSummaryFor(options, current)

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <BracketLink
          label={
            updateAlerts.isPending ? 'alerts: saving…' : `alerts: ${summary}`
          }
          ariaLabel={`Alerts for ${entityName}: ${summary}`}
          active
          disabled={updateAlerts.isPending}
          className={cn('font-mono text-[11px]', className)}
        />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        {options.map(option => (
          <DropdownMenuItem
            key={option.value}
            disabled={option.value === current}
            onSelect={() =>
              updateAlerts.mutate({
                entityType,
                entityId,
                update: followAlertUpdateFor(option.value),
              })
            }
          >
            {option.label}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
