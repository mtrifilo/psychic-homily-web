'use client'

import { useCallback, useRef, type ReactNode } from 'react'
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
import {
  ALERTS_ANCHOR,
  ALERTS_AREA_ANCHOR,
  CUSTOM_ALERTS_HREF,
  RELEASE_ALERTS_PENDING_NOTE,
  VENUE_ALERTS_PENDING_NOTE,
} from '@/components/shared/followAlertChoices'
import { useProfile } from '@/features/auth/hooks/useAuth'
import { useSetShowReminders } from '@/features/shows'
import { useSetCollectionDigestPreference } from '@/features/collections'
import { useSetSceneDigestPreference } from '@/features/scenes'
import {
  useAlertPreferences,
  useSetAlertDefaults,
} from '../../hooks/useAlertPreferences'
import { useUrlHash } from '@/lib/hooks/common/useUrlHash'
import { cn } from '@/lib/utils'

/**
 * A ref that scrolls its card into view when a link carrying that card's
 * fragment lands here.
 *
 * A callback REF rather than an effect keyed on the hash. Both anchors live
 * inside a Radix TabsContent that mounts only after the client navigation
 * commits, and the cards themselves can mount later still, so on a cold load
 * (bookmark, refresh, opened from an email) nothing with that id exists when
 * the browser, or an effect that only re-runs on `hashchange`, resolves the
 * fragment. Firing when the NODE arrives is the one signal that is always
 * available at the right moment. `once` keeps a later re-render from yanking
 * the page back after the user has scrolled away.
 *
 * Both cards need it, not just the area card: PSY-1896's artist show-alert
 * email links "Manage alerts in Settings" at `/settings/notifications`, which
 * this branch retargets to `#alerts`. Without this the emailed reader lands at
 * the top of the settings tab, two cards above the matrix they were sent to.
 */
function useAnchorScroll(anchorId: string) {
  const urlHash = useUrlHash()
  const scrolled = useRef(false)

  return useCallback(
    (node: HTMLDivElement | null) => {
      if (!node || scrolled.current) return
      if (urlHash.replace(/^#/, '') !== anchorId) return
      scrolled.current = true
      node.scrollIntoView({ behavior: 'smooth', block: 'start' })
    },
    [urlHash, anchorId]
  )
}

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
  // Only the two follow-alert rows can be unavailable, and only they: their
  // state comes from the account matrix. The reminder and digest rows read the
  // profile, so a failure over there must not reach them.
  | { kind: 'unavailable' }

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
      <span className="text-sm text-muted-foreground">
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

  if (cell.kind === 'unavailable') {
    return (
      <span className="font-mono text-[10px] uppercase tracking-[0.5px] text-muted-foreground">
        <span aria-hidden>unknown</span>
        <span className="sr-only">
          {label} could not be loaded, so it is not shown
        </span>
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
 * The Alerts card shell, shared by the matrix and its unavailable states.
 *
 * Carries `#alerts` because PSY-1896's artist show-alert email links its
 * "manage" CTA at this card, and that link has to land on the matrix in every
 * state, including the degraded ones.
 */
function AlertsCard({
  children,
  anchorRef,
}: {
  children: ReactNode
  anchorRef: (node: HTMLDivElement | null) => void
}) {
  return (
    <Card id={ALERTS_ANCHOR} ref={anchorRef} className="scroll-mt-24">
      <CardHeader>
        <CardTitle className="text-base">Alerts</CardTitle>
        <CardDescription>
          What the index tells you about, and where it reaches you. Which shows
          count for an artist you follow is set on that follow, not here.
        </CardDescription>
      </CardHeader>
      <CardContent>{children}</CardContent>
    </Card>
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
 */
export function AlertSettings() {
  const alertsAnchorRef = useAnchorScroll(ALERTS_ANCHOR)
  const areaAnchorRef = useAnchorScroll(ALERTS_AREA_ANCHOR)
  const { data: profileData } = useProfile()
  const {
    data: preferences,
    isError: preferencesFailed,
  } = useAlertPreferences()
  const setAlertDefaults = useSetAlertDefaults()
  const setShowReminders = useSetShowReminders()
  const setCollectionDigest = useSetCollectionDigestPreference()
  const setSceneDigest = useSetSceneDigestPreference()

  // No client-side fallback for the matrix. The shipped defaults have exactly
  // one home, on the server, and the whole point of the endpoint returning a
  // RESOLVED matrix is that the client never restates them. Painting `?? true`
  // / `?? false` over a failed read would report "we are not emailing you" to
  // someone we are, and every box would be live: one click then PINS a value
  // they never chose, from a state they were shown wrongly.
  const defaults = preferences?.alert_defaults
  const showRemindersEnabled =
    profileData?.user?.preferences?.show_reminders ?? false
  const collectionDigestEnabled =
    profileData?.user?.preferences?.notify_on_collection_digest ?? false
  const sceneDigestEnabled =
    profileData?.user?.preferences?.notify_on_scene_digest ?? false

  // One cell shape, spelled once. The two follow-alert rows share one mutation
  // (they are two axes of one PATCH endpoint) and therefore one pending flag;
  // the reminder and digest rows each have their own.
  const toggle = (
    checked: boolean,
    mutation: { isPending: boolean },
    onChange: (next: boolean) => void
  ): ChannelCell => ({ kind: 'toggle', checked, pending: mutation.isPending, onChange })

  /**
   * A follow-alert cell, which needs the resolved account matrix.
   *
   * Scoped to these two rows ON PURPOSE. Taking the whole card down when this
   * one endpoint fails would also remove the reminder and both digest rows,
   * which read the profile and are perfectly loadable — and those three are
   * the only in-product way to turn OFF a digest email someone is already
   * receiving. A new endpoint's bad day must not strand a shipped preference.
   */
  const matrixToggle = (
    read: (matrix: NonNullable<typeof defaults>) => boolean,
    write: (next: boolean) => void
  ): ChannelCell =>
    defaults
      ? toggle(read(defaults), setAlertDefaults, write)
      : { kind: 'unavailable' }

  const rows: AlertRow[] = [
    {
      id: 'shows',
      title: 'An artist or venue you follow announces a show',
      // One row because the account matrix has ONE `shows` key covering both,
      // which is also what PSY-1896's unsubscribe writes. Their DELIVERY
      // differs today, and saying so is the honest way to render one control
      // over two half-shipped things.
      // "In-app" is the claim that has been observed end to end. PSY-1896's
      // email lane is built and covered by integration tests, but no owner has
      // watched a real message arrive, so it is not named as live here.
      description:
        'Which shows count for an artist is that follow’s own scope, near me or everywhere. A venue sits in one place, so its alerts have no scope. In-app alerts for artists are live; venue alerts are still being switched on.',
      inApp: matrixToggle(
        matrix => matrix.shows.in_app,
        next => setAlertDefaults.mutate({ shows: { in_app: next } })
      ),
      email: matrixToggle(
        matrix => matrix.shows.email,
        next => setAlertDefaults.mutate({ shows: { email: next } })
      ),
    },
    {
      id: 'releases',
      title: 'An artist you follow puts out a release',
      description:
        'A record has no location, so this is never geography-scoped.',
      inApp: matrixToggle(
        matrix => matrix.releases.in_app,
        next => setAlertDefaults.mutate({ releases: { in_app: next } })
      ),
      email: matrixToggle(
        matrix => matrix.releases.email,
        next => setAlertDefaults.mutate({ releases: { email: next } })
      ),
    },
    {
      id: 'show-reminders',
      title: 'Day-before reminder for a show you saved',
      description: 'One email the day before, for shows in your library.',
      inApp: { kind: 'not-applicable' },
      email: toggle(showRemindersEnabled, setShowReminders, next =>
        setShowReminders.mutate(next)
      ),
    },
    {
      id: 'scene-digest',
      title: 'Weekly digest for scenes you follow',
      description:
        'One email a week with this week’s shows and new bands for the scenes you follow. Stays opt-in.',
      inApp: { kind: 'not-applicable' },
      email: toggle(sceneDigestEnabled, setSceneDigest, next =>
        setSceneDigest.mutate(next)
      ),
    },
    {
      id: 'collection-digest',
      title: 'Weekly digest for collections you follow',
      description:
        'One email a week summarizing items added to collections you subscribe to. Stays opt-in.',
      inApp: { kind: 'not-applicable' },
      email: toggle(collectionDigestEnabled, setCollectionDigest, next =>
        setCollectionDigest.mutate(next)
      ),
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
      <AlertsCard anchorRef={alertsAnchorRef}>
          {/* A real table, not a grid of role="…" divs: the browser cannot
              get row/cell nesting wrong, and the row title becomes a genuine
              row header rather than a cell that happens to be first. */}
            <table className="w-full table-fixed border-collapse">
              <caption className="sr-only">
                Alert types and the channels each one reaches you on
              </caption>
              <thead>
                <tr className="border-b border-border">
                  <th scope="col" className="sr-only">
                    Alert
                  </th>
                  <th
                    scope="col"
                    className="w-[90px] pb-2 text-center font-mono text-[11px] font-normal uppercase tracking-[0.6px] text-muted-foreground"
                  >
                    In-app
                  </th>
                  <th
                    scope="col"
                    className="w-[90px] pb-2 text-center font-mono text-[11px] font-normal uppercase tracking-[0.6px] text-muted-foreground"
                  >
                    Email
                  </th>
                </tr>
              </thead>
              <tbody>
                {rows.map((row, index) => (
                  <tr
                    key={row.id}
                    className={cn(
                      index < rows.length - 1 && 'border-b border-border'
                    )}
                  >
                    <th
                      scope="row"
                      className="py-3 pr-6 text-left align-middle font-normal"
                    >
                      <span className="block text-sm font-medium">
                        {row.title}
                      </span>
                      <span className="mt-0.5 block text-xs text-muted-foreground">
                        {row.description}
                      </span>
                    </th>
                    <td className="w-[90px] py-3 text-center align-middle">
                      <ChannelCellView
                        cell={row.inApp}
                        label={`In-app: ${row.title}`}
                      />
                    </td>
                    <td className="w-[90px] py-3 text-center align-middle">
                      <ChannelCellView
                        cell={row.email}
                        label={`Email: ${row.title}`}
                      />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>

          {/* Scoped to the two rows that actually need the account matrix.
              The reminder and digest rows below read the profile and stay
              usable, which matters: they are the only in-product way to turn
              OFF a digest someone is already receiving. */}
          {preferencesFailed && (
            <p className="mt-3 text-sm text-destructive" role="alert">
              Couldn&apos;t load your follow-alert settings. The rows marked
              unknown are unavailable until this page reloads.
            </p>
          )}

          {mutationFailed && (
            <p className="mt-3 text-sm text-destructive" role="alert">
              Failed to update setting. Please try again.
            </p>
          )}

          <div className="mt-4 space-y-1 border-t border-border pt-3.5">
            <p className="text-xs text-muted-foreground">
              Email stays off until you switch it on, row by row. Artist
              show-alert and reminder emails carry a one-click unsubscribe link
              that flips the same box you see here.
            </p>
            <p className="text-xs text-muted-foreground">
              {VENUE_ALERTS_PENDING_NOTE} {RELEASE_ALERTS_PENDING_NOTE}
            </p>
          </div>
      </AlertsCard>

      <Card
        id={ALERTS_AREA_ANCHOR}
        ref={areaAnchorRef}
        className="scroll-mt-24"
      >
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
