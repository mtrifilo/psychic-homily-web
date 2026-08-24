'use client'

import type { ReactNode } from 'react'
import Link from 'next/link'
import { Loader2 } from 'lucide-react'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import { HomeMetroSelect } from '@/components/shared/HomeMetroField'
import { FOLLOW_ALERTS_PENDING_NOTE } from '@/components/shared/followAlertChoices'
import { useProfile } from '@/features/auth/hooks/useAuth'
import { useSetShowReminders } from '@/features/shows'
import { useSetCollectionDigestPreference } from '@/features/collections'
import { useSetSceneDigestPreference } from '@/features/scenes'
import {
  useAlertPreferences,
  useSetAlertDefaults,
} from '../../hooks/useAlertPreferences'
import { cn } from '@/lib/utils'

/** Anchor target for "set your area" links from the entity-page reveal. */
export const ALERTS_AREA_ANCHOR = 'alerts-area'

const CUSTOM_ALERTS_HREF = '/settings/notification-filters'

/**
 * One channel cell. Three shapes, and the difference matters:
 *
 *  - a live checkbox, for a channel this alert type can actually use;
 *  - "per filter", for a channel the account cannot set because each custom
 *    alert carries its own `notify_in_app` / `notify_email`;
 *  - a dash, for a channel this alert type does not have at all.
 *
 * Rendering either of the last two as a disabled checkbox would say "off",
 * which is a different and wrong claim.
 */
type ChannelCell =
  | { kind: 'toggle'; checked: boolean; pending: boolean; onChange: (next: boolean) => void }
  | { kind: 'per-filter' }
  | { kind: 'not-applicable' }

interface AlertRow {
  id: string
  title: string
  description: ReactNode
  inApp: ChannelCell
  email: ChannelCell
}

function ChannelCellView({
  cell,
  label,
}: {
  cell: ChannelCell
  label: string
}) {
  if (cell.kind === 'not-applicable') {
    return (
      <span className="text-sm text-muted-foreground" title={`${label} does not apply here`}>
        <span aria-hidden>—</span>
        <span className="sr-only">{label} not available for this alert</span>
      </span>
    )
  }

  if (cell.kind === 'per-filter') {
    return (
      <span className="font-mono text-[10px] uppercase tracking-[0.5px] text-muted-foreground">
        <span aria-hidden>per filter</span>
        <span className="sr-only">{label} is set on each custom alert</span>
      </span>
    )
  }

  return (
    <span className="inline-flex items-center gap-1.5">
      <Checkbox
        checked={cell.checked}
        disabled={cell.pending}
        onCheckedChange={next => cell.onChange(next === true)}
        aria-label={label}
      />
      {cell.pending && (
        <Loader2 className="h-3 w-3 animate-spin text-muted-foreground" aria-hidden />
      )}
    </span>
  )
}

/**
 * The account alert matrix (PSY-1905), drawn to the PSY-1892 mock.
 *
 * Two axes: what the index tells you about, and where it reaches you. Which
 * shows count for a given follow is NOT here — that is per-follow scope, set
 * on the follow itself, because it is a property of that subscription rather
 * than of the account.
 *
 * The decided defaults are in-app ON for follow-driven alerts and EVERY email
 * column OFF. Switching email on is always an explicit act, per channel and
 * per alert type. Absent storage means inherit at every level, so a row the
 * user never touched keeps tracking the shipped default rather than being
 * frozen at whatever it was the day they first opened this card.
 *
 * The digest and reminder rows moved here from the Notifications card rather
 * than being duplicated: two controls over one boolean is a defect, not a
 * convenience.
 */
export function AlertSettings() {
  const { data: profileData } = useProfile()
  const { data: preferences, isLoading } = useAlertPreferences()
  const setAlertDefaults = useSetAlertDefaults()
  const setShowReminders = useSetShowReminders()
  const setCollectionDigest = useSetCollectionDigestPreference()
  const setSceneDigest = useSetSceneDigestPreference()

  const defaults = preferences?.alert_defaults
  const showRemindersEnabled =
    profileData?.user?.preferences?.show_reminders ?? false
  const collectionDigestEnabled =
    profileData?.user?.preferences?.notify_on_collection_digest ?? false
  const sceneDigestEnabled =
    profileData?.user?.preferences?.notify_on_scene_digest ?? false

  const rows: AlertRow[] = [
    {
      id: 'shows',
      title: 'An artist or venue you follow announces a show',
      description:
        'Which shows count for an artist is that follow’s own scope, near me or everywhere. A venue sits in one place, so its alerts have no scope.',
      inApp: {
        kind: 'toggle',
        checked: defaults?.shows.in_app ?? true,
        pending: setAlertDefaults.isPending,
        onChange: next =>
          setAlertDefaults.mutate({ shows: { in_app: next } }),
      },
      email: {
        kind: 'toggle',
        checked: defaults?.shows.email ?? false,
        pending: setAlertDefaults.isPending,
        onChange: next => setAlertDefaults.mutate({ shows: { email: next } }),
      },
    },
    {
      id: 'releases',
      title: 'An artist you follow puts out a release',
      description:
        'A record has no location, so this is never geography-scoped.',
      inApp: {
        kind: 'toggle',
        checked: defaults?.releases.in_app ?? true,
        pending: setAlertDefaults.isPending,
        onChange: next =>
          setAlertDefaults.mutate({ releases: { in_app: next } }),
      },
      email: {
        kind: 'toggle',
        checked: defaults?.releases.email ?? false,
        pending: setAlertDefaults.isPending,
        onChange: next =>
          setAlertDefaults.mutate({ releases: { email: next } }),
      },
    },
    {
      id: 'show-reminders',
      title: 'Day-before reminder for a show you saved',
      description: 'One email the day before, for shows in your library.',
      inApp: { kind: 'not-applicable' },
      email: {
        kind: 'toggle',
        checked: showRemindersEnabled,
        pending: setShowReminders.isPending,
        onChange: next => setShowReminders.mutate(next),
      },
    },
    {
      id: 'scene-digest',
      title: 'Weekly digest for scenes you follow',
      description:
        'One email a week with this week’s shows and new bands for the scenes you follow. Stays opt-in.',
      inApp: { kind: 'not-applicable' },
      email: {
        kind: 'toggle',
        checked: sceneDigestEnabled,
        pending: setSceneDigest.isPending,
        onChange: next => setSceneDigest.mutate(next),
      },
    },
    {
      id: 'collection-digest',
      title: 'Weekly digest for collections you follow',
      description:
        'One email a week summarizing items added to collections you subscribe to. Stays opt-in.',
      inApp: { kind: 'not-applicable' },
      email: {
        kind: 'toggle',
        checked: collectionDigestEnabled,
        pending: setCollectionDigest.isPending,
        onChange: next => setCollectionDigest.mutate(next),
      },
    },
    {
      id: 'custom-alerts',
      title: 'Custom alerts you built',
      description: (
        <>
          Filters by tag, price cap or several cities at once. Channels stay
          per filter, so they are set{' '}
          <Link href={CUSTOM_ALERTS_HREF} className="underline hover:text-foreground">
            on each alert
          </Link>
          .
        </>
      ),
      inApp: { kind: 'per-filter' },
      email: { kind: 'per-filter' },
    },
  ]

  const mutationFailed =
    setAlertDefaults.isError ||
    setShowReminders.isError ||
    setSceneDigest.isError ||
    setCollectionDigest.isError

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Alerts</CardTitle>
          <CardDescription>
            What the index tells you about, and where it reaches you. Which
            shows count for an artist you follow is set on that follow, not
            here.
          </CardDescription>
        </CardHeader>
        <CardContent>
          {isLoading && !preferences ? (
            <div className="flex justify-center py-6">
              <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
            </div>
          ) : (
            <div role="table" aria-label="Alert channels">
              <div
                role="row"
                className="flex items-center border-b border-border pb-2"
              >
                <span role="columnheader" className="flex-1 sr-only">
                  Alert
                </span>
                <span
                  role="columnheader"
                  className="w-[90px] text-center font-mono text-[11px] uppercase tracking-[0.6px] text-muted-foreground"
                >
                  In-app
                </span>
                <span
                  role="columnheader"
                  className="w-[90px] text-center font-mono text-[11px] uppercase tracking-[0.6px] text-muted-foreground"
                >
                  Email
                </span>
              </div>

              {rows.map((row, index) => (
                <div
                  role="row"
                  key={row.id}
                  className={cn(
                    'flex items-center py-3',
                    index < rows.length - 1 && 'border-b border-border'
                  )}
                >
                  <div role="cell" className="min-w-0 flex-1 pr-6">
                    <p className="text-sm font-medium">{row.title}</p>
                    <p className="mt-0.5 text-xs text-muted-foreground">
                      {row.description}
                    </p>
                  </div>
                  <div
                    role="cell"
                    className="flex w-[90px] shrink-0 justify-center"
                  >
                    <ChannelCellView
                      cell={row.inApp}
                      label={`In-app: ${row.title}`}
                    />
                  </div>
                  <div
                    role="cell"
                    className="flex w-[90px] shrink-0 justify-center"
                  >
                    <ChannelCellView
                      cell={row.email}
                      label={`Email: ${row.title}`}
                    />
                  </div>
                </div>
              ))}
            </div>
          )}

          {mutationFailed && (
            <p className="mt-3 text-sm text-destructive" role="alert">
              Failed to update setting. Please try again.
            </p>
          )}

          <div className="mt-4 space-y-1 border-t border-border pt-3.5">
            <p className="text-xs text-muted-foreground">
              Email stays off until you switch it on, row by row. Reminder
              emails carry a one-click unsubscribe link that flips the same box
              you see here.
            </p>
            <p className="text-xs text-muted-foreground">
              {FOLLOW_ALERTS_PENDING_NOTE}
            </p>
          </div>
        </CardContent>
      </Card>

      <Card id={ALERTS_AREA_ANCHOR} className="scroll-mt-24">
        <CardHeader>
          <CardTitle className="text-base">Your area</CardTitle>
          <CardDescription>
            The metro that &ldquo;near me&rdquo; means. Matching runs on the
            venue&apos;s metro code, not on a typed city name, so Tempe counts
            as Phoenix.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-2">
          <HomeMetroSelect metro={preferences?.home_metro ?? null} />
          <p className="text-xs text-muted-foreground">
            With no area set, a follow&apos;s near-me scope has nothing to
            match, so those alerts fall back to everywhere rather than
            silently covering nothing.
          </p>
        </CardContent>
      </Card>
    </div>
  )
}
