'use client'

import { useState } from 'react'
import Link from 'next/link'
import { useQueryClient } from '@tanstack/react-query'
import {
  Loader2,
  Disc3,
  ExternalLink,
  Music,
  Calendar,
  Users,
  Tag,
} from 'lucide-react'
import { useRelease } from '../hooks/useReleases'
import { useIsAuthenticated } from '@/features/auth'
import {
  EntityDetailLayout,
  EntityDetailContainer,
  EntityHeader,
  RevisionHistory,
  AddToCollectionButton,
  ReleaseSaveButton,
  BracketLink,
  ImageAttribution,
  MusicEmbed,
} from '@/components/shared'
import {
  AttributionLine,
  EntityEditDrawer,
  EntitySaveSuccessBanner,
  ReportEntityDialog,
  useEntitySaveSuccessBanner,
} from '@/features/contributions'
import { EntityTagList, AddTagDialog } from '@/features/tags'
import { AddReleaseLinkDialog } from './AddReleaseLinkDialog'
import { AsHeardOn } from '@/features/radio'
import { EntityCollections } from '@/features/collections'
import { EntityChartRankBadge } from '@/features/charts'
import { CommentThread } from '@/features/comments'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { queryKeys } from '@/lib/queryClient'
import { getReleaseTypeLabel, type ReleaseExternalLink } from '../types'

/** Known platform display info */
const PLATFORM_CONFIG: Record<string, { label: string; className?: string }> = {
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

/**
 * Pick the first Bandcamp album/track external link to feed the embed (PSY-1187).
 *
 * Only `/album/<slug>` and `/track/<slug>` URLs resolve to a playable Bandcamp
 * iframe; a bare profile root (e.g. `https://artist.bandcamp.com`) does not.
 * Restricting to those two path types here means MusicEmbed only fires the
 * resolver fetch when a player can actually render — a profile-only Bandcamp
 * link is left to the plain "Listen / Buy" card alone (and MusicEmbed itself
 * still falls back to a link if a chosen album/track URL fails to resolve).
 */
function findBandcampEmbedUrl(links: ReleaseExternalLink[]): string | null {
  const link = links.find(
    l =>
      l.platform.toLowerCase() === 'bandcamp' &&
      (l.url.includes('/album/') || l.url.includes('/track/'))
  )
  return link?.url ?? null
}

/**
 * Pick the first Spotify external link to feed the embed (PSY-1195).
 *
 * Returns the URL of any `spotify`-platform link (case-insensitive). MusicEmbed
 * runs `parseSpotifyEmbed` on it, which host-anchors + id-validates and accepts
 * only album/track/artist URLs — so a non-embeddable Spotify URL (e.g. a search
 * or playlist link) is left to the plain "Listen / Buy" card and MusicEmbed
 * renders no Spotify player for it. When a release also has a Bandcamp
 * album/track link, MusicEmbed's internal priority renders the Bandcamp embed
 * first, so passing both is safe (PSY-1187 precedence preserved).
 */
function findSpotifyEmbedUrl(links: ReleaseExternalLink[]): string | null {
  const link = links.find(l => l.platform.toLowerCase() === 'spotify')
  return link?.url ?? null
}

interface ReleaseDetailProps {
  idOrSlug: string | number
}

export function ReleaseDetail({ idOrSlug }: ReleaseDetailProps) {
  const { data: release, isLoading, error } = useRelease({ idOrSlug })
  const { user, isAuthenticated } = useIsAuthenticated()
  const queryClient = useQueryClient()
  const canEditDirectly =
    isAuthenticated &&
    (user?.is_admin ||
      user?.user_tier === 'trusted_contributor' ||
      user?.user_tier === 'local_ambassador')
  // PSY-660: adding an external link is gated to admin + trusted_contributor +
  // local_ambassador (the same tiers the backend authorizes on POST
  // /releases/{id}/links). Gating on the tier — not plain isAuthenticated —
  // keeps a new_user from seeing a [Add link] bracket the backend would 403.
  const canAddLink = !!canEditDirectly
  const [isEditing, setIsEditing] = useState(false)
  const [editFocusField, setEditFocusField] = useState<string | undefined>()
  const [addTagDialogOpen, setAddTagDialogOpen] = useState(false)
  const [addLinkDialogOpen, setAddLinkDialogOpen] = useState(false)
  const [isReportOpen, setIsReportOpen] = useState(false)
  const saveBanner = useEntitySaveSuccessBanner()

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
  const hasLabels = release.labels && release.labels.length > 0
  const hasDescription =
    !!release.description && release.description.trim().length > 0

  // PSY-1187: render a playable Bandcamp player when a release has a Bandcamp
  // album/track link. MusicEmbed resolves the URL to an iframe (falling back to
  // a link if it can't), so the clickable "Listen / Buy" cards below stay as-is.
  const bandcampEmbedUrl = release.external_links
    ? findBandcampEmbedUrl(release.external_links)
    : null
  // PSY-1195: also feed a Spotify link to MusicEmbed. Its internal priority
  // prefers Bandcamp, so a release with both still shows the Bandcamp embed;
  // a Spotify-only release gets a playable Spotify player instead of just a card.
  const spotifyEmbedUrl = release.external_links
    ? findSpotifyEmbedUrl(release.external_links)
    : null
  // Fallback link text uses the primary (main) artist's name, then the first
  // artist, then the release title.
  const primaryArtistName =
    release.artists?.find(a => a.role === 'main')?.name ??
    release.artists?.[0]?.name ??
    release.title

  const sidebar = (
    <div className="space-y-6">
      {/* Cover Art */}
      <div>
        <div className="rounded-lg border border-border/50 bg-card overflow-hidden">
          {release.cover_art_url ? (
            // Hotlinked provider art (PSY-1175 D2): raw <img> avoids next/image
            // per-host remotePatterns churn; CSP already allows img-src https:.
            // eslint-disable-next-line @next/next/no-img-element
            <img
              src={release.cover_art_url}
              alt={`${release.title} cover art`}
              loading="lazy"
              className="w-full aspect-square object-cover"
            />
          ) : (
            <div className="w-full aspect-square bg-muted/30 flex items-center justify-center">
              <Disc3 className="h-16 w-16 text-muted-foreground/30" />
            </div>
          )}
        </div>
        {release.cover_art_url && (
          <ImageAttribution
            source={release.cover_art_source}
            sourceUrl={release.cover_art_source_url}
            kind="cover"
            className="mt-1.5 px-0.5"
          />
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

          {release.release_year != null && (
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

      {/* Labels */}
      {hasLabels && (
        <div>
          <h3 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2">
            {release.labels.length === 1 ? 'Label' : 'Labels'}
          </h3>
          <div className="space-y-1">
            {release.labels.map(label => (
              <Link
                key={label.id}
                href={`/labels/${label.slug}`}
                className="flex items-center gap-2 text-sm text-muted-foreground hover:text-foreground transition-colors py-0.5"
              >
                <Tag className="h-3.5 w-3.5 shrink-0" />
                <span>{label.name}</span>
                {label.catalog_number && (
                  <span className="text-xs text-muted-foreground/60">
                    ({label.catalog_number})
                  </span>
                )}
              </Link>
            ))}
          </div>
        </div>
      )}

      {/* As Heard On (radio) */}
      <AsHeardOn entityType="release" entitySlug={release.slug} />

      {/* Chart rank — self-hides when unranked (PSY-1420) */}
      <EntityChartRankBadge entityType="release" entityId={release.id} />

      {/* In Collections */}
      <EntityCollections entityType="release" entityId={release.id} />
    </div>
  )

  return (
    <>
      <EntityDetailLayout
        fallback={{ href: '/releases', label: 'Releases' }}
        entityName={release.title}
        header={
          <>
            <EntityHeader
              title={release.title}
              subtitle={
                <>
                  <Badge variant="secondary">
                    {getReleaseTypeLabel(release.release_type)}
                  </Badge>
                  {release.release_year != null && (
                    <span>{release.release_year}</span>
                  )}
                </>
              }
              actions={
                <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
                  <ReleaseSaveButton releaseId={release.id} variant="bracket" />
                  <AddToCollectionButton
                    entityType="release"
                    entityId={release.id}
                    entityName={release.title}
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
                  {canAddLink && (
                    <BracketLink
                      label="Add link"
                      title="Add a Listen / Buy link"
                      onClick={() => setAddLinkDialogOpen(true)}
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
            <AttributionLine entityType="release" entityId={release.id} />
            <EntityTagList
              entityType="release"
              entityId={release.id}
              isAuthenticated={isAuthenticated}
            />
          </>
        }
        sidebar={sidebar}
      >
        <div className="space-y-8">
          {hasDescription && (
            <div>
              <h2 className="text-lg font-semibold mb-3">About</h2>
              <p className="text-muted-foreground leading-relaxed whitespace-pre-line">
                {release.description}
              </p>
            </div>
          )}

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

          {hasExternalLinks && (
            <div>
              <h2 className="text-lg font-semibold mb-3">Listen / Buy</h2>
              {/* Playable player above the link cards. PSY-1187 wired Bandcamp;
                  PSY-1195 adds Spotify. MusicEmbed prefers Bandcamp when both
                  are present, falls back to a link if a chosen URL can't resolve,
                  and renders nothing if neither URL is embeddable — so the link
                  cards below always remain. */}
              {(bandcampEmbedUrl || spotifyEmbedUrl) && (
                <div className="mb-4">
                  <MusicEmbed
                    bandcampAlbumUrl={bandcampEmbedUrl}
                    spotifyUrl={spotifyEmbedUrl}
                    artistName={primaryArtistName}
                    compact
                  />
                </div>
              )}
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
          )}
        </div>
      </EntityDetailLayout>

      {/* History + Discussion — shared container matches EntityDetailLayout's
          gutter + max-width so they align with the rest of the page (PSY-1026). */}
      <EntityDetailContainer>
        <RevisionHistory entityType="release" entityId={release.id} />
        <CommentThread entityType="release" entityId={release.id} />
      </EntityDetailContainer>

      {/* Edit Drawer (all authenticated users) */}
      {isAuthenticated && (
        <EntityEditDrawer
          open={isEditing}
          onOpenChange={open => {
            setIsEditing(open)
            if (!open) setEditFocusField(undefined)
          }}
          entityType="release"
          entityId={release.id}
          entityName={release.title}
          entity={release as unknown as Record<string, unknown>}
          canEditDirectly={!!canEditDirectly}
          focusField={editFocusField}
          onSuccess={result => {
            queryClient.invalidateQueries({
              queryKey: queryKeys.releases.detail(idOrSlug),
            })
            saveBanner.handleSaveSuccess(result)
          }}
        />
      )}

      {isAuthenticated && (
        <AddTagDialog
          entityType="release"
          entityId={release.id}
          open={addTagDialogOpen}
          onOpenChange={setAddTagDialogOpen}
        />
      )}

      {/* Add link dialog — PSY-660. Gated to admin/trusted/local_ambassador;
          backend enforces the same tier on POST /releases/{id}/links. */}
      {canAddLink && (
        <AddReleaseLinkDialog
          releaseId={release.id}
          releaseTitle={release.title}
          open={addLinkDialogOpen}
          onOpenChange={setAddLinkDialogOpen}
        />
      )}

      {/* Report Dialog (authenticated users) — PSY-661 */}
      {isAuthenticated && (
        <ReportEntityDialog
          open={isReportOpen}
          onOpenChange={setIsReportOpen}
          entityType="release"
          entityId={release.id}
          entityName={release.title}
        />
      )}
    </>
  )
}
