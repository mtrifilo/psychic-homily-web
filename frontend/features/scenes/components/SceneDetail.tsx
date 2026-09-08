'use client'

import Link from 'next/link'
import { MapPin, Tent, ArrowRight, Loader2 } from 'lucide-react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { useSceneDetail } from '../hooks'
import { FollowButton } from '@/components/shared/FollowButton'
import { ShareButton } from '@/components/shared/ShareButton'
import { SceneNotifyModeToggle } from './SceneNotifyModeToggle'
import { SceneAddToCalendar } from './SceneAddToCalendar'
import { SceneGraph, SCENE_ARTISTS_ANCHOR } from './SceneGraph'
import { SceneRooms } from './SceneRooms'
import { SceneNewBands } from './SceneNewBands'
import { SceneRoster } from './SceneRoster'
import { SceneCollections } from './SceneCollections'
import { SceneCrews } from './SceneCrews'
import { SceneGapLine } from './SceneGapLine'
import { formatTimeZoneLabel, sceneStatParts } from '../sceneCalendar'
import type { SceneDetail } from '../types'

interface SceneDetailProps {
  slug: string
  /**
   * The calendar slice, rendered on the SERVER and handed in as a slot.
   *
   * A slot rather than a `slice` data prop: the slice has no interactivity, so
   * passing its rows through this client boundary would serialize every one of
   * them into the flight payload on top of the HTML they already produce. As a
   * slot they cost HTML only. See `app/scenes/[slug]/page.tsx`.
   *
   * REQUIRED. The calendar is the first thing under the header and the reason
   * the page exists; an optional slot would let a caller drop it silently, and
   * the render below would leave no trace that anything was missing.
   */
  calendarSlot: React.ReactNode
  /**
   * IANA zone the slice's times are printed in, from the day payload.
   *
   * Passed as a plain string because the status band is inside this client
   * component. It comes from the BACKEND's answer for the scene rather than the
   * modal vote over fetched rows the old calendar hook had to make, so a scene
   * with nothing booked still names its clock.
   */
  timeZone?: string
  /**
   * Shows listed for tonight, from the same day payload the calendar slot
   * renders.
   *
   * A plain number rather than the slice itself, for the reason `calendarSlot`
   * is a slot: passing the payload across this client boundary would serialize
   * every row of it into the flight payload a second time.
   *
   * Absent when nothing answered for tonight. The band draws the clause only
   * for a count above zero, so absent and zero reach the reader alike.
   */
  tonightShowCount?: number
}

/**
 * Quiet metadata under the site nav: what is on tonight, how much is here, how
 * many rooms this page speaks for, and which clock every time below is on.
 *
 * Wave 1A copied ShowStatusStripe's inverse fill (`bg-foreground
 * text-background`). That token swap matches Figma DENSE 1335:16 in light
 * mode, and in dark mode it becomes a cream masthead. The mock also drew
 * the strip as the top of the page — no site chrome in the file. Under the
 * real nav, a filled invert is a second header. PSY-1815 demotes it to a
 * hairline + muted mono so it recedes in both themes.
 *
 * City/state is not in this line. Breadcrumb and H1 already name the place.
 *
 * The volume clause is the SPARSE frame's spelling (`1 UPCOMING SHOW`), which
 * is `upcoming_show_count` straight off this payload. It repeats a number the
 * stat line also carries, and so does the mock: the band is read at a glance
 * and the stat line is read as a sentence.
 *
 * The TONIGHT clause LEADS, because it is the clause a reader opening this page
 * is scanning for. It is drawn only above zero, never as "NOTHING TONIGHT":
 * this band speaks for the rooms this page tracks, and a zero in it would read
 * as a claim about the city. So a quiet night and a night nothing answered for
 * are one rendering, and neither states a number.
 *
 * ONE clause the mock draws is absent, because no honest number exists for it
 * here: `THIS WEEK n`. `GET /scenes/{slug}` carries no calendar-week field;
 * only `GET /scenes` does (`shows_calendar_week`). Labelling
 * `upcoming_show_count` "this week" would put 328 against a week page that
 * says 22.
 */
function SceneStatusBand({
  scene,
  timeZone,
  tonightShowCount,
}: {
  scene: SceneDetail
  timeZone?: string
  tonightShowCount?: number
}) {
  const zoneLabel = timeZone ? formatTimeZoneLabel(new Date(), timeZone) : null

  const { stats } = scene
  const parts = [
    // No plural branch: "tonight" is the noun the number counts against, and it
    // reads the same at one as at nine.
    tonightShowCount ? `${tonightShowCount} tonight` : null,
    `${stats.upcoming_show_count} upcoming show${stats.upcoming_show_count === 1 ? '' : 's'}`,
    `${stats.venue_count} room${stats.venue_count === 1 ? '' : 's'} tracked`,
    zoneLabel && `all times ${zoneLabel}`,
  ].filter(Boolean)

  // The negative margins cancel `app/scenes/[slug]/page.tsx`'s `px-4 py-8
  // md:px-8` so the hairline reaches the edges of the content column instead
  // of sitting inside its gutter. That is a real coupling to the route shell:
  // if the page's padding changes, these have to change with it.
  return (
    <div
      data-testid="scene-status-band"
      className="-mx-4 -mt-8 mb-6 border-b border-border px-4 py-2.5 text-muted-foreground md:-mx-8 md:px-8"
    >
      <p className="font-mono text-[11px] uppercase tracking-widest">
        {parts.join(' · ')}
      </p>
    </div>
  )
}

export function SceneDetailView({
  slug,
  calendarSlot,
  timeZone,
  tonightShowCount,
}: SceneDetailProps) {
  const { data: scene, isLoading, error } = useSceneDetail(slug)

  if (isLoading) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (error || !scene) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <div className="text-center">
          <MapPin className="h-12 w-12 mx-auto text-muted-foreground/50 mb-4" />
          <h1 className="text-2xl font-bold mb-2">Scene not found</h1>
          <p className="text-muted-foreground text-sm mb-4">
            This scene page doesn&apos;t exist or there isn&apos;t enough activity yet.
          </p>
          <Link
            href="/scenes"
            className="text-sm text-primary hover:underline"
          >
            Browse all scenes
          </Link>
        </div>
      </div>
    )
  }

  const { stats } = scene

  return (
    <div>
      <SceneStatusBand
        scene={scene}
        timeZone={timeZone}
        tonightShowCount={tonightShowCount}
      />

      <header>
        <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
          <Link href="/scenes" className="transition-colors hover:text-foreground">
            Scenes
          </Link>
          {' › '}
          <span>
            {scene.city}, {scene.state}
          </span>
        </nav>

        <h1 className="mt-1 text-3xl font-bold">
          {scene.city}, {scene.state}
        </h1>

        {/* The authored tagline (PSY-1848). Four to eight words, written by an
            admin, and the absent state is simply NOTHING — no placeholder, no
            "add a tagline" prompt, and no line derived from the data, which is
            how the locked sparse frame draws it. Most scenes have none.

            `scene.description` is deliberately NOT a fallback here: it asked
            for a paragraph rather than a headline, and rendering it in this
            slot was part of the PSY-1783 kill set. An empty tagline must read
            as absent, so the guard is on trimmed content, not on definedness —
            a whitespace-only value would otherwise reserve a line of height
            for nothing. */}
        {scene.tagline?.trim() ? (
          <p className="mt-1 text-lg text-muted-foreground">{scene.tagline}</p>
        ) : null}

        {/* Every part kept, including the zeroes. Dropping a zero-valued part
            made London read `2 venues · 197 upcoming shows`, as if artists were
            never a category this page tracks. */}
        <p className="mt-1 font-mono text-sm text-muted-foreground">
          {sceneStatParts(stats).join(' · ')}
        </p>

        {/* The crews and collectives chip row. A crew is a TAG, not an entity,
            so each chip lands on the tag page rather than on a page of its
            own. Absent, with no header and no scaffold, on the scenes no crew
            tag reaches. */}
        <SceneCrews scene={scene} />

        {/* Follow-a-scene (PSY-1340) + notify mode (PSY-1341), plus share and
            the scene .ics feed (PSY-1785 / locked P6). */}
        <div className="mt-3 flex flex-wrap items-center gap-3">
          <FollowButton entityType="scenes" entityId={slug} />
          <SceneNotifyModeToggle slug={slug} />
          <ShareButton
            path={`/scenes/${scene.slug}`}
            variant="bracket"
            ariaLabel="Share this scene"
          />
          <SceneAddToCalendar slug={scene.slug} />
        </div>
      </header>

      <div className="mt-6">{calendarSlot}</div>

      {/* The identity around the calendar, in the mock's order: the rooms this
          page speaks for, the bands that just appeared, the bands that live
          here, the collections about the place, what the place is missing,
          then the map.

          Every one of them is empty-capable and every one of them HIDES or
          SUBSTITUTES rather than scaffolding (decision 11). Wave 1A left the
          two cards these replace rendering `0 venues in X` and a collapsed
          roster stub, because its own ticket said both "hide empty modules" and
          "keep existing modules in their current form"; this wave rebuilds them
          and the sparse matrix governs. */}
      {/* The slug-suffixed keys on the two stateful sections are LOAD-BEARING,
          not decoration. This view is one component across every /scenes/{slug}
          route, so React reuses the same instances on a scene-to-scene
          navigation and their state would ride along: a reader who flipped
          Phoenix's rooms to alphabetical would land on the next scene still
          flipped, and one who expanded a 340-band roster would send `limit=100`
          at a scene with nine bands. Keying by the identity the state is ABOUT
          resets both without a single effect.

          The prefixes are required, not stylistic: keys must be unique among
          SIBLINGS, and a bare `scene.slug` on both is two children with the
          same key. */}
      <div className="mt-10 space-y-8">
        <SceneRooms key={`rooms-${scene.slug}`} scene={scene} />

        <SceneNewBands scene={scene} />

        {/* anchorId: the mobile graph teaser's link-out target (SceneGraph,
            PSY-1472). It travels with the roster because that is the section
            the teaser is sending the reader to. */}
        <SceneRoster
          key={`roster-${scene.slug}`}
          scene={scene}
          anchorId={SCENE_ARTISTS_ANCHOR}
        />

        <SceneCollections scene={scene} />

        <SceneGapLine scene={scene} />

        {/* Scene graph (PSY-367): read-only artist relationship map. Section
            self-hides when edge_count < 8 (locked decision 14) or the
            container is mobile. */}
        <SceneGraph slug={slug} city={scene.city} state={scene.state} />

        {/* The editorial slot sits HERE, and renders nothing.
            Scene reports are a later decision with no field behind them yet
            (brief decision 9). The locked sparse frame draws the absent state
            as simply nothing: no placeholder, no prompt, and no line derived
            from the data. */}

        {stats.festival_count > 0 && (
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="flex items-center gap-2 text-base">
                <Tent className="h-4 w-4 text-muted-foreground" />
                Festivals
              </CardTitle>
            </CardHeader>
            <CardContent>
              <p className="text-sm text-muted-foreground mb-3">
                {stats.festival_count} festival{stats.festival_count !== 1 ? 's' : ''} in {scene.city}.
              </p>
              <Link
                href="/festivals"
                className="inline-flex items-center gap-1.5 text-sm font-medium text-primary hover:underline"
              >
                View festivals
                <ArrowRight className="h-3.5 w-3.5" />
              </Link>
            </CardContent>
          </Card>
        )}
      </div>
    </div>
  )
}
