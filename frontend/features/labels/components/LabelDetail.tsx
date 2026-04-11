'use client'

import { useState } from 'react'
import Link from 'next/link'
import {
  Loader2,
  Tag,
  MapPin,
  Calendar,
  Users,
  Disc3,
  Music,
} from 'lucide-react'
import { useLabel, useLabelRoster, useLabelCatalog } from '../hooks/useLabels'
import { EntityDetailLayout, EntityHeader, SocialLinks, FollowButton, AddToCollectionButton } from '@/components/shared'
import { EntityCollections } from '@/features/collections'
import { CommentThread } from '@/features/comments'
import { NotifyMeButton } from '@/features/notifications'
import { TabsContent } from '@/components/ui/tabs'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  getLabelStatusLabel,
  getLabelStatusVariant,
  formatLabelLocation,
} from '../types'
import { getReleaseTypeLabel } from '@/features/releases/types'

interface LabelDetailProps {
  idOrSlug: string | number
}

export function LabelDetail({ idOrSlug }: LabelDetailProps) {
  const { data: label, isLoading, error } = useLabel({ idOrSlug })
  const { data: rosterData, isLoading: rosterLoading } = useLabelRoster({
    labelIdOrSlug: idOrSlug,
    enabled: !!label,
  })
  const { data: catalogData, isLoading: catalogLoading } = useLabelCatalog({
    labelIdOrSlug: idOrSlug,
    enabled: !!label,
  })
  const [activeTab, setActiveTab] = useState('overview')

  if (isLoading) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (error) {
    const errorMessage =
      error instanceof Error ? error.message : 'Failed to load label'
    const is404 =
      errorMessage.includes('not found') || errorMessage.includes('404')

    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <div className="text-center">
          <h1 className="text-2xl font-bold mb-2">
            {is404 ? 'Label Not Found' : 'Error Loading Label'}
          </h1>
          <p className="text-muted-foreground mb-4">
            {is404
              ? "The label you're looking for doesn't exist or has been removed."
              : errorMessage}
          </p>
          <Button asChild variant="outline">
            <Link href="/labels">Back to Labels</Link>
          </Button>
        </div>
      </div>
    )
  }

  if (!label) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <div className="text-center">
          <h1 className="text-2xl font-bold mb-2">Label Not Found</h1>
          <p className="text-muted-foreground mb-4">
            The label you&apos;re looking for doesn&apos;t exist.
          </p>
          <Button asChild variant="outline">
            <Link href="/labels">Back to Labels</Link>
          </Button>
        </div>
      </div>
    )
  }

  const location = formatLabelLocation(label)
  const roster = rosterData?.artists ?? []
  const catalog = catalogData?.releases ?? []

  const tabs = [
    { value: 'overview', label: 'Overview' },
    { value: 'roster', label: `Roster (${label.artist_count})` },
    { value: 'catalog', label: `Catalog (${label.release_count})` },
  ]

  const sidebar = (
    <div className="space-y-6">
      {/* Label Icon */}
      <div className="rounded-lg border border-border/50 bg-card overflow-hidden">
        <div className="w-full aspect-square bg-muted/30 flex items-center justify-center">
          <Tag className="h-16 w-16 text-muted-foreground/30" />
        </div>
      </div>

      {/* Quick Info */}
      <div className="rounded-lg border border-border/50 bg-card p-4 space-y-3">
        <h3 className="text-sm font-semibold text-foreground">Details</h3>

        <div className="space-y-2 text-sm">
          <div className="flex items-center gap-2 text-muted-foreground">
            <Tag className="h-4 w-4 shrink-0" />
            <span>
              Status:{' '}
              <Badge
                variant={getLabelStatusVariant(label.status)}
                className="text-[10px] px-1.5 py-0 ml-1"
              >
                {getLabelStatusLabel(label.status)}
              </Badge>
            </span>
          </div>

          {location && (
            <div className="flex items-center gap-2 text-muted-foreground">
              <MapPin className="h-4 w-4 shrink-0" />
              <span>{location}</span>
            </div>
          )}

          {label.founded_year && (
            <div className="flex items-center gap-2 text-muted-foreground">
              <Calendar className="h-4 w-4 shrink-0" />
              <span>Founded: {label.founded_year}</span>
            </div>
          )}

          <div className="flex items-center gap-2 text-muted-foreground">
            <Users className="h-4 w-4 shrink-0" />
            <span>
              {label.artist_count === 1
                ? '1 artist'
                : `${label.artist_count} artists`}
            </span>
          </div>

          <div className="flex items-center gap-2 text-muted-foreground">
            <Disc3 className="h-4 w-4 shrink-0" />
            <span>
              {label.release_count === 1
                ? '1 release'
                : `${label.release_count} releases`}
            </span>
          </div>
        </div>
      </div>

      {/* In Collections */}
      <EntityCollections entityType="label" entityId={label.id} />
    </div>
  )

  return (
    <>
    <EntityDetailLayout
      fallback={{ href: '/labels', label: 'Labels' }}
      entityName={label.name}
      header={
        <EntityHeader
          title={label.name}
          subtitle={
            <>
              <Badge variant={getLabelStatusVariant(label.status)}>
                {getLabelStatusLabel(label.status)}
              </Badge>
              {location && (
                <span className="flex items-center gap-1">
                  <MapPin className="h-3.5 w-3.5" />
                  {location}
                </span>
              )}
              {label.founded_year && <span>Est. {label.founded_year}</span>}
            </>
          }
          actions={
            <div className="flex items-center gap-2">
              <NotifyMeButton entityType="label" entityId={label.id} entityName={label.name} />
              <FollowButton entityType="labels" entityId={label.id} />
              <AddToCollectionButton entityType="label" entityId={label.id} entityName={label.name} />
            </div>
          }
        />
      }
      tabs={tabs}
      activeTab={activeTab}
      onTabChange={setActiveTab}
      sidebar={sidebar}
    >
      {/* Overview Tab */}
      <TabsContent value="overview">
        <div className="space-y-8">
          {/* Description */}
          {label.description && (
            <div>
              <h2 className="text-lg font-semibold mb-3">About</h2>
              <p className="text-muted-foreground leading-relaxed whitespace-pre-line">
                {label.description}
              </p>
            </div>
          )}

          {/* Social Links */}
          {label.social && (
            <div>
              <h2 className="text-lg font-semibold mb-3">Links</h2>
              <SocialLinks social={label.social} />
            </div>
          )}

          {/* Quick preview of roster + catalog when no description */}
          {!label.description && !label.social && (
            <div className="text-sm text-muted-foreground">
              No additional information available for this label.
            </div>
          )}
        </div>
      </TabsContent>

      {/* Roster Tab */}
      <TabsContent value="roster">
        <div>
          <h2 className="text-lg font-semibold mb-4">Artist Roster</h2>
          {rosterLoading ? (
            <div className="flex justify-center py-8">
              <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
            </div>
          ) : roster.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              No artists are currently associated with this label.
            </p>
          ) : (
            <div className="space-y-2">
              {roster.map(artist => (
                <div
                  key={artist.id}
                  className="flex items-center rounded-lg border border-border/50 bg-card p-3"
                >
                  <Users className="h-4 w-4 text-muted-foreground mr-3 shrink-0" />
                  <Link
                    href={`/artists/${artist.slug}`}
                    className="font-medium text-foreground hover:text-primary transition-colors"
                  >
                    {artist.name}
                  </Link>
                </div>
              ))}
            </div>
          )}
        </div>
      </TabsContent>

      {/* Catalog Tab */}
      <TabsContent value="catalog">
        <div>
          <h2 className="text-lg font-semibold mb-4">Release Catalog</h2>
          {catalogLoading ? (
            <div className="flex justify-center py-8">
              <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
            </div>
          ) : catalog.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              No releases are currently in this label&apos;s catalog.
            </p>
          ) : (
            <div className="space-y-2">
              {catalog.map(release => (
                <div
                  key={release.id}
                  className="flex items-center gap-3 rounded-lg border border-border/50 bg-card p-3"
                >
                  {/* Cover art or placeholder */}
                  <div className="h-10 w-10 shrink-0 rounded-md bg-muted/50 flex items-center justify-center overflow-hidden">
                    {release.cover_art_url ? (
                      <img
                        src={release.cover_art_url}
                        alt={`${release.title} cover art`}
                        className="h-full w-full object-cover"
                      />
                    ) : (
                      <Music className="h-5 w-5 text-muted-foreground/40" />
                    )}
                  </div>

                  <div className="flex-1 min-w-0">
                    <Link
                      href={`/releases/${release.slug}`}
                      className="font-medium text-foreground hover:text-primary transition-colors truncate block"
                    >
                      {release.title}
                    </Link>
                    <div className="flex items-center gap-2 text-xs text-muted-foreground mt-0.5">
                      <Badge
                        variant="secondary"
                        className="text-[10px] px-1.5 py-0"
                      >
                        {getReleaseTypeLabel(release.release_type)}
                      </Badge>
                      {release.release_year && (
                        <span>{release.release_year}</span>
                      )}
                      {release.catalog_number && (
                        <span className="text-muted-foreground/60">
                          {release.catalog_number}
                        </span>
                      )}
                    </div>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </TabsContent>
    </EntityDetailLayout>

    {/* Discussion */}
    <div className="mt-0 px-4 md:px-0">
      <CommentThread entityType="label" entityId={label.id} />
    </div>
  </>
  )
}
