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
        /* Two destinations, one card, and no nested anchors — which is why the
           card is no longer wrapped in a single `<Link>`. The title's link is
           stretched over the whole card by an `::after` overlay, so the card
           stays entirely clickable, and the week link sits above that overlay.
           Nesting the second link inside the first would have been invalid
           HTML: the parser lifts the inner anchor out, and what a crawler is
           handed stops matching what was written. */
        <Card
          key={scene.slug}
          className="relative h-full transition-colors hover:bg-muted/50"
        >
          <CardHeader className="pb-2">
            <CardTitle className="flex items-center gap-2 text-lg">
              <MapPin className="h-4 w-4 text-muted-foreground" />
              <Link
                href={`/scenes/${scene.slug}`}
                className="after:absolute after:inset-0 after:content-['']"
              >
                {scene.city}, {scene.state}
              </Link>
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
                on. `relative` lifts it out of the title link's overlay. */}
            <Link
              href={`/scenes/${scene.slug}/week`}
              className="relative mt-2 inline-block text-sm text-primary underline underline-offset-2 hover:no-underline"
            >
              {formatShowCountLine(scene.shows_this_week, true)} →
            </Link>
          </CardContent>
        </Card>
      ))}
    </div>
  )
}
