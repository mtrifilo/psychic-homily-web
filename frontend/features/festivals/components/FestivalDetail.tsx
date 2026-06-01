'use client'

import { useState, useMemo } from 'react'
import Link from 'next/link'
import {
  Loader2,
  Calendar,
  MapPin,
  Globe,
  Ticket,
  Building2,
} from 'lucide-react'
import {
  useFestival,
  useFestivalArtists,
  useFestivalVenues,
  useFestivals,
} from '../hooks/useFestivals'
import {
  EntityDetailLayout,
  EntityHeader,
  SocialLinks,
  FollowButton,
  AddToCollectionButton,
  RevisionHistory,
  BracketLink,
  SectionHeader,
  StatsList,
} from '@/components/shared'
import { EntityCollections } from '@/features/collections'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { FestivalLineup } from './FestivalLineup'
import { SimilarFestivals } from './SimilarFestivals'
import { RisingArtists } from './RisingArtists'
import { SeriesHistory } from './SeriesHistory'
import {
  getFestivalStatusLabel,
  getFestivalStatusVariant,
  formatFestivalLocation,
  formatFestivalDateRange,
} from '../types'
import { useIsAuthenticated } from '@/features/auth'
import { EntityEditDrawer, EntitySaveSuccessBanner, useEntitySaveSuccessBanner, AttributionLine, ReportEntityDialog, ContributionPrompt } from '@/features/contributions'
import { CommentThread } from '@/features/comments'
import { EntityTagList, AddTagDialog } from '@/features/tags'
import { useQueryClient } from '@tanstack/react-query'

interface FestivalDetailProps {
  idOrSlug: string | number
}

export function FestivalDetail({ idOrSlug }: FestivalDetailProps) {
  const { data: festival, isLoading, error } = useFestival({ idOrSlug })
  const { user, isAuthenticated } = useIsAuthenticated()
  const queryClient = useQueryClient()
  const canEditDirectly = isAuthenticated && (
    user?.is_admin ||
    user?.user_tier === 'trusted_contributor' ||
    user?.user_tier === 'local_ambassador'
  )
  const [isEditing, setIsEditing] = useState(false)
  const [editFocusField, setEditFocusField] = useState<string | undefined>()
  const [isReportOpen, setIsReportOpen] = useState(false)
  const [addTagDialogOpen, setAddTagDialogOpen] = useState(false)
  const saveBanner = useEntitySaveSuccessBanner()
  const { data: artistsData, isLoading: artistsLoading } = useFestivalArtists({
    festivalIdOrSlug: idOrSlug,
    enabled: !!festival,
  })
  const { data: venuesData, isLoading: venuesLoading } = useFestivalVenues({
    festivalIdOrSlug: idOrSlug,
    enabled: !!festival,
  })
  const { data: seriesData } = useFestivals({
    seriesSlug: festival?.series_slug,
  })
  // Narrow `festival?.series_slug` to a local so the memo body and its dep
  // array reference the same value. Reading `festival.series_slug` inside the
  // body while depending on `festival?.series_slug` made the React Compiler
  // infer the coarser `festival` dependency and refuse to preserve this memo
  // (react-hooks/preserve-manual-memoization). The result is unchanged: the
  // memo still recomputes only when the series list or the slug changes.
  const seriesSlug = festival?.series_slug
  const seriesEditions = useMemo(() => {
    if (!seriesData?.festivals || !seriesSlug) return []
    return seriesData.festivals
      .filter((f) => f.series_slug === seriesSlug)
      .map((f) => ({ year: f.edition_year }))
  }, [seriesData, seriesSlug])

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
  const hasDescription = !!festival.description && festival.description.trim().length > 0
  const hasSocialLinks =
    !!festival.social && Object.values(festival.social).some(v => !!v)
  const hasLinks =
    !!festival.website || !!festival.ticket_url || hasSocialLinks

  const statsItems = [
    { label: 'Artists', value: festival.artist_count },
    { label: 'Venues', value: festival.venue_count },
  ]

  const sidebar = (
    <div className="space-y-6">
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

      <section>
        <SectionHeader title="Statistics" />
        <StatsList items={statsItems} />
      </section>

      <EntityCollections entityType="festival" entityId={festival.id} />
    </div>
  )

  return (
  <>
    <EntityDetailLayout
      fallback={{ href: '/festivals', label: 'Festivals' }}
      entityName={festival.name}
      header={
        <>
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
            actions={
              <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
                <FollowButton
                  entityType="festivals"
                  entityId={festival.id}
                  variant="bracket"
                />
                <AddToCollectionButton
                  entityType="festival"
                  entityId={festival.id}
                  entityName={festival.name}
                  variant="bracket"
                />
                {isAuthenticated && (
                  <BracketLink
                    label={canEditDirectly ? 'Edit' : 'Suggest edit'}
                    onClick={() => setIsEditing(true)}
                  />
                )}
                {isAuthenticated && !hasDescription && (
                  <BracketLink
                    label="Suggest description"
                    onClick={() => {
                      setEditFocusField('description')
                      setIsEditing(true)
                    }}
                  />
                )}
                {isAuthenticated && (
                  <BracketLink
                    label="Add tag"
                    onClick={() => setAddTagDialogOpen(true)}
                  />
                )}
                {isAuthenticated && (
                  <BracketLink
                    label="Report"
                    title="Report an issue"
                    onClick={() => setIsReportOpen(true)}
                  />
                )}
              </div>
            }
          />
          <EntitySaveSuccessBanner visible={saveBanner.isVisible} />
          <AttributionLine entityType="festival" entityId={festival.id} />
          <EntityTagList
            entityType="festival"
            entityId={festival.id}
            isAuthenticated={isAuthenticated}
          />
          <ContributionPrompt
            entityType="festival"
            entityId={festival.id}
            entitySlug={festival.slug}
            isAuthenticated={!!isAuthenticated}
            onEditClick={(focusField) => {
              setEditFocusField(focusField)
              setIsEditing(true)
            }}
          />
        </>
      }
      sidebar={sidebar}
    >
      <div className="space-y-8">
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

        <SeriesHistory
          seriesSlug={festival.series_slug}
          editions={seriesEditions}
        />

        <RisingArtists festivalIdOrSlug={idOrSlug} />

        <SimilarFestivals festivalIdOrSlug={idOrSlug} />

        {hasDescription && (
          <div>
            <h2 className="text-lg font-semibold mb-3">About</h2>
            <p className="text-muted-foreground leading-relaxed whitespace-pre-line">
              {festival.description}
            </p>
          </div>
        )}

        {venuesLoading ? (
          <div className="flex justify-center py-8">
            <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
          </div>
        ) : venues.length > 0 ? (
          <div>
            <h2 className="text-lg font-semibold mb-3">Venues</h2>
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

        {hasLinks && (
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
              {hasSocialLinks && <SocialLinks social={festival.social!} />}
            </div>
          </div>
        )}
      </div>
    </EntityDetailLayout>

    <div className="mt-0">
      <RevisionHistory
        entityType="festival"
        entityId={festival.id}
        isAdmin={!!user?.is_admin}
      />
    </div>

    <div className="mt-0 px-4 md:px-0">
      <CommentThread entityType="festival" entityId={festival.id} />
    </div>

    {festival && isAuthenticated && (
      <EntityEditDrawer
        open={isEditing}
        onOpenChange={(open) => {
          setIsEditing(open)
          if (!open) setEditFocusField(undefined)
        }}
        entityType="festival"
        entityId={festival.id}
        entityName={festival.name}
        entity={festival as unknown as Record<string, unknown>}
        canEditDirectly={!!canEditDirectly}
        focusField={editFocusField}
        onSuccess={(result) => {
          queryClient.invalidateQueries({
            queryKey: ['festivals', 'detail'],
          })
          saveBanner.handleSaveSuccess(result)
        }}
      />
    )}

    {festival && isAuthenticated && (
      <ReportEntityDialog
        open={isReportOpen}
        onOpenChange={setIsReportOpen}
        entityType="festival"
        entityId={festival.id}
        entityName={festival.name}
      />
    )}

    {isAuthenticated && (
      <AddTagDialog
        entityType="festival"
        entityId={festival.id}
        open={addTagDialogOpen}
        onOpenChange={setAddTagDialogOpen}
      />
    )}
  </>
  )
}
