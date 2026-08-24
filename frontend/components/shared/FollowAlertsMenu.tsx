'use client'

import { BracketLink } from './BracketLink'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { useUpdateFollowAlerts } from '@/lib/hooks/common/useFollowAlerts'
import {
  followAlertChoice,
  followAlertOptions,
  followAlertSummaryFor,
  followAlertUpdateFor,
  type HomeMetroState,
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
  /**
   * Whether the viewer has a home area, deciding if "Near me" is offered.
   * `undefined` while that is still unknown, which is not the same as false.
   */
  hasHomeMetro: HomeMetroState
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
  const options = followAlertOptions({ entityType, hasHomeMetro })

  // The SERVER decides which follow types carry a subscription, and it says so
  // per row by populating `alerts` or leaving it out. Reading that beats
  // re-asserting a hardcoded list the backend would have to be kept in step
  // with by hand. A row without one renders no bracket at all, rather than a
  // disabled one implying it could be switched on. The same guard covers the
  // still-unknown home area, so a near-me follow is never briefly labelled
  // "everywhere".
  if (!current || !options) return null

  const summary = followAlertSummaryFor(options, current)

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <BracketLink
            label={
              updateAlerts.isPending ? 'alerts: saving…' : `alerts: ${summary}`
            }
            // Names the axis the bracket abbreviates. The visible text is the
            // mock's, but "alerts" alone would promise this also silences the
            // follow's RELEASE alerts, which are an account-level setting and
            // are untouched by every option here.
            ariaLabel={`Show alerts for ${entityName}: ${summary}`}
            title={`New-show alerts for ${entityName}. Release alerts are set in Settings.`}
            // No `active`: this is a menu button, not a toggle. aria-pressed
            // would both contradict an "off" label and collide with the
            // aria-haspopup/aria-expanded the trigger already carries.
            disabled={updateAlerts.isPending}
            className={cn('font-mono text-[11px]', className)}
          />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuLabel className="font-normal text-muted-foreground">
            New shows
          </DropdownMenuLabel>
          {options.map(option => (
            <DropdownMenuItem
              key={option.value}
              disabled={option.value === current}
              onSelect={() =>
                updateAlerts.mutate({
                  entityType,
                  entityId,
                  update: followAlertUpdateFor(option.value, { hasHomeMetro }),
                })
              }
            >
              {option.label}
            </DropdownMenuItem>
          ))}
        </DropdownMenuContent>
      </DropdownMenu>

      {/* The row renders from its served payload, not from the optimistic
          cache, so a failed write rolls back invisibly here: without this the
          outcome of a failure is byte-identical to nothing happening. The
          entity-page twin reports failures, and two controls over one field
          must not disagree about that. */}
      {updateAlerts.isError && (
        <span className="text-destructive" role="alert">
          Couldn&apos;t save that. Try again.
        </span>
      )}
    </>
  )
}
