'use client'

import Link from 'next/link'
import { MapPin, Building2, Calendar, Music } from 'lucide-react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { LoadingSpinner } from '@/components/shared'
import { useScenes } from '../hooks'
import { formatShowCountLine } from '../sceneWeek'

export function SceneList() {
  const { data, isLoading, error } = useScenes()

  if (isLoading) {
    return (
      <div className="flex justify-center items-center py-12">
        <LoadingSpinner />
      </div>
    )
  }

  // `&& !data`: an error only replaces the grid when there is no grid to show.
  // The server-seeded first screen is stale by construction (PSY-1624), so
  // every load forces a revalidation; without this guard one failed background
  // refetch would swap a fully rendered page for an error message.
  if (error && !data) {
    return (
      <div className="text-center py-12">
        <p className="text-muted-foreground">
          Failed to load scenes. Please try again later.
        </p>
      </div>
    )
  }

  if (!data?.scenes || data.scenes.length === 0) {
    return (
      <div className="text-center py-12">
        <MapPin className="h-12 w-12 mx-auto text-muted-foreground/50 mb-4" />
        <h2 className="text-lg font-medium mb-2">No scenes yet</h2>
        <p className="text-muted-foreground text-sm max-w-md mx-auto">
          Scene pages appear for cities with venue and show activity.
          Check back as the community grows.
        </p>
      </div>
    )
  }

  return (
    <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
      {data.scenes.map((scene) => (
        /* Two destinations on one card, and the card is no longer wrapped in a
           single `<Link>` because a second anchor inside the first is invalid
           HTML: the parser lifts the inner anchor out, so what a crawler is
           handed stops matching what was written.

           So the card link is an overlay element covering the card, and the
           week link sits above it (positioned, and later in tree order). The
           overlay is a DIRECT CHILD of the `relative` card, deliberately: an
           `::after` on the title inside `CardHeader` would have depended on
           whether `container-type: inline-size` — which `CardHeader` sets —
           makes that header the containing block for absolutely positioned
           descendants. Chromium says it does not (measured: a click 27px below
           the header still reached the card link), but that is a subtle rule to
           rest a whole card's click target on, and it is not worth a
           cross-browser bet when parenting the overlay to the card answers the
           question outright. */
        <Card
          key={scene.slug}
          className="relative h-full transition-colors hover:bg-muted/50"
        >
          <Link
            href={`/scenes/${scene.slug}`}
            aria-label={`${scene.city}, ${scene.state}`}
            className="absolute inset-0 rounded-lg"
          />
          <CardHeader className="pb-2">
            <CardTitle className="flex items-center gap-2 text-lg">
              <MapPin className="h-4 w-4 text-muted-foreground" />
              {scene.city}, {scene.state}
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="flex items-center gap-4 text-sm text-muted-foreground flex-wrap">
              <span className="flex items-center gap-1.5">
                <Building2 className="h-3.5 w-3.5" />
                {scene.venue_count} venue{scene.venue_count !== 1 ? 's' : ''}
              </span>
              <span className="flex items-center gap-1.5">
                <Music className="h-3.5 w-3.5" />
                {scene.total_show_count} show{scene.total_show_count !== 1 ? 's' : ''}
              </span>
              {scene.upcoming_show_count > 0 && (
                <span className="flex items-center gap-1.5">
                  <Calendar className="h-3.5 w-3.5" />
                  {scene.upcoming_show_count} upcoming
                </span>
              )}
            </div>
            {/* Rendered for every scene, including the quiet ones (PSY-1623):
                the week page is otherwise unreachable, and a link that appears
                only in the weeks a city is busy is one a crawler cannot rely
                on. A quiet scene drops the accent, for the same reason the
                `/shows` block mutes its zero rows: the point of always linking
                them is that emptiness costs nothing to look at, and "No shows
                this week" is the last thing a card should say loudest.

                `relative` lifts it above the card-wide link overlay. The
                `aria-label` names the city because the visible text does not:
                on a quiet week a dozen cards would otherwise offer a dozen
                links all called "No shows this week", pointing at a dozen
                different pages. */}
            <Link
              href={`/scenes/${scene.slug}/week`}
              aria-label={`${scene.city}, ${scene.state}, ${formatShowCountLine(scene.shows_this_week, true)}`}
              className={`relative mt-2 inline-block text-sm underline underline-offset-2 hover:no-underline ${
                scene.shows_this_week === 0 ? 'text-muted-foreground' : 'text-primary'
              }`}
            >
              {formatShowCountLine(scene.shows_this_week, true)}{' '}
              <span aria-hidden="true">→</span>
            </Link>
          </CardContent>
        </Card>
      ))}
    </div>
  )
}
