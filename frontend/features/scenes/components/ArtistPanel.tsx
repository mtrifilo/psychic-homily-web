'use client'

import { useEffect, useRef } from 'react'
import Link from 'next/link'
import { DismissableLayer } from '@radix-ui/react-dismissable-layer'
import { ChevronLeft, ChevronRight, X } from 'lucide-react'
// Deep imports, not the `@/components/shared` barrel: the barrel drags in every
// shared component (and their AuthContext/router dependencies) for one embed,
// and it is the path the Atlas suites already mock. Same rule VenuePanel states.
import { MusicEmbed } from '@/components/shared/MusicEmbed'
import { parseSpotifyEmbed } from '@/lib/spotify'
import { useArtistGraphCard } from '@/features/artists/hooks/useArtistGraphCard'
import { useArtistShows } from '@/features/artists/hooks/useArtists'
import {
  ARTIST_SHOWS_PAGE_LIMIT,
  ARTIST_SHOWS_VIEWER_TIMEZONE,
} from '@/features/artists/api'
import type { ArtistShow } from '@/features/artists/types'
import { formatShowTime } from '@/lib/utils/formatters'
import {
  ARTIST_PANEL_NEXT_SHOW_ROWS,
  CITY_VENUE_PANEL_BOTTOM_INSET_PX,
  CITY_VENUE_PANEL_WIDTH_PX,
  formatPanelShowDate,
} from '../cityView'
import {
  artistConnectionsLine,
  artistIdentityLine,
  artistStepAnnouncement,
  artistStepKicker,
  type ArtistStep,
} from '../artistDrillIn'

interface ArtistPanelProps {
  /**
   * The list the user drilled in FROM, already flattened to artists. The
   * stepper walks THIS — a locked user decision (2026-07-25): drill in from a
   * venue panel and you step that venue's week; drill in from a citywide
   * surface and you step citywide. The panel therefore knows nothing about
   * venues, and must not learn.
   *
   * MUST be referentially stable across steps. The caller holds it in state
   * built once at drill-in time rather than deriving it per render: this array
   * feeds the query keys below, and an identity that churned per render would
   * re-key them every frame — the PSY-1538 class of bug, invisible locally.
   */
  steps: readonly ArtistStep[]
  /** Index into `steps`. The caller owns it so it survives the panel's life. */
  index: number
  onStep: (nextIndex: number) => void
  /**
   * What the list IS, for the kicker: "this week at this venue", "in Austin".
   * A prop, not a literal, for the same reason `steps` is — see above.
   */
  scopeLabel: string
  /** Breadcrumb target, e.g. "Hotel Vegas" — the panel we drilled in from. */
  backLabel: string
  /** Pop ONE level: back to the originating panel, not out of the Atlas. */
  onBack: () => void
  /** Close the whole stack. */
  onClose: () => void
}

/**
 * The Atlas artist drill-in (PSY-1541) — "who is this and can I hear them",
 * answered without leaving the map. The payoff the travel mode exists for.
 *
 * Built to the approved mock: Product Designs Figma file, Atlas page, board 03
 * "Artist drill-in", node 1154:6. Every "the mock" below refers to it, with one
 * deliberate deviation the mock has since been corrected to match: there is no
 * "NEXT UP" row — see `nextStep` below for why it went.
 *
 * Dismissal contract, focus behaviour and three-region layout are VenuePanel's
 * verbatim — the two are the same panel at different depths of one stack, and
 * they must not disagree about Escape or about clearing the map's bottom-left
 * attribution control (an ODbL licensing requirement). See VenuePanel's own
 * doc for why neither uses `GraphPanelShell`.
 *
 * This is the SECOND three-region panel, and VenuePanel's note asks that a
 * THIRD promote the variant into a shared shell rather than copy it again —
 * that rule is being followed, not deferred. Two instances is the point at
 * which the shape is visible but not yet proven: the panels already differ in
 * their header (a breadcrumb + stepper here, a name + follow/confirm actions
 * there) and in what "dismiss" means (this one pops a level; that one closes),
 * so an extraction today would be guessing at which of those the third caller
 * shares. Extract on the third, when the common shape is evidence instead of
 * prediction.
 *
 * Escape pops ONE level. The panel is rendered INSTEAD of the venue panel
 * rather than over it, so there is exactly one Atlas panel on Radix's layer
 * stack at a time and `onDismiss` is the breadcrumb: artist → venue → closed,
 * one keystroke per level.
 */
export function ArtistPanel({
  steps,
  index,
  onStep,
  scopeLabel,
  backLabel,
  onBack,
  onClose,
}: ArtistPanelProps) {
  const backRef = useRef<HTMLButtonElement>(null)

  // Focus the breadcrumb on open, for the reason PSY-1540's review raised: the
  // panel opens from a show ROW, and a keyboard user left standing on that row
  // would have to tab through the rest of the venue's shows to reach what
  // their keystroke just opened. The breadcrumb (not the ✕) is the target
  // because it is the first control in the panel and the way back out.
  //
  // Mount-only, and the panel deliberately does NOT remount per step — moving
  // focus on every `›` press would rip it off the button being pressed.
  //
  // There is deliberately NO restore-focus-to-the-opener cleanup here, unlike
  // VenuePanel's. It would be dead code: this panel REPLACES the venue panel,
  // so React unmounts the clicked show row in the same commit that mounts
  // this, and the row is already gone (with `document.activeElement` reset to
  // `<body>`) by the time a passive effect could capture it. The return path
  // is covered instead by the panel we hand back to — VenuePanel remounts and
  // focuses its own close button, which the AtlasGlobe suite asserts.
  useEffect(() => {
    backRef.current?.focus()
  }, [])

  const total = steps.length
  const current = steps[index]
  // Prefetch target only — nothing about the next artist is DRAWN. The panel
  // once ended with a "NEXT UP <name> — hear them →" row; it was removed
  // because it implied a click was needed to hear anything (the current
  // artist's player is always already visible above it) and it duplicated the
  // `‹ ›` stepper, which is now the single forward affordance.
  const nextStep = index + 1 < total ? steps[index + 1] : null

  const { data: card, isError } = useArtistGraphCard({
    artistId: current?.artistId ?? null,
    enabled: Boolean(current),
  })

  // One step ahead, warmed. The graph card is a small JSON document and the
  // query caches per artist for 5 minutes, so prefetching the NEXT artist makes
  // `›` land on a populated header instead of a skeleton — which is the whole
  // felt quality of a stepper. The embed IFRAME is deliberately NOT prefetched:
  // mounting an off-screen player would start a second Bandcamp/Spotify frame
  // per step, and a walk down a five-band bill would leave five hidden players
  // resident. Metadata ahead, media on demand.
  useArtistGraphCard({
    artistId: nextStep?.artistId ?? null,
    enabled: Boolean(nextStep),
  })

  // Requests one full page and slices for display, rather than a `limit: 2`
  // request for the two rows drawn.
  //
  // That USED to be load-bearing: `artistQueryKeys.shows()` keyed only on
  // artist id and time filter, so a narrow request here silently handed the
  // artist page a two-row list for the whole staleTime. PSY-1754 put every
  // response-shaping param in the key, and the artist page now asks for a
  // larger page than this one, so the two no longer share an entry either way.
  //
  // It stays a full page only because nothing has measured whether narrowing it
  // is worth the loss on a reader who opens the artist page from here. Deciding
  // that is a follow-up, not a drive-by: what matters at this call site is that
  // the page it asks for is a whole one, which is what the test below pins.
  const { data: showsData } = useArtistShows({
    artistId: current?.artistId ?? 0,
    limit: ARTIST_SHOWS_PAGE_LIMIT,
    timezone: ARTIST_SHOWS_VIEWER_TIMEZONE,
    timeFilter: 'upcoming',
    enabled: Boolean(current),
  })
  const nextShows = (showsData?.shows ?? []).slice(0, ARTIST_PANEL_NEXT_SHOW_ROWS)

  // A drill-in with nothing to step to is a bug upstream, not a state to
  // render: the caller only opens the panel for a show that contributed at
  // least one artist. Bail rather than paint an empty chrome.
  if (!current) return null

  const artistName = card?.name ?? current.artistName
  const artistSlug = card?.slug || current.artistSlug
  const identity = card ? artistIdentityLine(card) : ''
  const connections = card ? artistConnectionsLine(card) : ''
  // Whether the LISTEN block will actually render a player. Mirrors MusicEmbed's
  // own resolution exactly — ArtistContextPanel's `hasPlayableAudio`, and for
  // the same reason (PSY-1302): a Bandcamp embed URL always yields content (an
  // iframe, or a fallback link), a Spotify link only when it parses to an
  // embeddable id. A headed LISTEN section with no player under it is the
  // failure mode this gate exists to prevent.
  const hasPlayableAudio = card
    ? Boolean(card.bandcamp_embed_url) ||
      Boolean(card.spotify && parseSpotifyEmbed(card.spotify))
    : false

  const atFirst = index <= 0
  const atLast = index >= total - 1
  const kicker = artistStepKicker({ index, total, scopeLabel })
  const announcement = artistStepAnnouncement({
    index,
    total,
    artistName,
    scopeLabel,
  })

  return (
    <DismissableLayer
      asChild
      // Pops ONE level, not the whole stack — the acceptance criterion. The
      // venue panel is unmounted while this is open, so this layer is the only
      // Atlas panel Radix is coordinating and Escape reaches it directly.
      onDismiss={onBack}
      // Pointer/focus dismissal off, exactly as VenuePanel: the map stays fully
      // interactive beside the panel, and an outside-click close would slam it
      // shut on the first map drag.
      onPointerDownOutside={(e) => e.preventDefault()}
      onFocusOutside={(e) => e.preventDefault()}
    >
      <section
        aria-label={`${artistName} — artist details`}
        data-testid="atlas-artist-panel"
        style={{
          width: CITY_VENUE_PANEL_WIDTH_PX,
          // Same bounded height as VenuePanel, for the same licensing reason:
          // top inset + this max can never reach within
          // CITY_VENUE_PANEL_BOTTOM_INSET_PX of the map's bottom edge, where
          // the OpenStreetMap attribution control lives.
          maxHeight: `calc(100% - 0.75rem - ${CITY_VENUE_PANEL_BOTTOM_INSET_PX}px)`,
        }}
        className="absolute right-3 top-3 z-20 flex max-w-[calc(100%-1.5rem)] flex-col overflow-hidden rounded-md border border-border bg-background/95 shadow-lg backdrop-blur"
      >
        <header className="border-b border-border px-4 pb-3 pt-3">
          <div className="flex items-start justify-between gap-2">
            <button
              ref={backRef}
              type="button"
              onClick={onBack}
              className="-ml-1 -mt-0.5 flex min-w-0 items-center gap-1 rounded-sm px-1 py-0.5 font-mono text-[11px] text-muted-foreground transition-colors hover:bg-muted/50 hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              <ChevronLeft className="h-3 w-3 shrink-0" aria-hidden="true" />
              <span className="truncate">{backLabel}</span>
            </button>
            <button
              type="button"
              onClick={onClose}
              aria-label={`Close ${artistName} panel`}
              className="-mr-1 -mt-1 shrink-0 rounded-sm p-1 text-muted-foreground transition-colors hover:bg-muted/50 hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              <X className="h-3.5 w-3.5" aria-hidden="true" />
            </button>
          </div>

          <div className="mt-1.5 flex items-center justify-between gap-2">
            <p
              data-testid="artist-panel-kicker"
              className="min-w-0 truncate font-mono text-[11px] uppercase tracking-wide text-primary"
            >
              {kicker}
            </p>
            {total > 1 && (
              <div className="flex shrink-0 items-center gap-0.5">
                <StepButton
                  direction="previous"
                  disabled={atFirst}
                  onClick={() => onStep(index - 1)}
                />
                <StepButton
                  direction="next"
                  disabled={atLast}
                  onClick={() => onStep(index + 1)}
                />
              </div>
            )}
          </div>

          {/* Stepping changes the panel's contents WITHOUT moving focus (by
              design — focus stays on the button being pressed), and the visible
              kicker is abbreviated mono caps whose `‹ ›` glyphs only imply the
              position visually. This is the only thing that states the new
              position, and who you landed on, to a screen reader. */}
          <p className="sr-only" role="status">
            {announcement}
          </p>

          <h2 className="mt-1 text-lg font-semibold leading-tight text-foreground">
            {artistName}
          </h2>

          {identity && (
            <p
              data-testid="artist-panel-identity"
              className="mt-1 font-mono text-[11px] leading-4 text-muted-foreground"
            >
              {identity}
            </p>
          )}
        </header>

        <div className="min-h-0 flex-1 overflow-y-auto">
          {isError && !card && (
            <p className="px-4 py-4 text-sm text-muted-foreground">
              Details couldn’t load — the artist page has the full picture.
            </p>
          )}

          {/* Embed-first, as the mock draws it: the whole promise of the
              drill-in is that you can HEAR the band, so the player comes before
              any metadata row. The heading renders only when a player will. */}
          {hasPlayableAudio && card && (
            <section
              data-testid="artist-panel-listen"
              className="border-b border-border/60 px-4 pb-2 pt-3"
            >
              <h3 className="pb-1.5 font-mono text-[11px] uppercase tracking-wide text-muted-foreground">
                Listen
              </h3>
              <MusicEmbed
                compact
                bandcampAlbumUrl={card.bandcamp_embed_url}
                spotifyUrl={card.spotify}
                artistName={artistName}
              />
            </section>
          )}

          {nextShows.length > 0 && (
            <section className="border-b border-border/60 px-4 pb-2.5 pt-3">
              <h3 className="pb-1 font-mono text-[11px] uppercase tracking-wide text-muted-foreground">
                {nextShows.length === 1 ? 'Next show' : 'Next shows'}
              </h3>
              <ul className="space-y-1">
                {nextShows.map((show) => (
                  <li key={show.id}>
                    <NextShowRow show={show} artistId={current.artistId} />
                  </li>
                ))}
              </ul>
            </section>
          )}

          {connections && (
            <section className="border-b border-border/60 px-4 pb-2.5 pt-3">
              <h3 className="pb-1 font-mono text-[11px] uppercase tracking-wide text-muted-foreground">
                Connections
              </h3>
              <p className="text-sm leading-snug text-foreground/90">
                {connections}
              </p>
            </section>
          )}
        </div>

        <footer className="border-t border-border px-4 py-2.5">
          <Link
            // Always rendered, and off the STEP's slug when the card hasn't
            // landed — the panel replaced a navigation, so it must never strand
            // the user pathless while a fetch is in flight or after it failed.
            href={`/artists/${encodeURIComponent(artistSlug || String(current.artistId))}`}
            className="font-mono text-xs text-primary underline-offset-4 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            Open artist page →
          </Link>
        </footer>
      </section>
    </DismissableLayer>
  )
}

/**
 * One `‹` / `›` control.
 *
 * `aria-disabled`, NOT the native `disabled` attribute — PSY-1540's review
 * found exactly this: `disabled` drops the control out of the tab order, so a
 * keyboard or screen-reader user never lands on it, never learns the stepper
 * exists, and never hears why it does nothing. This stays focusable and
 * announces the reason as part of its accessible name; the click is inert on
 * our side rather than the browser's.
 */
function StepButton({
  direction,
  disabled,
  onClick,
}: {
  direction: 'previous' | 'next'
  disabled: boolean
  onClick: () => void
}) {
  const Icon = direction === 'previous' ? ChevronLeft : ChevronRight
  const edge = direction === 'previous' ? 'first' : 'last'
  return (
    <button
      type="button"
      aria-disabled={disabled || undefined}
      aria-label={
        disabled
          ? `${direction === 'previous' ? 'Previous' : 'Next'} artist — already at the ${edge}`
          : `${direction === 'previous' ? 'Previous' : 'Next'} artist`
      }
      data-testid={`artist-panel-step-${direction}`}
      onClick={disabled ? undefined : onClick}
      className={`rounded-sm p-1 transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring ${
        disabled
          ? 'cursor-not-allowed text-muted-foreground/40'
          : 'text-muted-foreground hover:bg-muted/50 hover:text-foreground'
      }`}
    >
      <Icon className="h-3.5 w-3.5" aria-hidden="true" />
    </button>
  )
}

/** "SAT 8/1 · Hotel Vegas · w/ Farmer's Wife · 9:00 PM", per the mock. */
function NextShowRow({ show, artistId }: { show: ArtistShow; artistId: number }) {
  const venue = show.venue
  const date = formatPanelShowDate(show.event_date, venue?.state, venue?.timezone)
  // An unparseable date means we can't state a time either — `formatShowTime`
  // has no NaN guard and would render the literal "Invalid Date". One check
  // gates both halves, exactly as VenuePanel's ShowRow does.
  const time = date ? formatShowTime(show.event_date, venue?.state, venue?.timezone) : ''
  // `?? []` and not a bare `.map`: a show served without a bill must degrade,
  // not throw — `/atlas` has no route-level error boundary.
  const others = (show.artists ?? [])
    .filter((a) => a && a.id !== artistId)
    .map((a) => a.name)
  const parts = [
    date,
    venue?.name,
    others.length > 0 ? `w/ ${others.slice(0, 2).join(', ')}` : '',
    time,
  ].filter(Boolean)
  return (
    <p className="text-sm leading-snug text-foreground/90">{parts.join(' · ')}</p>
  )
}
