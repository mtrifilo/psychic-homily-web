'use client'

import Link from 'next/link'
import {
  ArrowLeft,
  ExternalLink,
  Globe,
  Heart,
  Loader2,
  Radio,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  useRadioStation,
  NetworkTabBar,
  StationOnAirBox,
  StationGraph,
  StationPlaylistsFeed,
  StationShowsDirectory,
  StationSidebar,
  getBroadcastTypeLabel,
} from '@/features/radio'

interface StationDetailProps {
  stationSlug: string
}

/**
 * Station page (PSY-1050, Option A "The Dial"): dense editorial layout.
 * Main column leads with the ON AIR box (v1 heuristic — see StationOnAirBox)
 * then the latest-playlists feed and the shows directory table. Sidebar
 * carries station info, 90d top artists/labels, and the station-filtered
 * New Release Radar.
 *
 * Serves INACTIVE stations' archives too — no active-only filtering here.
 * Shared by the flagship/standalone route and the network sub-channel route
 * (PSY-674 routing unchanged).
 */
export default function StationDetail({ stationSlug }: StationDetailProps) {
  const { data: station, isLoading, error } = useRadioStation(stationSlug)

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
  // Mono identity sub-line: "91.1 FM · Jersey City, NJ · FM/AM + Internet"
  const subline = [
    station.frequency_mhz ? `${station.frequency_mhz} FM` : null,
    location || null,
    getBroadcastTypeLabel(station.broadcast_type),
  ]
    .filter(Boolean)
    .join(' · ')

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
        <header className="mb-5">
          <div className="flex flex-wrap items-start justify-between gap-4">
            <div className="min-w-0">
              <div className="flex items-baseline gap-3 flex-wrap">
                {/* Page-level identity: the station name is the H1 for both
                    standalone stations and network members — the network name
                    surfaces through the tab bar below (PSY-674 semantics). */}
                <h1 className="text-3xl font-bold">{station.name}</h1>
                {subline && (
                  <span className="font-mono text-sm text-muted-foreground">
                    {subline}
                  </span>
                )}
              </div>
              {station.description && (
                <p className="text-muted-foreground mt-2 text-sm leading-relaxed max-w-3xl">
                  {station.description}
                </p>
              )}
            </div>

            {/* Actions */}
            <div className="flex items-center gap-2 shrink-0">
              {station.slug === 'wfmu' ? (
                <Button
                  size="sm"
                  onClick={() => {
                    window.open(
                      'https://www.radiorethink.com/tuner/?stationCode=wfmu&stream=hi',
                      'wfmu-player',
                      'width=400,height=700,scrollbars=yes,resizable=yes'
                    )
                  }}
                >
                  <Radio className="h-4 w-4 mr-2" />
                  Listen Live
                </Button>
              ) : station.stream_url ? (
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
              ) : null}
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
        </header>

        {/* PSY-674: channel tabs for network stations (underline idiom). */}
        <NetworkTabBar currentStation={station} />

        {/* Two-column body: main feed + sidebar */}
        <div className="grid gap-8 lg:grid-cols-[minmax(0,1fr)_300px]">
          <div className="flex flex-col gap-8 min-w-0">
            <StationOnAirBox station={station} />
            {/* id="recent-playlists": the mobile graph teaser's link-out target
                (StationGraph, PSY-1472). scroll-mt for the sticky header. */}
            <div id="recent-playlists" className="scroll-mt-20">
              <StationPlaylistsFeed station={station} />
            </div>
            <StationShowsDirectory
              stationId={station.id}
              stationSlug={station.slug}
            />
            {/* PSY-1299: within-station co-occurrence graph. Keyed by slug so
                cluster-toggle state resets when NetworkTabBar navigates to a
                sibling station — the "other" cluster id is shared across
                stations and would otherwise carry a stale hide over. */}
            <StationGraph
              key={station.slug}
              slug={station.slug}
              stationName={station.name}
            />
          </div>
          <StationSidebar station={station} />
        </div>
      </main>
    </div>
  )
}
