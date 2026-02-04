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
} from 'lucide-react'
import { useQueryClient } from '@tanstack/react-query'
import { useArtist } from '@/lib/hooks/useArtists'
import { queryKeys } from '@/lib/queryClient'
import { useIsAuthenticated } from '@/lib/hooks/useAuth'
import {
  useDiscoverMusic,
  useUpdateArtistBandcamp,
  useClearArtistBandcamp,
  useUpdateArtistSpotify,
  useClearArtistSpotify,
  type MusicPlatform,
} from '@/lib/hooks/useAdminArtists'
import { SocialLinks, MusicEmbed } from '@/components/shared'
import { ArtistShowsList } from './ArtistShowsList'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Alert, AlertDescription } from '@/components/ui/alert'

interface ArtistDetailProps {
  artistId: string | number
}

export function ArtistDetail({ artistId }: ArtistDetailProps) {
  const queryClient = useQueryClient()
  const { data: artist, isLoading, error } = useArtist({ artistId })
  const { user, isAuthenticated } = useIsAuthenticated()
  const isAdmin = isAuthenticated && user?.is_admin

  // Admin state for music embed management
  const [showManualInput, setShowManualInput] = useState<
    'bandcamp' | 'spotify' | null
  >(null)
  const [manualUrl, setManualUrl] = useState('')
  const [feedback, setFeedback] = useState<{
    type: 'success' | 'error'
    message: string
  } | null>(null)

  // Admin mutations
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

  // Helper to format platform name for display
  const formatPlatformName = (platform: MusicPlatform): string => {
    return platform === 'bandcamp' ? 'Bandcamp' : 'Spotify'
  }

  // Handlers - use artist.id (numeric) for mutations
  const handleDiscover = () => {
    if (!artist) return
    setFeedback(null)
    discoverMusic.mutate(artist.id, {
      onSuccess: data => {
        // Invalidate using the original artistId (slug) to refresh the UI
        queryClient.invalidateQueries({
          queryKey: queryKeys.artists.detail(artistId),
        })
        const platformName = data.platform
          ? formatPlatformName(data.platform)
          : 'music'
        setFeedback({
          type: 'success',
          message: `Found ${platformName}: ${data.url}`,
        })
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
        setFeedback({
          type: 'error',
          message: displayMessage,
        })
      },
    })
  }

  const handleManualSaveBandcamp = () => {
    if (!manualUrl.trim() || !artist) return

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
    if (!manualUrl.trim() || !artist) return

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
    if (!artist) return
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
    if (!artist) return
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

  const handleEditBandcampClick = () => {
    setManualUrl(artist?.bandcamp_embed_url || '')
    setShowManualInput('bandcamp')
    setFeedback(null)
  }

  const handleEditSpotifyClick = () => {
    setManualUrl(artist?.social?.spotify || '')
    setShowManualInput('spotify')
    setFeedback(null)
  }

  const handleCancelEdit = () => {
    setShowManualInput(null)
    setManualUrl('')
    setFeedback(null)
  }

  // Determine what embeds are configured
  const hasBandcamp = !!artist?.bandcamp_embed_url
  const hasSpotify = !!artist?.social?.spotify
  const hasAnyEmbed = hasBandcamp || hasSpotify

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
            <Link href="/shows">
              <ArrowLeft className="h-4 w-4 mr-2" />
              Back to Shows
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
            <Link href="/shows">
              <ArrowLeft className="h-4 w-4 mr-2" />
              Back to Shows
            </Link>
          </Button>
        </div>
      </div>
    )
  }

  const hasLocation = artist.city || artist.state

  return (
    <div className="container max-w-4xl mx-auto px-4 py-6">
      {/* Back Navigation */}
      <div className="mb-6">
        <Link
          href="/shows"
          className="inline-flex items-center text-sm text-muted-foreground hover:text-foreground transition-colors"
        >
          <ArrowLeft className="h-4 w-4 mr-1" />
          Back to Shows
        </Link>
      </div>

      {/* Header */}
      <header className="mb-8">
        <h1 className="text-2xl md:text-3xl font-bold">{artist.name}</h1>
        {hasLocation && (
          <div className="flex items-center gap-1 text-muted-foreground mt-2">
            <MapPin className="h-4 w-4" />
            <span>
              {[artist.city, artist.state].filter(Boolean).join(', ')}
            </span>
          </div>
        )}

        {/* Social Links */}
        {artist.social && <SocialLinks social={artist.social} className="mt-4" />}
      </header>

      {/* Music Embed */}
      <MusicEmbed
        bandcampAlbumUrl={artist.bandcamp_embed_url}
        bandcampProfileUrl={artist.social?.bandcamp}
        spotifyUrl={artist.social?.spotify}
        artistName={artist.name}
      />

      {/* Admin Controls for Music Embed Management */}
      {isAdmin && (
        <div className="mb-8">
          {/* Feedback Alert */}
          {feedback && (
            <Alert
              variant={feedback.type === 'error' ? 'destructive' : 'default'}
              className="mb-4"
            >
              {feedback.type === 'error' && (
                <AlertCircle className="h-4 w-4" />
              )}
              {feedback.type === 'success' && <Check className="h-4 w-4" />}
              <AlertDescription>{feedback.message}</AlertDescription>
            </Alert>
          )}

          {/* No embed configured - show discovery options */}
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

          {/* Embeds exist - show edit buttons */}
          {hasAnyEmbed && !showManualInput && (
            <div className="flex flex-wrap gap-2">
              {hasBandcamp && (
                <Button
                  variant="outline"
                  size="sm"
                  onClick={handleEditBandcampClick}
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
                  onClick={handleEditSpotifyClick}
                  disabled={isAnyLoading}
                >
                  <Pencil className="h-4 w-4 mr-2" />
                  Edit Spotify URL
                </Button>
              )}
              {/* Allow adding the other platform if only one is configured */}
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

          {/* Bandcamp URL input form */}
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

          {/* Spotify URL input form */}
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
      )}

      {/* Shows List */}
      <ArtistShowsList artistId={artist.id} artistName={artist.name} />
    </div>
  )
}
