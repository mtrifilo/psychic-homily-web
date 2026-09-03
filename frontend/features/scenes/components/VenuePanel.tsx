'use client'

import { useEffect, useMemo, useRef } from 'react'
import Link from 'next/link'
import { usePathname, useRouter } from 'next/navigation'
import { DismissableLayer } from '@radix-ui/react-dismissable-layer'
import { X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { useAuthContext } from '@/lib/context/AuthContext'
// Deep import, not the `@/components/shared` barrel: the barrel drags in every
// shared component (and their AuthContext/router dependencies) for one button,
// and it is the path the Atlas suites already mock.
import { FollowButton } from '@/components/shared/FollowButton'
// Deep imports for the same reason as FollowButton above: the teaser needs one
// byline renderer and one hook, not every shared component or the whole
// comments feature barrel (which drags in the field-note editor and its
// markdown pipeline for a three-line quote).
import { UserAttribution } from '@/components/shared/UserAttribution'
import { useVenueFieldNotes } from '@/features/comments/hooks'
import { pickFieldNoteForTeaser } from '@/features/comments/teaser'
import { dedupVenueShows } from '@/features/shows'
import {
  formatVenueConfirmError,
  useVenueConfirm,
  useVenueShows,
} from '@/features/venues/hooks'
import { VENUE_SHOWS_PAGE_LIMIT } from '@/features/venues/api'
import type { VenueShow, VenueWithShowCount } from '@/features/venues/types'
import { formatShowTime } from '@/lib/utils/formatters'
import { showPriceText } from '@/lib/utils/showPrice'
import { showDisplayTitle } from '@/lib/utils/showDisplayTitle'
import { formatTimeAgo } from '@/lib/formatTimeAgo'
import {
  CITY_VENUE_PANEL_BOTTOM_INSET_PX,
  CITY_VENUE_PANEL_WIDTH_PX,
  VENUE_PANEL_SHOW_ROWS,
  formatPanelShowDate,
  venuePanelIdentityLine,
  venuePanelShowCount,
  venueProvenanceSegments,
  venueFieldNoteAttribution,
  mergeVenueConfirmation,
} from '../cityView'

// This panel is now the only caller of `VENUE_SHOWS_PAGE_LIMIT`. It used to
// share a cache entry with the venue page, which asked the same question; since
// PSY-1753 the venue page's sections ask different ones (an unpaginated
// upcoming list, and a paged past archive), so the entries are separate.
// `venueQueryKeys.showsPage()` keys on every parameter (PSY-1698), so that
// separation is a split, never a corruption. The constants live beside the key.

interface VenuePanelProps {
  /** The selected venue, straight from the rail's already-fetched page. */
  venue: VenueWithShowCount
  onClose: () => void
  /**
   * The artist drill-in seam (PSY-1541). A show row is a button, not a link,
   * because the drill-in opens IN the Atlas rather than navigating away; rows
   * render inert when this is undefined rather than pretending to be
   * actionable.
   *
   * `listedShows` is the second argument because the drill-in's `‹ ›` stepper
   * walks THE LIST YOU DRILLED IN FROM (a locked user decision, 2026-07-25),
   * not the one show you clicked. It is exactly the rows this panel is
   * drawing — deduped and truncated to VENUE_PANEL_SHOW_ROWS — so "2 of 5"
   * means the same thing as reading these rows top to bottom, and the panel
   * that owns the list is the one that hands it over rather than the caller
   * re-deriving it from a differently-parameterized fetch.
   */
  onShowSelect?: (show: VenueShow, listedShows: VenueShow[]) => void
}

/**
 * The Atlas city view's venue panel (PSY-1540) — "what's coming up here",
 * answered without leaving the map.
 *
 * Built to the approved mock: Product Designs Figma file, Atlas page, board 02
 * "Venue panel", node 1152:84. Every "the mock" below refers to it.
 *
 * Non-modal, on the graph inspectors' exact dismissal contract: Escape
 * dismisses via Radix `DismissableLayer` so the panel joins the one global
 * layer stack (a ⌘K palette stacked over it still wins Escape), outside-click
 * and focus-out are explicitly prevented so the map stays fully interactive
 * behind it, and there is no focus trap.
 *
 * It does NOT reuse `GraphPanelShell` despite sharing that contract, and the
 * reason is structural rather than stylistic: that shell is one scrolling
 * card with the close button inline at the top of the content flow, which is
 * right for a short graph inspector. This panel needs a PINNED header (name,
 * actions, provenance), an independently scrolling show list, and a pinned
 * footer — three regions, only the middle of which scrolls. Fitting that into
 * the shell would mean overriding its overflow and re-nesting its header, at
 * which point nothing of it is left but the DismissableLayer props, which are
 * mirrored here verbatim. If a third such panel appears, promote the
 * three-region variant into the shell rather than copying this again.
 *
 * It floats over the map's RIGHT edge and stops short of the bottom
 * (CITY_VENUE_PANEL_BOTTOM_INSET_PX) so it can never cover the map's
 * bottom-left OpenStreetMap attribution, which the ODbL requires stay visible.
 */
export function VenuePanel({ venue, onClose, onShowSelect }: VenuePanelProps) {
  const closeRef = useRef<HTMLButtonElement>(null)
  const sectionRef = useRef<HTMLElement>(null)
  const router = useRouter()
  const pathname = usePathname()
  const { isAuthenticated } = useAuthContext()

  // Focus the close control on open and hand focus back on close — the same
  // move (and the same containment guard) as ScenePreviewPanel, the Atlas's
  // other non-modal panel. It matters more here: the panel opens from a rail
  // row, and a keyboard user left standing on that row would have to tab
  // through every REMAINING venue in the city before reaching the panel their
  // keystroke just opened.
  //
  // Mount-only. AtlasGlobe keys the panel on the venue id, so switching
  // venues remounts and re-runs this once per panel, never mid-life.
  useEffect(() => {
    const section = sectionRef.current
    const opener = document.activeElement
    closeRef.current?.focus()
    return () => {
      // Restore only when focus is still OURS to hand back. The panel is
      // non-modal and Escape is document-level, so the user may have tabbed
      // out to the map or the rail in the meantime — yanking them back would
      // be worse than not restoring at all (Radix FocusScope's own rule).
      const active = document.activeElement
      const focusIsOurs =
        active === document.body ||
        (active instanceof HTMLElement &&
          section !== null &&
          section.contains(active))
      if (
        focusIsOurs &&
        opener instanceof HTMLElement &&
        opener.isConnected &&
        opener !== document.body
      ) {
        opener.focus()
      }
    }
  }, [])

  const { data, isLoading, isError } = useVenueShows({
    venueId: venue.id,
    timeFilter: 'upcoming',
    limit: VENUE_SHOWS_PAGE_LIMIT,
  })

  const fetched = data?.shows
  const shows = useMemo(
    () => (fetched ? dedupVenueShows(fetched) : []),
    [fetched],
  )
  const showCount = venuePanelShowCount({
    total: data?.total,
    listed: shows.length,
    fetched: fetched?.length ?? 0,
  })
  const visible = shows.slice(0, VENUE_PANEL_SHOW_ROWS)
  const identity = venuePanelIdentityLine(venue)
  const venueHref = `/venues/${venue.slug || venue.id}`

  // ── Confirm-current (PSY-1542) ─────────────────────────────────────────
  // The cheapest contribution the app offers: one tap, no edit, open to any
  // signed-in account at any trust tier. Gating it behind a trust tier would
  // defeat the point — it is the on-ramp, not the reward.
  const confirm = useVenueConfirm()
  const confirmError = formatVenueConfirmError(confirm.error)

  // Reads carry no viewer state — GET /venues is public and cacheable, so a
  // per-viewer "you confirmed this" field there would poison a shared cache.
  // The mutation's own result is therefore what marks the button done for this
  // session; a reload legitimately offers the button again, and tapping it is
  // an idempotent no-op server-side.
  const hasConfirmed = confirm.data?.viewer_has_confirmed === true

  // The stamp folds in the count the mutation just returned. Invalidation
  // refetches the rail's list, but that round-trip is not instant and a tap
  // that visibly changes nothing reads as a tap that did nothing — while once
  // the refetch lands, the LIST may be the fresher of the two. mergeVenueConfirmation
  // owns that "whichever is newer wins" rule.
  const provenance = useMemo(
    () => mergeVenueConfirmation(venue.provenance, confirm.data, venue.updated_at),
    [confirm.data, venue.provenance, venue.updated_at],
  )

  const provenanceSegments = venueProvenanceSegments(provenance)

  // The control stays in the tab order while inert (see the button below), so
  // this guard is what actually stops a second write — not the browser.
  const confirmInert = confirm.isPending || hasConfirmed

  // ONE sign-in destination for both routes into it — the pre-tap redirect below
  // and the expired-session link under the error. They were built separately and
  // had already drifted (one preserved the query string, the other dropped it).
  // /atlas carries no URL state today so the drift was invisible, which is
  // exactly what makes it a trap for whoever adds some.
  const signInHref = () => {
    const search = typeof window === 'undefined' ? '' : window.location.search
    return `/auth?returnTo=${encodeURIComponent(`${pathname}${search}`)}`
  }

  const handleConfirm = () => {
    if (confirmInert) return
    if (!isAuthenticated) {
      router.push(signInHref())
      return
    }
    confirm.mutate(venue.id)
  }

  return (
    <DismissableLayer
      asChild
      onDismiss={onClose}
      // Pointer/focus dismissal is deliberately off: the whole point of the
      // panel is that you can keep panning and clicking the map beside it, and
      // an outside-click close would slam it shut on the first map drag.
      onPointerDownOutside={(e) => e.preventDefault()}
      onFocusOutside={(e) => e.preventDefault()}
    >
      <section
        ref={sectionRef}
        aria-label={`${venue.name} — upcoming shows`}
        data-testid="atlas-venue-panel"
        style={{
          width: CITY_VENUE_PANEL_WIDTH_PX,
          // Bounded, not stretched. `bottom` would pin the panel to a fixed
          // height and leave a tall empty box under a two-show venue; a max
          // gives the same floor-clearance guarantee (top inset + this max can
          // never reach within CITY_VENUE_PANEL_BOTTOM_INSET_PX of the map's
          // bottom edge, where the attribution control lives) while letting
          // the panel shrink to what it actually has to say.
          maxHeight: `calc(100% - 0.75rem - ${CITY_VENUE_PANEL_BOTTOM_INSET_PX}px)`,
        }}
        className="absolute right-3 top-3 z-20 flex max-w-[calc(100%-1.5rem)] flex-col overflow-hidden rounded-md border border-border bg-background/95 shadow-lg backdrop-blur"
      >
        <header className="border-b border-border px-4 pb-3 pt-3">
          <div className="flex items-start justify-between gap-2">
            <p className="font-mono text-[11px] uppercase tracking-wide text-muted-foreground">
              Venue
            </p>
            <button
              ref={closeRef}
              type="button"
              onClick={onClose}
              aria-label={`Close ${venue.name} panel`}
              className="-mr-1 -mt-1 shrink-0 rounded-sm p-1 text-muted-foreground transition-colors hover:bg-muted/50 hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              <X className="h-3.5 w-3.5" aria-hidden="true" />
            </button>
          </div>

          <h2 className="mt-1 text-lg font-semibold leading-tight text-foreground">
            {venue.name}
          </h2>

          {identity && (
            <p
              data-testid="venue-panel-identity"
              className="mt-1 font-mono text-[11px] leading-4 text-muted-foreground"
            >
              {identity}
            </p>
          )}

          <div className="mt-3 flex flex-wrap items-center gap-2">
            <FollowButton entityType="venues" entityId={venue.id} />
            {/* `aria-disabled`, NOT the native `disabled` attribute — the same
                rule ArtistPanel's stepper follows (PSY-1540's review): the
                native attribute drops the control out of the tab order, so a
                keyboard or screen-reader user who tabs back to the panel after
                confirming never lands on it and never hears that their
                confirmation registered. This stays focusable and says so in its
                accessible name; the click is inert on our side (handleConfirm
                returns early) rather than the browser's. */}
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={handleConfirm}
              aria-disabled={confirmInert || undefined}
              className={confirmInert ? 'cursor-not-allowed opacity-70' : undefined}
              data-testid="venue-panel-confirm"
              aria-label={
                hasConfirmed
                  ? `You confirmed ${venue.name}’s info is current`
                  : confirm.isPending
                    ? `Confirming ${venue.name}’s info is current`
                    : `Confirm ${venue.name}’s info is current`
              }
            >
              {hasConfirmed
                ? '✓ Confirmed'
                : confirm.isPending
                  ? 'Confirming…'
                  : '✓ Confirm info'}
            </Button>
          </div>

          {/* Inline, beside the control that failed — there is no toast
              library in this codebase. `role="alert"` so the 429 ("try again
              in 47s") is announced rather than silently appearing under a
              button the user is about to tap again. */}
          {confirmError && (
            <p
              role="alert"
              data-testid="venue-panel-confirm-error"
              className="mt-2 font-mono text-[11px] leading-4 text-destructive"
            >
              {confirmError}
              {confirm.error?.status === 401 && (
                <>
                  {' '}
                  <Link
                    href={signInHref()}
                    className="underline underline-offset-4"
                  >
                    Sign in
                  </Link>{' '}
                  to confirm.
                </>
              )}
            </p>
          )}

          {/* Provenance (PSY-1542). Every segment is a real aggregate and a
              zero one is omitted rather than rendered as "0 edits" — a stamp
              that lists what it doesn't have reads as broken. The mock's
              "ingest + community" tail only appears when the backend actually
              has a source to name. */}
          <p
            data-testid="venue-panel-provenance"
            className="mt-2 font-mono text-[11px] leading-4 text-muted-foreground"
          >
            <span>UPDATED</span>{' '}
            {venue.updated_at ? formatTimeAgo(venue.updated_at) : 'unknown'}
            {provenanceSegments.map((segment) => (
              <span key={segment}> · {segment}</span>
            ))}
          </p>
        </header>

        <div className="min-h-0 flex-1 overflow-y-auto">
          <h3 className="px-4 pb-1 pt-3 font-mono text-[11px] uppercase tracking-wide text-muted-foreground">
            Upcoming
            {!isLoading && !isError && (
              <> — {showCount} {showCount === 1 ? 'show' : 'shows'}</>
            )}
          </h3>

          {isLoading ? (
            <p className="px-4 py-4 text-sm text-muted-foreground">
              Loading shows…
            </p>
          ) : isError ? (
            <p className="px-4 py-4 text-sm text-destructive">
              Couldn’t load this venue’s shows.
            </p>
          ) : visible.length === 0 ? (
            <p className="px-4 py-4 text-sm text-muted-foreground">
              Nothing on the calendar yet.
            </p>
          ) : (
            <>
              <ul>
                {visible.map((show) => (
                  <li key={show.id}>
                    <ShowRow
                      show={show}
                      venue={venue}
                      listedShows={visible}
                      onSelect={onShowSelect}
                    />
                  </li>
                ))}
              </ul>
              {showCount > visible.length && (
                <Link
                  href={venueHref}
                  className="block px-4 py-2 font-mono text-[11px] text-primary underline-offset-4 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring"
                >
                  view all {showCount} →
                </Link>
              )}
            </>
          )}

          <FieldNotesTeaser venue={venue} />
        </div>

        <footer className="border-t border-border px-4 py-2.5">
          <Link
            href={venueHref}
            className="font-mono text-xs text-primary underline-offset-4 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            Open venue page →
          </Link>
        </footer>
      </section>
    </DismissableLayer>
  )
}

/**
 * The mock's FIELD NOTES teaser (PSY-1590) — one quoted note about a night in
 * this room, attributed to the night it was written about.
 *
 * It is a ROLLUP, and that shapes everything below. Field notes are SHOW-scoped
 * by construction: `CreateFieldNote` writes entity_type='show', so a
 * venue-scoped field note cannot exist and this section can never be written
 * to directly. What it shows is the best-ranked note somebody wrote about a
 * show held here, which is only an honest thing to display while the show it
 * came from is named beside it — hence the hard requirement below that an
 * unattributable note render nothing at all rather than a bare quote. A quote
 * with the night stripped off reads as a standing verdict on the venue, and no
 * field note ever made that claim.
 *
 * Ordering is the backend's: most-upvoted first (Wilson score), no staleness
 * cutoff. The named month is what lets the reader judge the note's age.
 *
 * It quotes the first note `pickFieldNoteForTeaser` deems quotable HERE,
 * not simply the top-ranked one. The gap between those matters most for
 * setlist spoilers: FieldNoteCard hides a flagged note behind a click-to-reveal
 * gate, this panel has nowhere to put one, and sorting by score means an
 * upvoted spoiler is exactly the note that would otherwise surface.
 *
 * There is deliberately NO "N notes →" affordance, only a plain count. The
 * mock's arrow needs a destination — a venue-scoped notes list — and none
 * exists; inventing one would ship a link to nowhere (locked decision,
 * 2026-08-27).
 */
function FieldNotesTeaser({ venue }: { venue: VenueWithShowCount }) {
  const { data } = useVenueFieldNotes(venue.id)

  const total = data?.total ?? 0
  // Not `notes[0]`, and the pick hands back the prose with the note so this
  // component holds no second copy of the "non-empty body" invariant and does
  // not flatten the body twice.
  const picked = pickFieldNoteForTeaser(data?.notes)

  // Two ways to have nothing worth showing, one outcome: no section at all,
  // never an empty box under a heading.
  //
  //   - the venue has no notes (much the commonest case), or the request is
  //     still in flight or failed. A teaser is supplementary to the panel's
  //     actual job, which is the show list; a spinner or an error line here
  //     would be noise beside content that loaded fine, and the panel already
  //     reports its own failure for the shows themselves.
  //   - nothing on the page is quotable (see the pick).
  //
  // An untitled show is NOT one of them — see the attribution.
  if (!picked) return null

  const { note, text } = picked
  const attribution = venueFieldNoteAttribution(
    note.show_title,
    note.show_artists,
    note.show_date,
    venue.state,
    venue.timezone,
  )

  return (
    <section
      data-testid="venue-panel-field-notes"
      aria-label={`Field notes from shows at ${venue.name}`}
      className="border-t border-border/60 px-4 pb-4 pt-3"
    >
      {/* Parallel to the "Upcoming — 11 shows" heading above it, and the count
          is PLAIN TEXT, not a link. See the component's note. */}
      <h3 className="font-mono text-[11px] uppercase tracking-wide text-muted-foreground">
        Field notes — {total} {total === 1 ? 'note' : 'notes'}
      </h3>

      {/* Plain text, not the backend's rendered `body_html`: no reason to open
          an HTML injection surface on the Atlas — which has no route-level
          error boundary — for three clamped lines. `fieldNoteTeaserText`
          is what keeps the Markdown SOURCE from showing its own asterisks.

          Marked as a quotation by the rule and the italics rather than by
          quote GLYPHS. A pair of quote marks cannot survive `line-clamp`: the
          closing one is inside the clamped box, so any note longer than three
          lines would render an opening quote, an ellipsis, and no partner.

          No `block` utility beside `line-clamp-3`: the two are both `display`
          rules and the later one silently wins, which kills the clamp. */}
      <blockquote className="mt-1.5 line-clamp-3 border-l-2 border-border pl-2.5 text-sm italic leading-snug text-foreground">
        {text}
      </blockquote>

      <p className="mt-1 font-mono text-[11px] leading-4 text-muted-foreground">
        <UserAttribution
          name={note.author_name}
          username={note.author_username}
          // Plain text, no profile link. The locked decision ships this teaser
          // with no link affordance at all, and one byline link would make the
          // unlinked show name beside it read as the broken half.
          linkable={false}
          testId="venue-panel-field-note-author"
        />
        {' — '}
        <span data-testid="venue-panel-field-note-attribution">
          {attribution}
        </span>
      </p>
    </section>
  )
}

function ShowRow({
  show,
  venue,
  listedShows,
  onSelect,
}: {
  show: VenueShow
  venue: VenueWithShowCount
  listedShows: VenueShow[]
  onSelect?: (show: VenueShow, listedShows: VenueShow[]) => void
}) {
  // Per-show `state` first, venue's as the fallback — the same precedence
  // VenueShowsList uses, so the two surfaces can't disagree about a date.
  const state = show.state ?? venue.state
  const date = formatPanelShowDate(show.event_date, state, venue.timezone)
  // An unparseable date means we can't state a time either. `formatShowTime`
  // has no NaN guard of its own — it would render the literal string "Invalid
  // Date" into the meta line beside an empty date gutter — so the row's one
  // date check gates both halves. A guessed venue zone is the helper's own
  // null, and the meta line's `filter(Boolean)` drops either absence along
  // with its separator.
  const time = date
    ? formatShowTime(show.event_date, state, venue.timezone)
    : null
  const title = showDisplayTitle(
    show.title,
    // `?? []` rather than a bare `.map`: an absent bill must degrade to
    // "Untitled Show", not throw. `/atlas` has no route-level error boundary,
    // so a TypeError here takes down the whole app shell, and every sibling
    // that touches this field already guards it (headlinerArtistId,
    // showDisplayTitle's own null-tolerance contract).
    (show.artists ?? []).map((a) => a.name),
    { cap: 3 },
  )
  const meta = [time, showPriceText(show)]
    .filter(Boolean)
    .join(' · ')

  const body = (
    <>
      <span className="w-16 shrink-0 pt-px font-mono text-[11px] leading-4 text-primary">
        {date}
      </span>
      <span className="min-w-0 flex-1">
        <span className="block text-sm leading-snug text-foreground">
          {title}
        </span>
        {meta && (
          <span className="mt-0.5 block font-mono text-[11px] leading-4 text-muted-foreground">
            {meta}
          </span>
        )}
      </span>
    </>
  )

  if (!onSelect) {
    return (
      <div className="flex gap-3 border-b border-border/60 px-4 py-2.5">
        {body}
      </div>
    )
  }

  return (
    <button
      type="button"
      onClick={() => onSelect(show, listedShows)}
      className="flex w-full gap-3 border-b border-border/60 px-4 py-2.5 text-left transition-colors hover:bg-muted/30 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring"
    >
      {body}
      <span
        aria-hidden="true"
        className="shrink-0 self-center font-mono text-xs text-muted-foreground"
      >
        →
      </span>
    </button>
  )
}
