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
  ALERTS_HREF,
  ALERTS_PAUSED_SUMMARY,
  followAlertChoice,
  followAlertHasScopeAxis,
  followAlertOptions,
  followAlertsPaused,
  followAlertsPausedDetail,
  followAlertsPausedNote,
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
  // Release alerts and geographic scope happen to split the same way (a venue
  // has neither), but they are separate facts; this names the one it means.
  const hasReleaseAxis = followAlertHasScopeAxis(entityType)

  // The SERVER decides which follow types carry a subscription, and it says so
  // per row by populating `alerts` or leaving it out. Reading that beats
  // re-asserting a hardcoded list the backend would have to be kept in step
  // with by hand. A row without one renders no bracket at all, rather than a
  // disabled one implying it could be switched on. The same guard covers the
  // still-unknown home area, so a near-me follow is never briefly labelled
  // "everywhere".
  // The entity-page twin's rule, on a row: an enabled subscription with both
  // account channels off delivers nothing, so the bracket must not summarize
  // it as "near me". It becomes a LINK to the one place the channel can be
  // switched back on, rather than a menu whose every option writes a field
  // that changes nothing. The stored scope is untouched and resumes there.
  //
  // ABOVE the option guard, matching the twin, and the Library bar is why.
  // That bar derives its own paused line from these same row payloads, which
  // need no home area; `followAlertOptions` returns undefined until the area
  // read resolves, and permanently if it fails. Guarding first would print
  // "New-show alerts: paused" over a column of rows carrying no bracket at
  // all, in the exact window the bar's contract promises they agree.
  if (followAlertsPaused(alerts)) {
    return (
      <BracketLink
        label={`alerts: ${ALERTS_PAUSED_SUMMARY}`}
        // The explanation goes in the ACCESSIBLE NAME, not only the title.
        // `title` is a hover affordance: keyboard, touch and most
        // screen-reader users never see it, and `BracketLink`'s anchor branch
        // sets aria-label, which wins as the name anyway. Without this they
        // get the word "paused" and never the reassurance that their scope
        // survived, which is the half that stops "paused" reading as "your
        // setting was discarded". The entity-page twin has a real tooltip; a
        // row cannot carry one per row.
        ariaLabel={`New-show alerts for ${entityName}: paused. ${followAlertsPausedDetail(entityType)}`}
        title={followAlertsPausedNote(entityType)}
        href={ALERTS_HREF}
        className={cn('font-mono text-[11px]', className)}
      />
    )
  }

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
            // The release sentence is ARTIST-ONLY. A venue follow has no
            // release axis at all: the server omits `releases` from a venue's
            // settings and 422s a PATCH that sends one. Pointing a venue
            // follower at a Settings row that will never fire for them is the
            // same cross-surface disagreement this control exists to end, and
            // `FollowAlertsReveal` already forks its tooltip this way.
            title={
              hasReleaseAxis
                ? `New-show alerts for ${entityName}. Release alerts are set in Settings.`
                : `New-show alerts for ${entityName}.`
            }
            // No `active`: this is a menu button, not a toggle. aria-pressed
            // would both contradict an "off" label and collide with the
            // aria-haspopup/aria-expanded the trigger already carries.
            //
            // aria-disabled, NOT disabled, and the reason is specific to a
            // MENU trigger: Radix restores focus to it on close, and `focus()`
            // on a disabled element is a no-op, so committing a choice by
            // keyboard dropped focus to <body> and restarted the next Tab at
            // the top of a list that can run 50 rows deep. The guard in
            // onSelect is what actually blocks a second write.
            aria-disabled={updateAlerts.isPending || undefined}
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
              onSelect={() => {
                if (updateAlerts.isPending) return
                updateAlerts.mutate({
                  entityType,
                  entityId,
                  update: followAlertUpdateFor(option.value, { hasHomeMetro }),
                })
              }}
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
