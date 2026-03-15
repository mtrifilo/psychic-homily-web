'use client'

import Link from 'next/link'
import { useState } from 'react'
import {
  ArrowLeft,
  Loader2,
  MapPin,
  Sparkles,
  Pencil,
  X,
  Check,
  AlertCircle,
  Edit2,
  Disc3,
  Tag,
} from 'lucide-react'
import { useQueryClient } from '@tanstack/react-query'
import { useArtist } from '../hooks/useArtists'
import { useArtistReleases } from '@/features/releases/hooks/useReleases'
import { useArtistAliases } from '@/lib/hooks/admin/useAdminArtists'
import { useArtistLabels, useLabelRoster } from '@/features/labels/hooks/useLabels'
import { queryKeys } from '@/lib/queryClient'
import { useIsAuthenticated } from '@/features/auth'
import {
  useDiscoverMusic,
  useUpdateArtistBandcamp,
  useClearArtistBandcamp,
  useUpdateArtistSpotify,
  useClearArtistSpotify,
  type MusicPlatform,
} from '@/lib/hooks/admin/useAdminArtists'
import { SocialLinks, MusicEmbed, EntityDetailLayout, EntityHeader, RevisionHistory, FollowButton } from '@/components/shared'
import { EntityTagList } from '@/features/tags'
import { ArtistEditForm } from '@/components/forms/ArtistEditForm'
import { ArtistShowsList } from './ArtistShowsList'
import { ReportArtistButton } from './ReportArtistButton'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { TabsContent } from '@/components/ui/tabs'
import { Badge } from '@/components/ui/badge'
import { getReleaseTypeLabel } from '@/features/releases/types'
import type { ArtistReleaseListItem } from '@/features/releases/types'
import type { ArtistLabel } from '@/features/labels/types'

interface ArtistDetailProps {
  artistId: string | number
}

// --- Discography Tab ---

interface DiscographySectionProps {
  title: string
  releases: ArtistReleaseListItem[]
}

function DiscographySection({ title, releases }: DiscographySectionProps) {
  if (releases.length === 0) return null

  return (
    <div className="mb-6">
      <h3 className="text-sm font-semibold text-muted-foreground uppercase tracking-wider mb-3">
        {title}
      </h3>
      <div className="space-y-2">
        {releases.map(release => (
          <Link
            key={release.id}
            href={`/releases/${release.slug}`}
            className="flex items-center gap-3 p-2 rounded-md hover:bg-muted/50 transition-colors group"
          >
            <div className="w-10 h-10 bg-muted rounded flex-shrink-0 flex items-center justify-center">
              {release.cover_art_url ? (
                <img
                  src={release.cover_art_url}
                  alt={release.title}
                  className="w-10 h-10 rounded object-cover"
                />
              ) : (
                <Disc3 className="h-5 w-5 text-muted-foreground" />
              )}
            </div>
            <div className="flex-1 min-w-0">
              <p className="text-sm font-medium group-hover:text-foreground truncate">
                {release.title}
              </p>
              <div className="flex items-center gap-2 text-xs text-muted-foreground">
                <Badge variant="secondary" className="text-[10px] px-1.5 py-0">
                  {getReleaseTypeLabel(release.release_type)}
                </Badge>
                {release.release_year && <span>{release.release_year}</span>}
              </div>
            </div>
          </Link>
        ))}
      </div>
    </div>
  )
}

function DiscographyTab({ artistIdOrSlug }: { artistIdOrSlug: string | number }) {
  const { data, isLoading, error } = useArtistReleases({ artistIdOrSlug })

  if (isLoading) {
    return (
      <div className="flex justify-center py-8">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (error) {
    return (
      <div className="py-8 text-center text-sm text-destructive">
        Failed to load discography
      </div>
    )
  }

  if (!data?.releases || data.releases.length === 0) {
    return (
      <div className="py-8 text-center text-sm text-muted-foreground">
        No releases yet
      </div>
    )
  }

  // Group releases by role category
  const albumsAndEPs = data.releases.filter(
    r => r.role === 'main' && (r.release_type === 'lp' || r.release_type === 'ep')
  )
  const singles = data.releases.filter(
    r => r.role === 'main' && r.release_type === 'single'
  )
  const otherMain = data.releases.filter(
    r =>
      r.role === 'main' &&
      r.release_type !== 'lp' &&
      r.release_type !== 'ep' &&
      r.release_type !== 'single'
  )
  const appearsOn = data.releases.filter(r => r.role === 'featured')
  const production = data.releases.filter(
    r =>
      r.role === 'producer' ||
      r.role === 'remixer' ||
      r.role === 'composer' ||
      r.role === 'dj'
  )

  return (
    <div>
      <DiscographySection title="Albums & EPs" releases={albumsAndEPs} />
      <DiscographySection title="Singles" releases={singles} />
      <DiscographySection title="Other Releases" releases={otherMain} />
      <DiscographySection title="Appears On" releases={appearsOn} />
      <DiscographySection title="Production" releases={production} />
    </div>
  )
}

// --- Labels Tab ---

function LabelsTab({ artistIdOrSlug }: { artistIdOrSlug: string | number }) {
  const { data, isLoading, error } = useArtistLabels({ artistIdOrSlug })

  if (isLoading) {
    return (
      <div className="flex justify-center py-8">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (error) {
    return (
      <div className="py-8 text-center text-sm text-destructive">
        Failed to load labels
      </div>
    )
  }

  if (!data?.labels || data.labels.length === 0) {
    return (
      <div className="py-8 text-center text-sm text-muted-foreground">
        No label affiliations yet
      </div>
    )
  }

  return (
    <div className="space-y-3">
      {data.labels.map(label => (
        <Link
          key={label.id}
          href={`/labels/${label.slug}`}
          className="flex items-center gap-3 p-3 rounded-md border border-border/50 hover:bg-muted/50 transition-colors group"
        >
          <div className="w-10 h-10 bg-muted rounded flex-shrink-0 flex items-center justify-center">
            <Tag className="h-5 w-5 text-muted-foreground" />
          </div>
          <div className="flex-1 min-w-0">
            <p className="text-sm font-medium group-hover:text-foreground">
              {label.name}
            </p>
            {(label.city || label.state) && (
              <p className="text-xs text-muted-foreground">
                {[label.city, label.state].filter(Boolean).join(', ')}
              </p>
            )}
          </div>
        </Link>
      ))}
    </div>
  )
}

// --- Also on this label sidebar section ---

function AlsoOnThisLabel({
  labels,
  currentArtistId,
}: {
  labels: ArtistLabel[]
  currentArtistId: number
}) {
  if (labels.length === 0) return null

  return (
    <div className="space-y-4">
      {labels.map(label => (
        <AlsoOnLabelSection
          key={label.id}
          label={label}
          currentArtistId={currentArtistId}
        />
      ))}
    </div>
  )
}

function AlsoOnLabelSection({
  label,
  currentArtistId,
}: {
  label: ArtistLabel
  currentArtistId: number
}) {
  const { data } = useLabelRoster({
    labelIdOrSlug: label.id,
    enabled: true,
  })

  // Filter out current artist and limit to 5
  const otherArtists = data?.artists
    ?.filter(a => a.id !== currentArtistId)
    .slice(0, 5)

  if (!otherArtists || otherArtists.length === 0) return null

  return (
    <div>
      <h4 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2">
        Also on{' '}
        <Link
          href={`/labels/${label.slug}`}
          className="text-foreground hover:underline"
        >
          {label.name}
        </Link>
      </h4>
      <div className="space-y-1">
        {otherArtists.map(artist => (
          <Link
            key={artist.id}
            href={`/artists/${artist.slug}`}
            className="block text-sm text-muted-foreground hover:text-foreground transition-colors py-0.5"
          >
            {artist.name}
          </Link>
        ))}
      </div>
    </div>
  )
}

// --- Sidebar ---

function ArtistSidebar({
  artist,
  labels,
  labelsLoading,
}: {
  artist: {
    id: number
    name: string
    city: string | null
    state: string | null
    bandcamp_embed_url: string | null
    social: {
      instagram: string | null
      facebook: string | null
      twitter: string | null
      youtube: string | null
      spotify: string | null
      soundcloud: string | null
      bandcamp: string | null
      website: string | null
    }
  }
  labels: ArtistLabel[]
  labelsLoading: boolean
}) {
  const hasLocation = artist.city || artist.state
  const { data: aliasesData } = useArtistAliases(artist.id)
  const aliases = aliasesData?.aliases ?? []

  return (
    <div className="space-y-6">
      {/* Location */}
      {hasLocation && (
        <div>
          <h3 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2">
            Location
          </h3>
          <div className="flex items-center gap-1.5 text-sm">
            <MapPin className="h-4 w-4 text-muted-foreground" />
            <span>{[artist.city, artist.state].filter(Boolean).join(', ')}</span>
          </div>
        </div>
      )}

      {/* Aliases */}
      {aliases.length > 0 && (
        <div>
          <h3 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2">
            Also known as
          </h3>
          <div className="space-y-1">
            {aliases.map(alias => (
              <p key={alias.id} className="text-sm text-muted-foreground">
                {alias.alias}
              </p>
            ))}
          </div>
        </div>
      )}

      {/* Social Links */}
      {artist.social && (
        <div>
          <h3 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2">
            Links
          </h3>
          <SocialLinks social={artist.social} />
        </div>
      )}

      {/* Label Affiliations */}
      {!labelsLoading && labels.length > 0 && (
        <div>
          <h3 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2">
            Labels
          </h3>
          <div className="space-y-1">
            {labels.map(label => (
              <Link
                key={label.id}
                href={`/labels/${label.slug}`}
                className="block text-sm text-muted-foreground hover:text-foreground transition-colors py-0.5"
              >
                {label.name}
              </Link>
            ))}
          </div>
        </div>
      )}

      {/* Music Embed */}
      <MusicEmbed
        bandcampAlbumUrl={artist.bandcamp_embed_url}
        bandcampProfileUrl={artist.social?.bandcamp}
        spotifyUrl={artist.social?.spotify}
        artistName={artist.name}
      />

      {/* Also on this label */}
      {!labelsLoading && labels.length > 0 && (
        <AlsoOnThisLabel labels={labels} currentArtistId={artist.id} />
      )}
    </div>
  )
}

// --- Admin Controls ---

function AdminMusicControls({
  artist,
  artistId,
}: {
  artist: {
    id: number
    name: string
    bandcamp_embed_url: string | null
    social: {
      spotify: string | null
      bandcamp: string | null
    }
  }
  artistId: string | number
}) {
  const queryClient = useQueryClient()
  const [showManualInput, setShowManualInput] = useState<
    'bandcamp' | 'spotify' | null
  >(null)
  const [manualUrl, setManualUrl] = useState('')
  const [feedback, setFeedback] = useState<{
    type: 'success' | 'error'
    message: string
  } | null>(null)

  const discoverMusic = useDiscoverMusic()
  const updateBandcamp = useUpdateArtistBandcamp()
  const clearBandcamp = useClearArtistBandcamp()
  const updateSpotify = useUpdateArtistSpotify()
  const clearSpotify = useClearArtistSpotify()

  const isAnyLoading =
    discoverMusic.isPending ||
    updateBandcamp.isPending ||
    clearBandcamp.isPending ||
    updateSpotify.isPending ||
    clearSpotify.isPending

  const formatPlatformName = (platform: MusicPlatform): string => {
    return platform === 'bandcamp' ? 'Bandcamp' : 'Spotify'
  }

  const handleDiscover = () => {
    setFeedback(null)
    discoverMusic.mutate(artist.id, {
      onSuccess: data => {
        queryClient.invalidateQueries({
          queryKey: queryKeys.artists.detail(artistId),
        })

        let message: string
        if (data.platforms) {
          const found: string[] = []
          if (data.platforms.bandcamp?.found) found.push('Bandcamp')
          if (data.platforms.spotify?.found) found.push('Spotify')
          message =
            found.length > 0
              ? `Found ${found.join(' and ')}`
              : `Found ${data.platform ? formatPlatformName(data.platform) : 'music'}: ${data.url}`
        } else {
          const platformName = data.platform
            ? formatPlatformName(data.platform)
            : 'music'
          message = `Found ${platformName}: ${data.url}`
        }

        setFeedback({ type: 'success', message })
        setShowManualInput(null)
      },
      onError: err => {
        const message = err instanceof Error ? err.message : 'Discovery failed'
        let displayMessage: string
        if (message.includes('credits') || message.includes('Credits')) {
          displayMessage =
            'AI discovery unavailable: API credits exhausted. Use manual entry.'
        } else if (
          message.includes('NOT_FOUND') ||
          message.includes('Could not find')
        ) {
          displayMessage = `No music found for this artist on Bandcamp or Spotify. Try manual entry.`
        } else {
          displayMessage = message
        }
        setFeedback({ type: 'error', message: displayMessage })
      },
    })
  }

  const handleManualSaveBandcamp = () => {
    if (!manualUrl.trim()) return
    setFeedback(null)
    updateBandcamp.mutate(
      { artistId: artist.id, bandcampUrl: manualUrl.trim() },
      {
        onSuccess: () => {
          queryClient.invalidateQueries({
            queryKey: queryKeys.artists.detail(artistId),
          })
          setFeedback({ type: 'success', message: 'Bandcamp URL saved' })
          setShowManualInput(null)
          setManualUrl('')
        },
        onError: err => {
          setFeedback({
            type: 'error',
            message: err instanceof Error ? err.message : 'Failed to save URL',
          })
        },
      }
    )
  }

  const handleManualSaveSpotify = () => {
    if (!manualUrl.trim()) return
    setFeedback(null)
    updateSpotify.mutate(
      { artistId: artist.id, spotifyUrl: manualUrl.trim() },
      {
        onSuccess: () => {
          queryClient.invalidateQueries({
            queryKey: queryKeys.artists.detail(artistId),
          })
          setFeedback({ type: 'success', message: 'Spotify URL saved' })
          setShowManualInput(null)
          setManualUrl('')
        },
        onError: err => {
          setFeedback({
            type: 'error',
            message: err instanceof Error ? err.message : 'Failed to save URL',
          })
        },
      }
    )
  }

  const handleClearBandcamp = () => {
    setFeedback(null)
    clearBandcamp.mutate(artist.id, {
      onSuccess: () => {
        queryClient.invalidateQueries({
          queryKey: queryKeys.artists.detail(artistId),
        })
        setFeedback({ type: 'success', message: 'Bandcamp URL cleared' })
        setShowManualInput(null)
      },
      onError: err => {
        setFeedback({
          type: 'error',
          message: err instanceof Error ? err.message : 'Failed to clear URL',
        })
      },
    })
  }

  const handleClearSpotify = () => {
    setFeedback(null)
    clearSpotify.mutate(artist.id, {
      onSuccess: () => {
        queryClient.invalidateQueries({
          queryKey: queryKeys.artists.detail(artistId),
        })
        setFeedback({ type: 'success', message: 'Spotify URL cleared' })
        setShowManualInput(null)
      },
      onError: err => {
        setFeedback({
          type: 'error',
          message: err instanceof Error ? err.message : 'Failed to clear URL',
        })
      },
    })
  }

  const handleCancelEdit = () => {
    setShowManualInput(null)
    setManualUrl('')
    setFeedback(null)
  }

  const hasBandcamp = !!artist.bandcamp_embed_url
  const hasSpotify = !!artist.social?.spotify
  const hasAnyEmbed = hasBandcamp || hasSpotify

  return (
    <div className="mb-6">
      {feedback && (
        <Alert
          variant={feedback.type === 'error' ? 'destructive' : 'default'}
          className="mb-4"
        >
          {feedback.type === 'error' && <AlertCircle className="h-4 w-4" />}
          {feedback.type === 'success' && <Check className="h-4 w-4" />}
          <AlertDescription>{feedback.message}</AlertDescription>
        </Alert>
      )}

      {!hasAnyEmbed && !showManualInput && (
        <div className="p-4 rounded-lg border border-dashed border-muted-foreground/25 bg-muted/30">
          <p className="text-sm text-muted-foreground mb-3">
            No music embed configured
          </p>
          <div className="flex flex-wrap gap-2">
            <Button
              onClick={handleDiscover}
              disabled={isAnyLoading}
              size="sm"
            >
              {discoverMusic.isPending ? (
                <>
                  <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                  Discovering...
                </>
              ) : (
                <>
                  <Sparkles className="h-4 w-4 mr-2" />
                  Discover Music
                </>
              )}
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={() => setShowManualInput('bandcamp')}
              disabled={isAnyLoading}
            >
              <Pencil className="h-4 w-4 mr-2" />
              Enter Bandcamp URL
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={() => setShowManualInput('spotify')}
              disabled={isAnyLoading}
            >
              <Pencil className="h-4 w-4 mr-2" />
              Enter Spotify URL
            </Button>
          </div>
        </div>
      )}

      {hasAnyEmbed && !showManualInput && (
        <div className="flex flex-wrap gap-2">
          {hasBandcamp && (
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                setManualUrl(artist.bandcamp_embed_url || '')
                setShowManualInput('bandcamp')
                setFeedback(null)
              }}
              disabled={isAnyLoading}
            >
              <Pencil className="h-4 w-4 mr-2" />
              Edit Bandcamp URL
            </Button>
          )}
          {hasSpotify && (
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                setManualUrl(artist.social?.spotify || '')
                setShowManualInput('spotify')
                setFeedback(null)
              }}
              disabled={isAnyLoading}
            >
              <Pencil className="h-4 w-4 mr-2" />
              Edit Spotify URL
            </Button>
          )}
          {hasBandcamp && !hasSpotify && (
            <Button
              variant="outline"
              size="sm"
              onClick={() => setShowManualInput('spotify')}
              disabled={isAnyLoading}
            >
              <Pencil className="h-4 w-4 mr-2" />
              Add Spotify URL
            </Button>
          )}
          {hasSpotify && !hasBandcamp && (
            <Button
              variant="outline"
              size="sm"
              onClick={() => setShowManualInput('bandcamp')}
              disabled={isAnyLoading}
            >
              <Pencil className="h-4 w-4 mr-2" />
              Add Bandcamp URL
            </Button>
          )}
        </div>
      )}

      {showManualInput === 'bandcamp' && (
        <div className="p-4 rounded-lg border border-muted-foreground/25 bg-muted/30">
          <label
            htmlFor="bandcamp-url"
            className="block text-sm font-medium mb-2"
          >
            Bandcamp Album URL
          </label>
          <div className="flex gap-2">
            <Input
              id="bandcamp-url"
              type="url"
              placeholder="https://artist.bandcamp.com/album/album-name"
              value={manualUrl}
              onChange={e => setManualUrl(e.target.value)}
              disabled={isAnyLoading}
              className="flex-1"
            />
            <Button
              onClick={handleManualSaveBandcamp}
              disabled={isAnyLoading || !manualUrl.trim()}
              size="sm"
            >
              {updateBandcamp.isPending ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                <Check className="h-4 w-4" />
              )}
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={handleCancelEdit}
              disabled={isAnyLoading}
            >
              <X className="h-4 w-4" />
            </Button>
          </div>
          {hasBandcamp && (
            <div className="mt-3 pt-3 border-t border-muted-foreground/25">
              <Button
                variant="ghost"
                size="sm"
                onClick={handleClearBandcamp}
                disabled={isAnyLoading}
                className="text-destructive hover:text-destructive"
              >
                {clearBandcamp.isPending ? (
                  <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                ) : (
                  <X className="h-4 w-4 mr-2" />
                )}
                Clear Bandcamp URL
              </Button>
            </div>
          )}
        </div>
      )}

      {showManualInput === 'spotify' && (
        <div className="p-4 rounded-lg border border-muted-foreground/25 bg-muted/30">
          <label
            htmlFor="spotify-url"
            className="block text-sm font-medium mb-2"
          >
            Spotify Artist URL
          </label>
          <div className="flex gap-2">
            <Input
              id="spotify-url"
              type="url"
              placeholder="https://open.spotify.com/artist/..."
              value={manualUrl}
              onChange={e => setManualUrl(e.target.value)}
              disabled={isAnyLoading}
              className="flex-1"
            />
            <Button
              onClick={handleManualSaveSpotify}
              disabled={isAnyLoading || !manualUrl.trim()}
              size="sm"
            >
              {updateSpotify.isPending ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                <Check className="h-4 w-4" />
              )}
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={handleCancelEdit}
              disabled={isAnyLoading}
            >
              <X className="h-4 w-4" />
            </Button>
          </div>
          {hasSpotify && (
            <div className="mt-3 pt-3 border-t border-muted-foreground/25">
              <Button
                variant="ghost"
                size="sm"
                onClick={handleClearSpotify}
                disabled={isAnyLoading}
                className="text-destructive hover:text-destructive"
              >
                {clearSpotify.isPending ? (
                  <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                ) : (
                  <X className="h-4 w-4 mr-2" />
                )}
                Clear Spotify URL
              </Button>
            </div>
          )}
        </div>
      )}
    </div>
  )
}

// --- Main Component ---

export function ArtistDetail({ artistId }: ArtistDetailProps) {
  const queryClient = useQueryClient()
  const { data: artist, isLoading, error } = useArtist({ artistId })
  const { user, isAuthenticated } = useIsAuthenticated()
  const isAdmin = isAuthenticated && user?.is_admin

  const [activeTab, setActiveTab] = useState('overview')
  const [isEditing, setIsEditing] = useState(false)

  // Fetch labels for sidebar
  const { data: labelsData, isLoading: labelsLoading } = useArtistLabels({
    artistIdOrSlug: artistId,
    enabled: !!artist,
  })

  if (isLoading) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (error) {
    const errorMessage =
      error instanceof Error ? error.message : 'Failed to load artist'
    const is404 =
      errorMessage.includes('not found') || errorMessage.includes('404')

    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <div className="text-center">
          <h1 className="text-2xl font-bold mb-2">
            {is404 ? 'Artist Not Found' : 'Error Loading Artist'}
          </h1>
          <p className="text-muted-foreground mb-4">
            {is404
              ? "The artist you're looking for doesn't exist or has been removed."
              : errorMessage}
          </p>
          <Button asChild variant="outline">
            <Link href="/artists">
              <ArrowLeft className="h-4 w-4 mr-2" />
              Back to Artists
            </Link>
          </Button>
        </div>
      </div>
    )
  }

  if (!artist) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <div className="text-center">
          <h1 className="text-2xl font-bold mb-2">Artist Not Found</h1>
          <p className="text-muted-foreground mb-4">
            The artist you&apos;re looking for doesn&apos;t exist.
          </p>
          <Button asChild variant="outline">
            <Link href="/artists">
              <ArrowLeft className="h-4 w-4 mr-2" />
              Back to Artists
            </Link>
          </Button>
        </div>
      </div>
    )
  }

  const labels = labelsData?.labels ?? []

  // Build tabs - only show tabs that have content or always show
  const tabs = [
    { value: 'overview', label: 'Overview' },
    { value: 'discography', label: 'Discography' },
    { value: 'labels', label: 'Labels' },
  ]

  const headerSubtitle = (artist.city || artist.state) ? (
    <>
      <MapPin className="h-4 w-4" />
      <span>{[artist.city, artist.state].filter(Boolean).join(', ')}</span>
    </>
  ) : undefined

  const headerActions = (
    <div className="flex items-center gap-2">
      <FollowButton entityType="artists" entityId={artist.id} />
      {isAdmin && (
        <Button
          variant="ghost"
          size="sm"
          onClick={() => setIsEditing(true)}
          className="text-muted-foreground hover:text-foreground"
        >
          <Edit2 className="h-4 w-4" />
        </Button>
      )}
      <ReportArtistButton artistId={artist.id} artistName={artist.name} />
    </div>
  )

  return (
    <>
      <EntityDetailLayout
        backLink={{ href: '/artists', label: 'Back to Artists' }}
        header={
          <EntityHeader
            title={artist.name}
            subtitle={headerSubtitle}
            actions={headerActions}
          />
        }
        tabs={tabs}
        activeTab={activeTab}
        onTabChange={setActiveTab}
        sidebar={
          <ArtistSidebar
            artist={artist}
            labels={labels}
            labelsLoading={labelsLoading}
          />
        }
      >
        {/* Overview Tab */}
        <TabsContent value="overview">
          {/* Admin music embed controls */}
          {isAdmin && (
            <AdminMusicControls artist={artist} artistId={artistId} />
          )}

          {/* Shows List */}
          <ArtistShowsList artistId={artist.id} />
        </TabsContent>

        {/* Discography Tab */}
        <TabsContent value="discography">
          <DiscographyTab artistIdOrSlug={artistId} />
        </TabsContent>

        {/* Labels Tab */}
        <TabsContent value="labels">
          <LabelsTab artistIdOrSlug={artistId} />
        </TabsContent>
      </EntityDetailLayout>

      {/* Tags */}
      <div className="mt-0 px-4 md:px-0">
        <EntityTagList
          entityType="artist"
          entityId={artist.id}
          isAuthenticated={isAuthenticated}
        />
      </div>

      {/* Revision History */}
      <div className="mt-0">
        <RevisionHistory
          entityType="artist"
          entityId={artist.id}
          isAdmin={!!isAdmin}
        />
      </div>

      {/* Admin Edit Dialog */}
      {isAdmin && (
        <ArtistEditForm
          artist={artist}
          open={isEditing}
          onOpenChange={setIsEditing}
          onSuccess={() => {
            queryClient.invalidateQueries({
              queryKey: queryKeys.artists.detail(artistId),
            })
          }}
        />
      )}
    </>
  )
}
