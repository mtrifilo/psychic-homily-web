'use client'

import { useState } from 'react'
import Link from 'next/link'
import {
  Loader2,
  Disc3,
  ExternalLink,
  Music,
  Calendar,
  Users,
} from 'lucide-react'
import { useRelease } from '@/lib/hooks/useReleases'
import { EntityDetailLayout, EntityHeader } from '@/components/shared'
import { TabsContent } from '@/components/ui/tabs'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { getReleaseTypeLabel } from '@/lib/types/release'

/** Known platform display info */
const PLATFORM_CONFIG: Record<
  string,
  { label: string; className?: string }
> = {
  bandcamp: { label: 'Bandcamp' },
  spotify: { label: 'Spotify' },
  apple_music: { label: 'Apple Music' },
  youtube: { label: 'YouTube' },
  youtube_music: { label: 'YouTube Music' },
  soundcloud: { label: 'SoundCloud' },
  tidal: { label: 'Tidal' },
  deezer: { label: 'Deezer' },
  amazon_music: { label: 'Amazon Music' },
  discogs: { label: 'Discogs' },
}

function getPlatformLabel(platform: string): string {
  return (
    PLATFORM_CONFIG[platform]?.label ??
    platform
      .split('_')
      .map(w => w.charAt(0).toUpperCase() + w.slice(1))
      .join(' ')
  )
}

interface ReleaseDetailProps {
  idOrSlug: string | number
}

export function ReleaseDetail({ idOrSlug }: ReleaseDetailProps) {
  const { data: release, isLoading, error } = useRelease({ idOrSlug })
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
      error instanceof Error ? error.message : 'Failed to load release'
    const is404 =
      errorMessage.includes('not found') || errorMessage.includes('404')

    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <div className="text-center">
          <h1 className="text-2xl font-bold mb-2">
            {is404 ? 'Release Not Found' : 'Error Loading Release'}
          </h1>
          <p className="text-muted-foreground mb-4">
            {is404
              ? "The release you're looking for doesn't exist or has been removed."
              : errorMessage}
          </p>
          <Button asChild variant="outline">
            <Link href="/releases">Back to Releases</Link>
          </Button>
        </div>
      </div>
    )
  }

  if (!release) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <div className="text-center">
          <h1 className="text-2xl font-bold mb-2">Release Not Found</h1>
          <p className="text-muted-foreground mb-4">
            The release you&apos;re looking for doesn&apos;t exist.
          </p>
          <Button asChild variant="outline">
            <Link href="/releases">Back to Releases</Link>
          </Button>
        </div>
      </div>
    )
  }

  const hasExternalLinks =
    release.external_links && release.external_links.length > 0

  const tabs = [
    { value: 'overview', label: 'Overview' },
    ...(hasExternalLinks
      ? [{ value: 'listen', label: 'Listen / Buy' }]
      : []),
  ]

  const sidebar = (
    <div className="space-y-6">
      {/* Cover Art */}
      <div className="rounded-lg border border-border/50 bg-card overflow-hidden">
        {release.cover_art_url ? (
          <img
            src={release.cover_art_url}
            alt={`${release.title} cover art`}
            className="w-full aspect-square object-cover"
          />
        ) : (
          <div className="w-full aspect-square bg-muted/30 flex items-center justify-center">
            <Disc3 className="h-16 w-16 text-muted-foreground/30" />
          </div>
        )}
      </div>

      {/* Quick Info */}
      <div className="rounded-lg border border-border/50 bg-card p-4 space-y-3">
        <h3 className="text-sm font-semibold text-foreground">Details</h3>

        <div className="space-y-2 text-sm">
          <div className="flex items-center gap-2 text-muted-foreground">
            <Music className="h-4 w-4 shrink-0" />
            <span>Type: {getReleaseTypeLabel(release.release_type)}</span>
          </div>

          {release.release_year && (
            <div className="flex items-center gap-2 text-muted-foreground">
              <Calendar className="h-4 w-4 shrink-0" />
              <span>Year: {release.release_year}</span>
            </div>
          )}

          {release.release_date && (
            <div className="flex items-center gap-2 text-muted-foreground">
              <Calendar className="h-4 w-4 shrink-0" />
              <span>
                Released:{' '}
                {new Date(release.release_date).toLocaleDateString('en-US', {
                  year: 'numeric',
                  month: 'long',
                  day: 'numeric',
                })}
              </span>
            </div>
          )}

          {release.artists && release.artists.length > 0 && (
            <div className="flex items-start gap-2 text-muted-foreground">
              <Users className="h-4 w-4 shrink-0 mt-0.5" />
              <div>
                <span>
                  {release.artists.length === 1
                    ? '1 artist'
                    : `${release.artists.length} artists`}
                </span>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  )

  return (
    <EntityDetailLayout
      backLink={{ href: '/releases', label: 'Back to Releases' }}
      header={
        <EntityHeader
          title={release.title}
          subtitle={
            <>
              <Badge variant="secondary">
                {getReleaseTypeLabel(release.release_type)}
              </Badge>
              {release.release_year && <span>{release.release_year}</span>}
            </>
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
          {release.description && (
            <div>
              <h2 className="text-lg font-semibold mb-3">About</h2>
              <p className="text-muted-foreground leading-relaxed whitespace-pre-line">
                {release.description}
              </p>
            </div>
          )}

          {/* Artists */}
          {release.artists && release.artists.length > 0 && (
            <div>
              <h2 className="text-lg font-semibold mb-3">Artists</h2>
              <div className="space-y-2">
                {release.artists.map(artist => (
                  <div
                    key={artist.id}
                    className="flex items-center justify-between rounded-lg border border-border/50 bg-card p-3"
                  >
                    <Link
                      href={`/artists/${artist.slug}`}
                      className="font-medium text-foreground hover:text-primary transition-colors"
                    >
                      {artist.name}
                    </Link>
                    {artist.role && (
                      <span className="text-sm text-muted-foreground capitalize">
                        {artist.role}
                      </span>
                    )}
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* Show external links in overview if there's no separate tab */}
          {!hasExternalLinks && (
            <div className="text-sm text-muted-foreground">
              No external links available for this release.
            </div>
          )}
        </div>
      </TabsContent>

      {/* Listen / Buy Tab */}
      {hasExternalLinks && (
        <TabsContent value="listen">
          <div>
            <h2 className="text-lg font-semibold mb-4">Listen / Buy</h2>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
              {release.external_links.map(link => (
                <a
                  key={link.id}
                  href={link.url}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="flex items-center gap-3 rounded-lg border border-border/50 bg-card p-4 transition-colors hover:bg-muted/50"
                >
                  <ExternalLink className="h-5 w-5 text-muted-foreground shrink-0" />
                  <div className="flex-1 min-w-0">
                    <div className="font-medium text-foreground">
                      {getPlatformLabel(link.platform)}
                    </div>
                    <div className="text-xs text-muted-foreground truncate">
                      {link.url}
                    </div>
                  </div>
                </a>
              ))}
            </div>
          </div>
        </TabsContent>
      )}
    </EntityDetailLayout>
  )
}
