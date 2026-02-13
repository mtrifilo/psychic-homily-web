'use client'

import { useEffect, useState } from 'react'
import * as Sentry from '@sentry/nextjs'
import { ExternalLink, Loader2, Music, Play } from 'lucide-react'

interface MusicEmbedProps {
  bandcampAlbumUrl?: string | null
  bandcampProfileUrl?: string | null
  spotifyUrl?: string | null
  artistName: string
  compact?: boolean
}

function parseSpotifyArtistId(url: string): string | null {
  const webMatch = url.match(/spotify\.com\/artist\/([a-zA-Z0-9]+)/)
  if (webMatch) return webMatch[1]

  const uriMatch = url.match(/spotify:artist:([a-zA-Z0-9]+)/)
  if (uriMatch) return uriMatch[1]

  return null
}

type EmbedState =
  | { type: 'loading' }
  | { type: 'bandcamp'; albumId: string }
  | { type: 'spotify'; artistId: string }
  | { type: 'fallback'; url: string; label: string }
  | { type: 'none' }

function BandcampFacade({
  albumId,
  artistName,
  compact,
  isLoaded,
  onLoad,
}: {
  albumId: string
  artistName: string
  compact: boolean
  isLoaded: boolean
  onLoad: () => void
}) {
  if (isLoaded) {
    return (
      <iframe
        title={`${artistName} on Bandcamp`}
        style={{ border: 0, width: '100%', maxWidth: '700px', height: '120px' }}
        src={`https://bandcamp.com/EmbeddedPlayer/album=${albumId}/size=large/bgcol=1a1a1a/linkcol=f59e0b/tracklist=false/artwork=small/`}
        seamless
      />
    )
  }

  return (
    <button
      onClick={onLoad}
      className="flex items-center justify-center w-full bg-[#1a1a1a] hover:bg-[#2a2a2a] transition-colors rounded-md"
      style={{ height: compact ? '60px' : '120px' }}
      aria-label={`Play ${artistName} on Bandcamp`}
    >
      <div className="flex items-center gap-3 text-white">
        <div className="flex items-center justify-center w-10 h-10 rounded-full bg-[#629aa9]">
          <Play className="h-5 w-5 ml-0.5" fill="white" />
        </div>
        <span className="font-medium">Listen on Bandcamp</span>
      </div>
    </button>
  )
}

function SpotifyFacade({
  artistId,
  artistName,
  compact,
  isLoaded,
  onLoad,
}: {
  artistId: string
  artistName: string
  compact: boolean
  isLoaded: boolean
  onLoad: () => void
}) {
  if (isLoaded) {
    return (
      <iframe
        title={`${artistName} on Spotify`}
        style={{ borderRadius: '12px', width: '100%', height: compact ? '152px' : '352px' }}
        src={`https://open.spotify.com/embed/artist/${artistId}?utm_source=generator&theme=0`}
        frameBorder="0"
        allow="autoplay; clipboard-write; encrypted-media; fullscreen; picture-in-picture"
      />
    )
  }

  return (
    <button
      onClick={onLoad}
      className="flex items-center justify-center w-full bg-black hover:bg-[#181818] transition-colors rounded-lg"
      style={{ height: compact ? '152px' : '352px' }}
      aria-label={`Listen to ${artistName} on Spotify`}
    >
      <div className="flex flex-col items-center gap-3">
        <div className="w-16 h-16 rounded-full bg-[#1DB954] flex items-center justify-center">
          <Play className="h-8 w-8 ml-1" fill="black" />
        </div>
        <span className="text-white font-medium">Listen on Spotify</span>
      </div>
    </button>
  )
}

export function MusicEmbed({
  bandcampAlbumUrl,
  bandcampProfileUrl,
  spotifyUrl,
  artistName,
  compact = false,
}: MusicEmbedProps) {
  const [embed, setEmbed] = useState<EmbedState>({ type: 'loading' })
  const [isEmbedLoaded, setIsEmbedLoaded] = useState(false)

  useEffect(() => {
    async function resolveEmbed() {
      // Priority 1: Bandcamp album URL - fetch album ID
      if (bandcampAlbumUrl) {
        try {
          const response = await fetch(
            `/api/bandcamp/album-id?url=${encodeURIComponent(bandcampAlbumUrl)}`
          )
          if (response.ok) {
            const data = await response.json()
            if (data.albumId) {
              setEmbed({ type: 'bandcamp', albumId: data.albumId })
              return
            }
          }
        } catch (error) {
          Sentry.captureException(error, {
            level: 'error',
            tags: { service: 'music-embed' },
            extra: { bandcampAlbumUrl },
          })
          console.error('Failed to fetch Bandcamp album ID:', error)
        }
      }

      // Priority 2: Spotify artist URL
      if (spotifyUrl) {
        const artistId = parseSpotifyArtistId(spotifyUrl)
        if (artistId) {
          setEmbed({ type: 'spotify', artistId })
          return
        }
      }

      // Priority 3: Bandcamp fallback links
      if (bandcampAlbumUrl) {
        setEmbed({
          type: 'fallback',
          url: bandcampAlbumUrl,
          label: `Listen to ${artistName} on Bandcamp`,
        })
        return
      }

      if (bandcampProfileUrl) {
        setEmbed({
          type: 'fallback',
          url: bandcampProfileUrl,
          label: `Listen to ${artistName} on Bandcamp`,
        })
        return
      }

      setEmbed({ type: 'none' })
    }

    resolveEmbed()
  }, [bandcampAlbumUrl, bandcampProfileUrl, spotifyUrl, artistName])

  const handleLoadEmbed = () => {
    setIsEmbedLoaded(true)
  }

  if (embed.type === 'none') {
    return null
  }

  if (embed.type === 'loading') {
    return (
      <section className={compact ? 'mb-2' : 'mb-8'}>
        {!compact && (
          <h2 className="text-lg font-semibold mb-4 flex items-center gap-2">
            <Music className="h-5 w-5" />
            Music
          </h2>
        )}
        <div className={`flex items-center justify-center ${compact ? 'py-4' : 'py-8'} bg-muted/30 rounded-lg`}>
          <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
        </div>
      </section>
    )
  }

  return (
    <section className={compact ? 'mb-2' : 'mb-8'}>
      {!compact && (
        <h2 className="text-lg font-semibold mb-4 flex items-center gap-2">
          <Music className="h-5 w-5" />
          Music
        </h2>
      )}
      {embed.type === 'fallback' ? (
        <a
          href={embed.url}
          target="_blank"
          rel="noopener noreferrer"
          className="flex items-center gap-2 text-primary hover:underline text-sm"
        >
          {embed.label}
          <ExternalLink className="h-4 w-4" />
        </a>
      ) : embed.type === 'bandcamp' ? (
        <div className="music-embed-container">
          <BandcampFacade
            albumId={embed.albumId}
            artistName={artistName}
            compact={compact}
            isLoaded={isEmbedLoaded}
            onLoad={handleLoadEmbed}
          />
        </div>
      ) : (
        <div className="music-embed-container">
          <SpotifyFacade
            artistId={embed.artistId}
            artistName={artistName}
            compact={compact}
            isLoaded={isEmbedLoaded}
            onLoad={handleLoadEmbed}
          />
        </div>
      )}
    </section>
  )
}
