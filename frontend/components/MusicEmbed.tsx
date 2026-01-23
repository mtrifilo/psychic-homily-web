'use client'

import { useEffect, useState } from 'react'
import { ExternalLink, Loader2, Music } from 'lucide-react'

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

export function MusicEmbed({
  bandcampAlbumUrl,
  bandcampProfileUrl,
  spotifyUrl,
  artistName,
  compact = false,
}: MusicEmbedProps) {
  const [embed, setEmbed] = useState<EmbedState>({ type: 'loading' })

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
          <iframe
            title={`${artistName} on Bandcamp`}
            style={{ border: 0, width: '100%', maxWidth: '700px', height: '120px' }}
            src={`https://bandcamp.com/EmbeddedPlayer/album=${embed.albumId}/size=large/bgcol=1a1a1a/linkcol=f59e0b/tracklist=false/artwork=small/`}
            seamless
          />
        </div>
      ) : (
        <div className="music-embed-container">
          <iframe
            title={`${artistName} on Spotify`}
            style={{ borderRadius: '12px', width: '100%', height: compact ? '152px' : '352px' }}
            src={`https://open.spotify.com/embed/artist/${embed.artistId}?utm_source=generator&theme=0`}
            frameBorder="0"
            allow="autoplay; clipboard-write; encrypted-media; fullscreen; picture-in-picture"
            loading="lazy"
          />
        </div>
      )}
    </section>
  )
}
