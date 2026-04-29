'use client'

import Link from 'next/link'
import {
  MapPin, Building2, Mic2, Calendar, Tent, ArrowRight, Loader2, Music,
} from 'lucide-react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { TagPill } from '@/components/shared'
import { useSceneDetail, useSceneArtists, useSceneGenres } from '../hooks'
import { ScenePulse } from './ScenePulse'
import { SceneGraph } from './SceneGraph'

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
        No active artists in the last 90 days.
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
          <Badge variant="secondary" className="ml-2 shrink-0 text-xs">
            {artist.show_count} show{artist.show_count !== 1 ? 's' : ''}
          </Badge>
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

function SceneGenreDistribution({ slug }: { slug: string }) {
  const { data, isLoading } = useSceneGenres(slug)

  if (isLoading) {
    return (
      <div className="flex justify-center py-4">
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (!data?.genres || data.genres.length === 0) {
    return null
  }

  return (
    <Card className="lg:col-span-2">
      <CardHeader className="pb-3">
        <CardTitle className="flex items-center gap-2 text-base">
          <Music className="h-4 w-4 text-muted-foreground" />
          Genre Distribution
          {data.diversity_label && (
            <Badge variant="secondary" className="ml-1 text-xs font-normal">
              {data.diversity_label}
            </Badge>
          )}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <div className="flex flex-wrap gap-2">
          {data.genres.map((genre) => (
            <TagPill
              key={genre.tag_id}
              label={genre.name}
              voteCount={genre.count}
              href={`/tags/${genre.slug}`}
            />
          ))}
        </div>
      </CardContent>
    </Card>
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
  const statParts = [
    stats.venue_count > 0 && `${stats.venue_count} venue${stats.venue_count !== 1 ? 's' : ''}`,
    stats.artist_count > 0 && `${stats.artist_count} artist${stats.artist_count !== 1 ? 's' : ''}`,
    stats.upcoming_show_count > 0 && `${stats.upcoming_show_count} upcoming show${stats.upcoming_show_count !== 1 ? 's' : ''}`,
  ].filter(Boolean)

  return (
    <div className="space-y-6">
      {/* Header */}
      <div>
        <div className="flex items-center gap-2 text-sm text-muted-foreground mb-1">
          <Link href="/scenes" className="hover:text-foreground transition-colors">
            Scenes
          </Link>
          <span>/</span>
        </div>
        <h1 className="text-3xl font-bold">
          {scene.city}, {scene.state}
        </h1>
        {statParts.length > 0 && (
          <p className="text-muted-foreground mt-1">
            {statParts.join(' \u00B7 ')}
          </p>
        )}
        {scene.description && (
          <p className="text-muted-foreground mt-3 max-w-2xl">
            {scene.description}
          </p>
        )}
      </div>

      {/* Scene Pulse */}
      <ScenePulse pulse={scene.pulse} />

      {/* Scene graph (PSY-367) — read-only artist relationship map. Section
          self-hides when there are <3 connected artists or container is mobile. */}
      <SceneGraph slug={slug} city={scene.city} state={scene.state} />

      {/* Content sections */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        {/* Upcoming Shows */}
        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="flex items-center gap-2 text-base">
              <Calendar className="h-4 w-4 text-muted-foreground" />
              Upcoming Shows
            </CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-sm text-muted-foreground mb-3">
              {stats.upcoming_show_count} show{stats.upcoming_show_count !== 1 ? 's' : ''} coming up in {scene.city}.
            </p>
            <Link
              href={`/shows?city=${encodeURIComponent(scene.city)}&state=${encodeURIComponent(scene.state)}`}
              className="inline-flex items-center gap-1.5 text-sm font-medium text-primary hover:underline"
            >
              View upcoming shows
              <ArrowRight className="h-3.5 w-3.5" />
            </Link>
          </CardContent>
        </Card>

        {/* Top Venues */}
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
              href={`/venues?city=${encodeURIComponent(scene.city)}&state=${encodeURIComponent(scene.state)}`}
              className="inline-flex items-center gap-1.5 text-sm font-medium text-primary hover:underline"
            >
              View all venues
              <ArrowRight className="h-3.5 w-3.5" />
            </Link>
          </CardContent>
        </Card>

        {/* Active Artists */}
        <Card className="lg:col-span-2">
          <CardHeader className="pb-3">
            <CardTitle className="flex items-center gap-2 text-base">
              <Mic2 className="h-4 w-4 text-muted-foreground" />
              Active Artists
              <span className="text-xs font-normal text-muted-foreground">(last 90 days)</span>
            </CardTitle>
          </CardHeader>
          <CardContent>
            <SceneArtistsList slug={slug} />
          </CardContent>
        </Card>

        {/* Genre Distribution */}
        <SceneGenreDistribution slug={slug} />

        {/* Festivals (only show if there are festivals) */}
        {stats.festival_count > 0 && (
          <Card className="lg:col-span-2">
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
