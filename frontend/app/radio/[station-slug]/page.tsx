'use client'

import { use } from 'react'
import Link from 'next/link'
import {
  ArrowLeft,
  Loader2,
  Radio,
  MapPin,
  ExternalLink,
  Heart,
  Globe,
  Music,
  Disc3,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import {
  useRadioStation,
  useRadioShows,
  useNewReleaseRadar,
  RadioShowCard,
  getBroadcastTypeLabel,
} from '@/features/radio'
import type { RadioNewReleaseRadarEntry } from '@/features/radio'

interface StationPageProps {
  params: Promise<{ 'station-slug': string }>
}

function NewReleaseRadarSection({ stationId }: { stationId: number }) {
  const { data, isLoading } = useNewReleaseRadar({ stationId, limit: 10 })

  if (isLoading || !data?.releases || data.releases.length === 0) return null

  return (
    <section className="mt-10">
      <h2 className="text-xl font-bold mb-4 flex items-center gap-2">
        <Disc3 className="h-5 w-5" />
        New Release Radar
      </h2>
      <div className="space-y-1">
        {data.releases.map((entry: RadioNewReleaseRadarEntry, i: number) => (
          <div
            key={`${entry.artist_name}-${entry.album_title}-${i}`}
            className="flex items-center gap-3 px-3 py-2 rounded-md hover:bg-muted/50 transition-colors"
          >
            <span className="text-xs text-muted-foreground tabular-nums w-5 text-right">
              {i + 1}
            </span>
            <div className="flex-1 min-w-0">
              <div className="flex items-center gap-2">
                {entry.artist_slug ? (
                  <Link
                    href={`/artists/${entry.artist_slug}`}
                    className="text-sm font-medium hover:text-primary transition-colors"
                  >
                    {entry.artist_name}
                  </Link>
                ) : (
                  <span className="text-sm font-medium">{entry.artist_name}</span>
                )}
                {entry.album_title && (
                  <>
                    <span className="text-muted-foreground">-</span>
                    {entry.release_slug ? (
                      <Link
                        href={`/releases/${entry.release_slug}`}
                        className="text-sm text-muted-foreground hover:text-foreground transition-colors truncate"
                      >
                        {entry.album_title}
                      </Link>
                    ) : (
                      <span className="text-sm text-muted-foreground truncate">
                        {entry.album_title}
                      </span>
                    )}
                  </>
                )}
              </div>
              {entry.label_name && (
                <span className="text-xs text-muted-foreground">
                  {entry.label_slug ? (
                    <Link
                      href={`/labels/${entry.label_slug}`}
                      className="hover:text-foreground transition-colors"
                    >
                      {entry.label_name}
                    </Link>
                  ) : (
                    entry.label_name
                  )}
                </span>
              )}
            </div>
            <div className="shrink-0 text-xs text-muted-foreground tabular-nums">
              {entry.play_count} plays
              {entry.station_count > 1 && ` / ${entry.station_count} stations`}
            </div>
          </div>
        ))}
      </div>
    </section>
  )
}

export default function StationPage({ params }: StationPageProps) {
  const { 'station-slug': stationSlug } = use(params)
  const { data: station, isLoading, error } = useRadioStation(stationSlug)
  const { data: showsData, isLoading: showsLoading } = useRadioShows(station?.id)

  if (isLoading) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (error || !station) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <div className="text-center">
          <h1 className="text-2xl font-bold mb-2">Station Not Found</h1>
          <p className="text-muted-foreground mb-4">
            The radio station you&apos;re looking for doesn&apos;t exist.
          </p>
          <Button asChild variant="outline">
            <Link href="/radio">
              <ArrowLeft className="h-4 w-4 mr-2" />
              Back to Radio
            </Link>
          </Button>
        </div>
      </div>
    )
  }

  const location = [station.city, station.state].filter(Boolean).join(', ')
  const broadcastLabel = getBroadcastTypeLabel(station.broadcast_type)

  return (
    <div className="flex min-h-screen items-start justify-center">
      <main className="w-full max-w-6xl px-4 py-8 md:px-8">
        {/* Breadcrumb */}
        <div className="mb-6">
          <Link
            href="/radio"
            className="text-sm text-muted-foreground hover:text-foreground transition-colors flex items-center gap-1"
          >
            <ArrowLeft className="h-3.5 w-3.5" />
            Radio
          </Link>
        </div>

        {/* Station header */}
        <div className="flex items-start gap-5 mb-8">
          {/* Logo */}
          <div className="shrink-0 rounded-xl bg-muted/50 flex items-center justify-center overflow-hidden h-20 w-20">
            {station.logo_url ? (
              <img
                src={station.logo_url}
                alt={`${station.name} logo`}
                className="h-full w-full object-cover"
              />
            ) : (
              <Radio className="h-10 w-10 text-muted-foreground/40" />
            )}
          </div>

          <div className="flex-1 min-w-0">
            <h1 className="text-3xl font-bold">{station.name}</h1>

            <div className="flex items-center gap-3 flex-wrap mt-2">
              <Badge variant="secondary">{broadcastLabel}</Badge>
              {station.frequency_mhz && (
                <span className="text-sm text-muted-foreground tabular-nums">
                  {station.frequency_mhz} MHz
                </span>
              )}
              {location && (
                <span className="flex items-center gap-1 text-sm text-muted-foreground">
                  <MapPin className="h-3.5 w-3.5" />
                  {location}
                </span>
              )}
              {station.show_count > 0 && (
                <span className="flex items-center gap-1 text-sm text-muted-foreground">
                  <Music className="h-3.5 w-3.5" />
                  {station.show_count} shows
                </span>
              )}
            </div>

            {station.description && (
              <p className="text-muted-foreground mt-3 text-sm leading-relaxed max-w-3xl">
                {station.description}
              </p>
            )}

            {/* Action buttons */}
            <div className="flex items-center gap-2 mt-4">
              {station.stream_url && (
                <Button asChild size="sm">
                  <a
                    href={station.stream_url}
                    target="_blank"
                    rel="noopener noreferrer"
                  >
                    <Radio className="h-4 w-4 mr-2" />
                    Listen Live
                  </a>
                </Button>
              )}
              {station.donation_url && (
                <Button asChild variant="outline" size="sm">
                  <a
                    href={station.donation_url}
                    target="_blank"
                    rel="noopener noreferrer"
                  >
                    <Heart className="h-4 w-4 mr-2" />
                    Donate
                  </a>
                </Button>
              )}
              {station.website && (
                <Button asChild variant="ghost" size="sm">
                  <a
                    href={station.website}
                    target="_blank"
                    rel="noopener noreferrer"
                  >
                    <Globe className="h-4 w-4 mr-2" />
                    Website
                    <ExternalLink className="h-3 w-3 ml-1" />
                  </a>
                </Button>
              )}
            </div>
          </div>
        </div>

        {/* Shows */}
        <section>
          <h2 className="text-xl font-bold mb-4 flex items-center gap-2">
            <Music className="h-5 w-5" />
            Shows
          </h2>

          {showsLoading && (
            <div className="flex justify-center py-8">
              <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
            </div>
          )}

          {!showsLoading && showsData?.shows && showsData.shows.length > 0 ? (
            <div className="grid gap-3 md:grid-cols-2">
              {showsData.shows.map(show => (
                <RadioShowCard
                  key={show.id}
                  show={show}
                  stationSlug={stationSlug}
                />
              ))}
            </div>
          ) : !showsLoading ? (
            <div className="py-8 text-center text-sm text-muted-foreground">
              No shows yet
            </div>
          ) : null}
        </section>

        {/* New Release Radar */}
        <NewReleaseRadarSection stationId={station.id} />
      </main>
    </div>
  )
}
