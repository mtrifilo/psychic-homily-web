'use client'

import { useState, useMemo } from 'react'
import Link from 'next/link'
import {
  Loader2,
  Calendar,
  MapPin,
  Users,
  Globe,
  Ticket,
  Building2,
} from 'lucide-react'
import {
  useFestival,
  useFestivalArtists,
  useFestivalVenues,
} from '../hooks/useFestivals'
import { EntityDetailLayout, EntityHeader, SocialLinks, FollowButton } from '@/components/shared'
import { TabsContent } from '@/components/ui/tabs'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { FestivalLineup } from './FestivalLineup'
import {
  getFestivalStatusLabel,
  getFestivalStatusVariant,
  formatFestivalLocation,
  formatFestivalDateRange,
} from '../types'

interface FestivalDetailProps {
  idOrSlug: string | number
}

export function FestivalDetail({ idOrSlug }: FestivalDetailProps) {
  const { data: festival, isLoading, error } = useFestival({ idOrSlug })
  const { data: artistsData, isLoading: artistsLoading } = useFestivalArtists({
    festivalIdOrSlug: idOrSlug,
    enabled: !!festival,
  })
  const { data: venuesData, isLoading: venuesLoading } = useFestivalVenues({
    festivalIdOrSlug: idOrSlug,
    enabled: !!festival,
  })
  const [activeTab, setActiveTab] = useState('lineup')

  // Determine if this is a multi-day festival with day assignments
  const hasMultipleDays = useMemo(() => {
    if (!artistsData?.artists) return false
    const uniqueDays = new Set(
      artistsData.artists
        .map(a => a.day_date)
        .filter(Boolean)
    )
    return uniqueDays.size > 1
  }, [artistsData])

  if (isLoading) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (error) {
    const errorMessage =
      error instanceof Error ? error.message : 'Failed to load festival'
    const is404 =
      errorMessage.includes('not found') || errorMessage.includes('404')

    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <div className="text-center">
          <h1 className="text-2xl font-bold mb-2">
            {is404 ? 'Festival Not Found' : 'Error Loading Festival'}
          </h1>
          <p className="text-muted-foreground mb-4">
            {is404
              ? "The festival you're looking for doesn't exist or has been removed."
              : errorMessage}
          </p>
          <Button asChild variant="outline">
            <Link href="/festivals">Back to Festivals</Link>
          </Button>
        </div>
      </div>
    )
  }

  if (!festival) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <div className="text-center">
          <h1 className="text-2xl font-bold mb-2">Festival Not Found</h1>
          <p className="text-muted-foreground mb-4">
            The festival you&apos;re looking for doesn&apos;t exist.
          </p>
          <Button asChild variant="outline">
            <Link href="/festivals">Back to Festivals</Link>
          </Button>
        </div>
      </div>
    )
  }

  const location = formatFestivalLocation(festival)
  const dateRange = formatFestivalDateRange(festival.start_date, festival.end_date)
  const artists = artistsData?.artists ?? []
  const venues = venuesData?.venues ?? []

  const tabs = [
    { value: 'lineup', label: `Lineup (${festival.artist_count})` },
    { value: 'info', label: 'Info' },
  ]

  const sidebar = (
    <div className="space-y-6">
      {/* Festival Flyer or Placeholder */}
      {festival.flyer_url ? (
        <div className="rounded-lg border border-border/50 bg-card overflow-hidden">
          <img
            src={festival.flyer_url}
            alt={`${festival.name} flyer`}
            className="w-full object-contain"
          />
        </div>
      ) : (
        <div className="rounded-lg border border-border/50 bg-card overflow-hidden">
          <div className="w-full aspect-square bg-muted/30 flex items-center justify-center">
            <Calendar className="h-16 w-16 text-muted-foreground/30" />
          </div>
        </div>
      )}

      {/* Quick Info */}
      <div className="rounded-lg border border-border/50 bg-card p-4 space-y-3">
        <h3 className="text-sm font-semibold text-foreground">Details</h3>

        <div className="space-y-2 text-sm">
          <div className="flex items-center gap-2 text-muted-foreground">
            <Calendar className="h-4 w-4 shrink-0" />
            <span>{dateRange}</span>
          </div>

          {location && (
            <div className="flex items-center gap-2 text-muted-foreground">
              <MapPin className="h-4 w-4 shrink-0" />
              <span>{location}</span>
            </div>
          )}

          <div className="flex items-center gap-2 text-muted-foreground">
            <Users className="h-4 w-4 shrink-0" />
            <span>
              {festival.artist_count === 1
                ? '1 artist'
                : `${festival.artist_count} artists`}
            </span>
          </div>

          <div className="flex items-center gap-2 text-muted-foreground">
            <Building2 className="h-4 w-4 shrink-0" />
            <span>
              {festival.venue_count === 1
                ? '1 venue'
                : `${festival.venue_count} venues`}
            </span>
          </div>
        </div>
      </div>

      {/* Venues */}
      {venues.length > 0 && (
        <div className="rounded-lg border border-border/50 bg-card p-4 space-y-3">
          <h3 className="text-sm font-semibold text-foreground">Venues</h3>
          <div className="space-y-2 text-sm">
            {venues.map(venue => (
              <div key={venue.id} className="flex items-center gap-2">
                <Building2 className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
                <Link
                  href={venue.venue_slug ? `/venues/${venue.venue_slug}` : '#'}
                  className="text-foreground hover:text-primary transition-colors"
                >
                  {venue.venue_name}
                </Link>
                {venue.is_primary && (
                  <Badge
                    variant="secondary"
                    className="text-[10px] px-1.5 py-0"
                  >
                    Primary
                  </Badge>
                )}
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Links */}
      <div className="rounded-lg border border-border/50 bg-card p-4 space-y-3">
        <h3 className="text-sm font-semibold text-foreground">Links</h3>
        <div className="space-y-2 text-sm">
          {festival.website && (
            <a
              href={festival.website}
              target="_blank"
              rel="noopener noreferrer"
              className="flex items-center gap-2 text-primary hover:text-primary/80 transition-colors"
            >
              <Globe className="h-4 w-4 shrink-0" />
              <span>Website</span>
            </a>
          )}
          {festival.ticket_url && (
            <a
              href={festival.ticket_url}
              target="_blank"
              rel="noopener noreferrer"
              className="flex items-center gap-2 text-primary hover:text-primary/80 transition-colors"
            >
              <Ticket className="h-4 w-4 shrink-0" />
              <span>Tickets</span>
            </a>
          )}
          {festival.social && <SocialLinks social={festival.social} />}
        </div>
      </div>
    </div>
  )

  return (
    <EntityDetailLayout
      backLink={{ href: '/festivals', label: 'Back to Festivals' }}
      header={
        <EntityHeader
          title={festival.name}
          subtitle={
            <>
              <Badge variant={getFestivalStatusVariant(festival.status)}>
                {getFestivalStatusLabel(festival.status)}
              </Badge>
              <span className="flex items-center gap-1">
                <Calendar className="h-3.5 w-3.5" />
                {dateRange}
              </span>
              {location && (
                <span className="flex items-center gap-1">
                  <MapPin className="h-3.5 w-3.5" />
                  {location}
                </span>
              )}
            </>
          }
          actions={<FollowButton entityType="festivals" entityId={festival.id} />}
        />
      }
      tabs={tabs}
      activeTab={activeTab}
      onTabChange={setActiveTab}
      sidebar={sidebar}
    >
      {/* Lineup Tab */}
      <TabsContent value="lineup">
        {artistsLoading ? (
          <div className="flex justify-center py-8">
            <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
          </div>
        ) : (
          <FestivalLineup
            artists={artists}
            multiDay={hasMultipleDays}
          />
        )}
      </TabsContent>

      {/* Info Tab */}
      <TabsContent value="info">
        <div className="space-y-8">
          {/* Description */}
          {festival.description && (
            <div>
              <h2 className="text-lg font-semibold mb-3">About</h2>
              <p className="text-muted-foreground leading-relaxed whitespace-pre-line">
                {festival.description}
              </p>
            </div>
          )}

          {/* Venues Section */}
          {venuesLoading ? (
            <div className="flex justify-center py-8">
              <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
            </div>
          ) : venues.length > 0 ? (
            <div>
              <h2 className="text-lg font-semibold mb-4">Venues</h2>
              <div className="space-y-2">
                {venues.map(venue => (
                  <div
                    key={venue.id}
                    className="flex items-center gap-3 rounded-lg border border-border/50 bg-card p-3"
                  >
                    <Building2 className="h-4 w-4 text-muted-foreground shrink-0" />
                    <div className="flex-1 min-w-0">
                      <Link
                        href={
                          venue.venue_slug
                            ? `/venues/${venue.venue_slug}`
                            : '#'
                        }
                        className="font-medium text-foreground hover:text-primary transition-colors"
                      >
                        {venue.venue_name}
                      </Link>
                      <p className="text-xs text-muted-foreground">
                        {venue.city}, {venue.state}
                      </p>
                    </div>
                    {venue.is_primary && (
                      <Badge
                        variant="secondary"
                        className="text-[10px] px-1.5 py-0"
                      >
                        Primary
                      </Badge>
                    )}
                  </div>
                ))}
              </div>
            </div>
          ) : null}

          {/* Social / External Links */}
          {(festival.website || festival.ticket_url || festival.social) && (
            <div>
              <h2 className="text-lg font-semibold mb-3">Links</h2>
              <div className="space-y-2">
                {festival.website && (
                  <a
                    href={festival.website}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="flex items-center gap-2 text-primary hover:text-primary/80 transition-colors text-sm"
                  >
                    <Globe className="h-4 w-4" />
                    Official Website
                  </a>
                )}
                {festival.ticket_url && (
                  <a
                    href={festival.ticket_url}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="flex items-center gap-2 text-primary hover:text-primary/80 transition-colors text-sm"
                  >
                    <Ticket className="h-4 w-4" />
                    Buy Tickets
                  </a>
                )}
                {festival.social && <SocialLinks social={festival.social} />}
              </div>
            </div>
          )}

          {/* Fallback if no info */}
          {!festival.description &&
            venues.length === 0 &&
            !festival.website &&
            !festival.ticket_url &&
            !festival.social && (
              <div className="text-sm text-muted-foreground">
                No additional information available for this festival.
              </div>
            )}
        </div>
      </TabsContent>
    </EntityDetailLayout>
  )
}
