'use client'

import Link from 'next/link'
import {
  MapPin, Building2, Mic2, Tent, ArrowRight, Loader2,
} from 'lucide-react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { buildCitiesParam } from '@/components/filters/cityParams'
import { useSceneDetail, useSceneArtists } from '../hooks'
import { FollowButton } from '@/components/shared/FollowButton'
import { SceneNotifyModeToggle } from './SceneNotifyModeToggle'
import { SceneGraph, SCENE_ARTISTS_ANCHOR } from './SceneGraph'
import { SceneCalendar, useSceneCalendarWindow } from './SceneCalendar'
import { formatTimeZoneLabel, sceneStatParts } from '../sceneCalendar'
import type { SceneDetail } from '../types'

interface SceneDetailProps {
  slug: string
}

function SceneArtistsList({ slug }: { slug: string }) {
  const { data, isLoading } = useSceneArtists({ slug, limit: 10 })

  if (isLoading) {
    return (
      <div className="flex justify-center py-4">
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (!data?.artists || data.artists.length === 0) {
    return (
      <p className="text-sm text-muted-foreground py-2">
        No artists based in this scene yet.
      </p>
    )
  }

  return (
    <div className="space-y-1.5">
      {data.artists.map((artist) => (
        <Link
          key={artist.id}
          href={`/artists/${artist.slug}`}
          className="flex items-center justify-between rounded-md px-3 py-2 text-sm transition-colors hover:bg-muted/50"
        >
          <div className="flex items-center gap-2 min-w-0">
            <Mic2 className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
            <span className="truncate font-medium">{artist.name}</span>
          </div>
          <div className="ml-2 flex shrink-0 items-center gap-1.5">
            {artist.is_active && (
              <Badge variant="success" className="text-xs" title="Has an upcoming show or one in the last ~6 months">
                Active
              </Badge>
            )}
            <Badge variant="secondary" className="text-xs">
              {artist.show_count} show{artist.show_count !== 1 ? 's' : ''}
            </Badge>
          </div>
        </Link>
      ))}
      {data.total > 10 && (
        <p className="text-xs text-muted-foreground px-3 pt-1">
          and {data.total - 10} more artist{data.total - 10 !== 1 ? 's' : ''}
        </p>
      )}
    </div>
  )
}

/**
 * The band across the top of the page: where you are, what is on tonight, how
 * many rooms this page speaks for, and which clock every time below is on.
 *
 * The tonight clause is the SAME number the tonight group renders below, by
 * construction. The calendar window opens at now, so both are answering "what
 * is still ahead", and the two must not be allowed to drift apart. The zero
 * state is the mock's words rather than a numeral: "nothing tonight".
 *
 * Both clauses DROP when the scene's timezone cannot be resolved from the rows.
 * "Tonight" is a same-day claim, and a same-day claim made in the reader's own
 * timezone about a city they are not in is a confidently wrong one.
 *
 * DELIBERATELY NOT the mock's `THIS WEEK n`. `GET /scenes/{slug}` carries no
 * calendar-week field; only `GET /scenes` does (`shows_calendar_week`). The
 * only count on this payload is `upcoming_show_count`, which spans every future
 * show, so labelling that "this week" would put 328 against a week page that
 * says 22, the exact defect PSY-1623 removed from two other surfaces. Counting
 * the fetched rows instead would be wrong the other way on any dense scene,
 * because the window is capped at 20. Add the field to the detail payload
 * first, then draw the number.
 */
function SceneStatusBand({ scene }: { scene: SceneDetail }) {
  const { timeZone, tonightCount } = useSceneCalendarWindow(scene.slug)
  const zoneLabel = timeZone ? formatTimeZoneLabel(new Date(), timeZone) : null

  const { stats } = scene
  const tonight =
    tonightCount === null
      ? null
      : tonightCount === 0
        ? 'nothing tonight'
        : `tonight ${tonightCount} show${tonightCount === 1 ? '' : 's'}`

  const parts = [
    `${scene.city}, ${scene.state}`,
    tonight,
    `${stats.venue_count} room${stats.venue_count === 1 ? '' : 's'} tracked`,
    zoneLabel && `all times ${zoneLabel}`,
  ].filter(Boolean)

  // The negative margins cancel `app/scenes/[slug]/page.tsx`'s `px-4 py-8
  // md:px-8` so the band reaches the edges of the content column instead of
  // sitting inside its gutter. That is a real coupling to the route shell: if
  // the page's padding changes, these have to change with it.
  return (
    <div className="-mx-4 -mt-8 mb-6 bg-foreground px-4 py-2.5 text-background md:-mx-8 md:px-8">
      <p className="font-mono text-[11px] uppercase tracking-widest">
        {parts.join(' · ')}
      </p>
    </div>
  )
}

export function SceneDetailView({ slug }: SceneDetailProps) {
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
      <SceneStatusBand scene={scene} />

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

        {/* The tagline slot sits HERE, and renders nothing.
            Authored scene taglines are a Wave 3 decision with no field behind
            them yet, and the locked sparse frame draws the absent state as
            simply nothing: no placeholder, no "add a tagline" prompt, and no
            line derived from the data. `scene.description` is NOT that field.
            0 of 28 production scenes populate it, it asked for a paragraph
            rather than four to eight words, and rendering it here was part of
            the kill set. */}

        {/* Every part kept, including the zeroes. Dropping a zero-valued part
            made London read `2 venues · 197 upcoming shows`, as if artists were
            never a category this page tracks. */}
        <p className="mt-1 font-mono text-sm text-muted-foreground">
          {sceneStatParts(stats).join(' · ')}
        </p>

        {/* Follow-a-scene (PSY-1340) + notify mode (PSY-1341), moved under the
            stat line into the mock's action row. Share and the scene .ics feed
            join them in a later wave; neither exists for a scene today. */}
        <div className="mt-3 flex flex-wrap items-center gap-3">
          <FollowButton entityType="scenes" entityId={slug} />
          <SceneNotifyModeToggle slug={slug} />
        </div>

        {/* The crews and collectives chip row sits HERE, and renders nothing.
            The crew entity is DEFERRED (brief decision 7): the row is reserved
            in the mock so the position is settled, and until there is data it
            is absent rather than an empty header over blank space. */}
      </header>

      <div className="mt-6">
        <SceneCalendar scene={scene} />
      </div>

      {/* Everything below is the mock's order with each module in its CURRENT
          form, wording included. Refining them (named venue list with counts,
          roster with open embeds and descriptors, the graph's gate and copy) is
          Wave 1B/1C.
          KNOWN GAP, disclosed rather than guessed at: these two cards still
          render a header on a scene with nothing in them (`0 venues in X`, and
          the roster's "No artists based in this scene yet"). The sparse rule
          says an empty-capable module hides; the "current form" rule says do
          not touch them this wave. The ticket says both, so 1A leaves them and
          1B closes it rather than this wave picking a side. */}
      <div className="mt-10 space-y-4">
        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="flex items-center gap-2 text-base">
              <Building2 className="h-4 w-4 text-muted-foreground" />
              Venues
            </CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-sm text-muted-foreground mb-3">
              {stats.venue_count} venue{stats.venue_count !== 1 ? 's' : ''} in {scene.city}.
            </p>
            <Link
              href={`/venues?cities=${encodeURIComponent(buildCitiesParam([{ city: scene.city, state: scene.state }]))}`}
              className="inline-flex items-center gap-1.5 text-sm font-medium text-primary hover:underline"
            >
              View all venues
              <ArrowRight className="h-3.5 w-3.5" />
            </Link>
          </CardContent>
        </Card>

        {/* Local Artists: the metro roster, active bands first.
            id="scene-artists": the mobile graph teaser's link-out target
            (SceneGraph, PSY-1472). scroll-mt for the sticky entity header. */}
        <Card id={SCENE_ARTISTS_ANCHOR} className="scroll-mt-20">
          <CardHeader className="pb-3">
            <CardTitle className="flex items-center gap-2 text-base">
              <Mic2 className="h-4 w-4 text-muted-foreground" />
              Local Artists
              <span className="text-xs font-normal text-muted-foreground">(based here · active highlighted)</span>
            </CardTitle>
          </CardHeader>
          <CardContent>
            <SceneArtistsList slug={slug} />
          </CardContent>
        </Card>

        {/* Scene graph (PSY-367): read-only artist relationship map. Section
            self-hides when there are <3 connected artists or container is mobile. */}
        <SceneGraph slug={slug} city={scene.city} state={scene.state} />

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
