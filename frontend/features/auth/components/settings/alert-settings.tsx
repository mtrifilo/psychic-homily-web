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
 * Clears the sticky TopBar so a hash deep-link shows the card's heading rather
 * than parking it under the bar. Same token the profile tab's field anchors
 * use; a hardcoded `scroll-mt-24` guesses the bar's height instead of reading
 * it, which now matters because an EMAIL links here.
 */
const ALERTS_SCROLL_MT = 'scroll-mt-[calc(var(--topbar-height)+1rem)]'

const prefersReducedMotion = () =>
  typeof window !== 'undefined' &&
  window.matchMedia?.('(prefers-reduced-motion: reduce)').matches === true

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
      // Moving the VIEWPORT is only half of following a link. Without focus, a
      // keyboard or screen-reader user arriving from the alert email's "Manage
      // alerts in Settings" gets the page scrolled to the matrix while focus
      // stays at the document start, so their next Tab lands in the top nav
      // rather than on the control they were sent to. The card is not
      // otherwise focusable, hence the -1 tabindex.
      node.scrollIntoView({
        behavior: prefersReducedMotion() ? 'auto' : 'smooth',
        block: 'start',
      })
      node.focus({ preventScroll: true })
    },
    [urlHash, anchorId]
  )
}

/**
 * One channel cell. Six shapes, and the differences all matter, because a
 * checkbox is the only one of them that means "you can set this, and here is
 * what it is set to". Rendering any of the other five as a disabled checkbox
 * would say "off", which is a different and wrong claim:
 *
 *  - `toggle`         a live checkbox, for a channel this alert type can use;
 *  - `per-filter`     the account cannot set it, because each custom alert
 *                     carries its own `notify_email`;
 *  - `always-on`      it fires for this alert type with no way to turn it off;
 *  - `not-applicable` this alert type has no such channel at all;
 *  - `unavailable`    the read that would say FAILED;
 *  - `pending`        that read is still in flight, which is not the same
 *                     thing and must not be reported as failure.
 */
type ChannelCell =
  | { kind: 'toggle'; checked: boolean; pending: boolean; onChange: (next: boolean) => void }
  | { kind: 'per-filter' }
  // A channel that fires for this alert type with no way to turn it off. Not a
  // checked checkbox, which would invite a click that silently does nothing.
  | { kind: 'always-on' }
  | { kind: 'not-applicable' }
  // Only the two follow-alert rows can be unavailable, and only they: their
  // state comes from the account matrix. The reminder and digest rows read the
  // profile, so a failure over there must not reach them.
  | { kind: 'unavailable' }
  // PENDING is not UNAVAILABLE. Collapsing them made every cold load of the
  // settings tab announce "could not be loaded" over four settings whose
  // request was still in flight, with no banner to explain it because nothing
  // had actually failed. `FollowAlertsReveal` states this rule for the sibling
  // control; the matrix has to keep it too.
  | { kind: 'pending' }

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

  if (cell.kind === 'always-on') {
    return (
      <span className="font-mono text-[10px] uppercase tracking-[0.5px] text-muted-foreground">
        <span aria-hidden>always</span>
        <span className="sr-only">
          {label} is always on for this alert and cannot be turned off
        </span>
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

  if (cell.kind === 'pending') {
    return (
      <span className="inline-flex items-center justify-center">
        <Loader2
          className="h-3.5 w-3.5 animate-spin text-muted-foreground"
          aria-hidden
        />
        <span className="sr-only">{label} is still loading</span>
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
      {/* aria-disabled, NOT disabled, for the reason AlertChipRadioGroup spells
          out: a disabled element cannot hold focus, so parking the box during
          its own PATCH drops a keyboard user to <body> and their next Tab
          restarts from the top of the page. The guard in onCheckedChange is
          what actually blocks the second write. */}
      <Checkbox
        checked={cell.checked}
        aria-disabled={cell.pending || undefined}
        onCheckedChange={next => {
          if (cell.pending) return
          cell.onChange(next === true)
        }}
        className={cn(cell.pending && 'cursor-not-allowed opacity-60')}
        aria-label={label}
      />
      {cell.pending && (
        <Loader2 className="h-3 w-3 animate-spin text-muted-foreground" aria-hidden />
      )}
    </span>
  )
}

/**
 * The Alerts card shell.
 *
 * Carries `#alerts` because PSY-1896's artist show-alert email links its
 * "manage" CTA at this card, and that link has to land on the matrix in every
 * state, including the degraded ones. Unavailability is per CELL, so the card
 * itself renders the same shell either way.
 */
function AlertsCard({
  children,
  anchorRef,
}: {
  children: ReactNode
  anchorRef: (node: HTMLDivElement | null) => void
}) {
  return (
    <Card
      id={ALERTS_ANCHOR}
      ref={anchorRef}
      tabIndex={-1}
      className={cn(ALERTS_SCROLL_MT, 'focus:outline-none')}
    >
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
  const {
    data: profileData,
    isSuccess: profileLoaded,
    isError: profileFailed,
  } = useProfile()
  const { data: preferences, isError: preferencesErrored } =
    useAlertPreferences()
  // A failed BACKGROUND refetch keeps `data` and only flips status, so status
  // alone made the two halves of this one card disagree about one response:
  // the matrix kept rendering live checkboxes off the cached payload while the
  // area card below announced it could not be loaded. Only a failure with
  // nothing cached is a failure the user needs told about.
  const preferencesFailed = preferencesErrored && !preferences
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
  const profilePreferences = profileData?.user?.preferences

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
  ): ChannelCell => {
    if (defaults) return toggle(read(defaults), setAlertDefaults, write)
    return preferencesFailed ? { kind: 'unavailable' } : { kind: 'pending' }
  }

  /**
   * A profile-backed cell, for the reminder and digest rows.
   *
   * Same tri-state as `matrixToggle`, and for a sharper reason. These three
   * are, by this card's own reckoning, the only in-product way to turn OFF a
   * digest someone is ALREADY receiving, and this card is now where PSY-1896's
   * alert email sends a reader who wants less mail. `?? false` over an
   * unresolved profile therefore told exactly that reader "we are not emailing
   * you" about three streams we are, over a live checkbox that would pin the
   * wrong value on one click. The rule was already written for the rows above;
   * these rows arrived from another file and did not have it applied.
   */
  const profileToggle = (
    read: (prefs: NonNullable<typeof profilePreferences>) => boolean | undefined,
    mutation: { isPending: boolean },
    write: (next: boolean) => void
  ): ChannelCell => {
    if (profileLoaded) {
      return toggle(read(profilePreferences ?? {}) ?? false, mutation, write)
    }
    return profileFailed ? { kind: 'unavailable' } : { kind: 'pending' }
  }

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
      email: profileToggle(
        prefs => prefs.show_reminders,
        setShowReminders,
        next => setShowReminders.mutate(next)
      ),
    },
    {
      id: 'scene-digest',
      title: 'Weekly digest for scenes you follow',
      description:
        'One email a week with this week’s shows and new bands for the scenes you follow. Stays opt-in.',
      inApp: { kind: 'not-applicable' },
      email: profileToggle(
        prefs => prefs.notify_on_scene_digest,
        setSceneDigest,
        next => setSceneDigest.mutate(next)
      ),
    },
    {
      id: 'collection-digest',
      title: 'Weekly digest for collections you follow',
      description:
        'One email a week summarizing items added to collections you subscribe to. Stays opt-in.',
      inApp: { kind: 'not-applicable' },
      email: profileToggle(
        prefs => prefs.notify_on_collection_digest,
        setCollectionDigest,
        next => setCollectionDigest.mutate(next)
      ),
    },
    {
      id: 'custom-alerts',
      title: 'Custom alerts you built',
      description: (
        <>
          Filters by tag, price cap or several cities at once. A match always
          reaches your inbox here; whether it also emails you is set{' '}
          <Link href={CUSTOM_ALERTS_HREF} className="underline hover:text-foreground">
            on each alert
          </Link>
          .
        </>
      ),
      // In-app is NOT per filter, however the builder draws it. The matcher
      // writes the notification row unconditionally and branches only on
      // `NotifyEmail`, so a custom alert always lands in the inbox and the
      // builder's own in-app switch is labelled "coming soon". Saying "per
      // filter" here would send someone to a control that cannot turn this
      // off. Email genuinely is per filter.
      inApp: { kind: 'always-on' },
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
              Every email governed by this card stays off until you switch it
              on, row by row, and carries a one-click unsubscribe link.
              Unsubscribing flips the matching box above, except for a custom
              alert, which switches off email for that one alert instead.
            </p>
            <p className="text-xs text-muted-foreground">
              {VENUE_ALERTS_PENDING_NOTE} {RELEASE_ALERTS_PENDING_NOTE}
            </p>
          </div>
      </AlertsCard>

      <Card
        id={ALERTS_AREA_ANCHOR}
        ref={areaAnchorRef}
        tabIndex={-1}
        className={cn(ALERTS_SCROLL_MT, 'focus:outline-none')}
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
          {/* Same tri-state rule the matrix above obeys, for the same reason.
              `?? null` collapsed UNKNOWN into "no home area", so on every load
              before the read resolved (and permanently after it failed) this
              card showed "No home area" selected to someone with a metro
              stored, over an ENABLED select, and the sentence below then told
              them their near-me follows had fallen back to everywhere. One
              click from that state overwrites a real preference with a value
              they were shown wrongly. */}
          {preferencesFailed ? (
            <p className="text-sm text-destructive" role="alert">
              Couldn&apos;t load your area. Reload the page to try again.
            </p>
          ) : preferences ? (
            <HomeMetroSelect metro={preferences.home_metro ?? null} />
          ) : (
            <span className="inline-flex items-center gap-2 text-sm text-muted-foreground">
              <Loader2 className="h-4 w-4 animate-spin" aria-hidden />
              Loading your area…
            </span>
          )}
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
