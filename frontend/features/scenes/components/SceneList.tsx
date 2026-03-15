'use client'

import Link from 'next/link'
import { MapPin, Building2, Calendar } from 'lucide-react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { LoadingSpinner } from '@/components/shared'
import { useScenes } from '../hooks'

export function SceneList() {
  const { data, isLoading, error } = useScenes()

  if (isLoading) {
    return (
      <div className="flex justify-center items-center py-12">
        <LoadingSpinner />
      </div>
    )
  }

  if (error) {
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
          Scene pages appear for cities with enough venue and show activity.
          Check back as the community grows.
        </p>
      </div>
    )
  }

  return (
    <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
      {data.scenes.map((scene) => (
        <Link key={scene.slug} href={`/scenes/${scene.slug}`}>
          <Card className="h-full transition-colors hover:bg-muted/50 cursor-pointer">
            <CardHeader className="pb-2">
              <CardTitle className="flex items-center gap-2 text-lg">
                <MapPin className="h-4 w-4 text-muted-foreground" />
                {scene.city}, {scene.state}
              </CardTitle>
            </CardHeader>
            <CardContent>
              <div className="flex items-center gap-4 text-sm text-muted-foreground">
                <span className="flex items-center gap-1.5">
                  <Building2 className="h-3.5 w-3.5" />
                  {scene.venue_count} venue{scene.venue_count !== 1 ? 's' : ''}
                </span>
                <span className="flex items-center gap-1.5">
                  <Calendar className="h-3.5 w-3.5" />
                  {scene.upcoming_show_count} upcoming show{scene.upcoming_show_count !== 1 ? 's' : ''}
                </span>
              </div>
            </CardContent>
          </Card>
        </Link>
      ))}
    </div>
  )
}
