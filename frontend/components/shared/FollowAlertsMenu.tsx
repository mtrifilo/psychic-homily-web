'use client'

import Link from 'next/link'
import { BracketLink } from './BracketLink'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { useUpdateFollowAlerts } from '@/lib/hooks/common/useFollowAlerts'
import {
  ALERTS_HREF,
  ALERTS_PAUSED_CHOICE_LABEL,
  ALERTS_PAUSED_SUMMARY,
  followAlertChoice,
  followAlertHasReleaseAxis,
  followAlertOptions,
  followAlertPendingNote,
  followAlertsPaused,
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
  // has neither), but they are separate facts, so this reads the release one.
  // Was a local alias over the scope predicate; the real predicate now lives
  // beside it in the shared vocabulary, so the two cannot drift.
  const hasReleaseAxis = followAlertHasReleaseAxis(entityType)

  // The SERVER decides which follow types carry a subscription, and it says so
  // per row by populating `alerts` or leaving it out. Reading that beats
  // re-asserting a hardcoded list the backend would have to be kept in step
  // with by hand. A row without one renders no bracket at all, rather than a
  // disabled one implying it could be switched on. The same guard covers the
  // still-unknown home area, so a near-me follow is never briefly labelled
  // "everywhere".
  if (!current || !options) return null

  // An enabled subscription with no channel behind it delivers nothing, so the
  // bracket must not summarize it as "near me".
  //
  // The MENU STAYS. Swapping it for a plain link was the tempting shape and it
  // broke two things at once. Radix restores focus to the trigger on close, so
  // a trigger that unmounts mid-commit dropped focus to <body> and restarted
  // the next Tab at the top of a list that can run 50 rows deep, which is the
  // exact failure the aria-disabled note below exists to prevent. And it
  // removed the only way to switch a paused follow OFF, leaving that reachable
  // only by first un-pausing every follow on the account.
  //
  // So the trigger is the same node either way, and only what it says changes.
  const paused = followAlertsPaused(alerts)
  const summary = paused
    ? ALERTS_PAUSED_SUMMARY
    : followAlertSummaryFor(options, current)

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
            // A NAME, not a description. The paused explanation runs to a
            // paragraph and belongs in the menu, where it is one node rather
            // than one per row: an accessible name repeated verbatim down 50
            // rows makes a links list unusable, and the row's own name would
            // sit buried at character 21 of it.
            ariaLabel={`Show alerts for ${entityName}: ${summary}`}
            // The release sentence is ARTIST-ONLY. A venue follow has no
            // release axis at all: the server omits `releases` from a venue's
            // settings and 422s a PATCH that sends one. Pointing a venue
            // follower at a Settings row that will never fire for them is the
            // same cross-surface disagreement this control exists to end, and
            // `FollowAlertsReveal` already forks its tooltip this way.
            // `BracketLink` sets an explicit aria-label, so `title` is the
            // accessible DESCRIPTION, announced on every row a screen reader
            // lands on. So it carries only what the NAME does not already
            // say: nothing at all while paused on a type whose alerts
            // deliver (the name says "paused" and the menu holds the prose),
            // and the pending-delivery disclosure on a type where "paused"
            // alone would read as "un-pause and they resume".
            title={
              paused
                ? (followAlertPendingNote(entityType) ?? undefined)
                : hasReleaseAxis
                  ? `New-show alerts for ${entityName}. Release alerts are set in Settings, and are still being switched on.`
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
        <DropdownMenuContent align="end" className="max-w-xs">
          {/* The explanation lives HERE, once, reachable by keyboard and
              touch rather than by hover. `title` is a mouse affordance, and
              putting a paragraph in the trigger's accessible name instead
              would repeat it down every row of the tab. */}
          {paused && (
            <>
              <DropdownMenuLabel className="whitespace-normal font-normal text-muted-foreground">
                {followAlertsPausedNote(entityType)}
              </DropdownMenuLabel>
              <DropdownMenuItem asChild>
                <Link href={ALERTS_HREF}>Turn a channel on</Link>
              </DropdownMenuItem>
              <DropdownMenuSeparator />
            </>
          )}
          <DropdownMenuLabel className="font-normal text-muted-foreground">
            {paused ? ALERTS_PAUSED_CHOICE_LABEL : 'New shows'}
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
