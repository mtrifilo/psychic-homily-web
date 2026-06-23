'use client'

import { useEffect, useState } from 'react'
import * as Sentry from '@sentry/nextjs'
import { ExternalLink, Loader2, Music } from 'lucide-react'
import { parseSpotifyEmbed, type SpotifyEmbedKind } from '@/lib/spotify'
import { bandcampEmbedSrc, type BandcampEmbedResponse } from '@/lib/bandcamp'

interface MusicEmbedProps {
  bandcampAlbumUrl?: string | null
  bandcampProfileUrl?: string | null
  spotifyUrl?: string | null
  artistName: string
  compact?: boolean
}

type EmbedState =
  | { type: 'loading' }
  | { type: 'bandcamp'; embedKind: 'album' | 'track'; embedId: string }
  | { type: 'spotify'; spotifyKind: SpotifyEmbedKind; spotifyId: string }
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
    let cancelled = false

    async function resolveEmbed(): Promise<EmbedState> {
      // Priority 1: Bandcamp album/track URL - resolve the embed kind + id
      if (bandcampAlbumUrl) {
        try {
          const response = await fetch(
            `/api/bandcamp/album-id?url=${encodeURIComponent(bandcampAlbumUrl)}`
          )
          if (response.ok) {
            const data = (await response.json()) as BandcampEmbedResponse
            if (data.id && (data.kind === 'album' || data.kind === 'track')) {
              return { type: 'bandcamp', embedKind: data.kind, embedId: data.id }
            }
          }
        } catch (error) {
          Sentry.captureException(error, {
            level: 'error',
            tags: { service: 'music-embed' },
            extra: { bandcampAlbumUrl },
          })
          // Error captured by Sentry above
        }
      }

      // Priority 2: Spotify artist/album/track URL. Artist pages pass an artist
      // URL; release pages pass an album/track URL (PSY-1195). parseSpotifyEmbed
      // host-anchors + id-validates all three, so a bad/look-alike URL falls
      // through to the fallbacks below rather than producing a broken iframe.
      if (spotifyUrl) {
        const parsed = parseSpotifyEmbed(spotifyUrl)
        if (parsed) {
          return { type: 'spotify', spotifyKind: parsed.kind, spotifyId: parsed.id }
        }
      }

      // Priority 3: Bandcamp fallback links
      if (bandcampAlbumUrl) {
        return {
          type: 'fallback',
          url: bandcampAlbumUrl,
          label: `Listen to ${artistName} on Bandcamp`,
        }
      }

      if (bandcampProfileUrl) {
        return {
          type: 'fallback',
          url: bandcampProfileUrl,
          label: `Listen to ${artistName} on Bandcamp`,
        }
      }

      return { type: 'none' }
    }

    // Ignore a resolution that finishes after the deps changed/unmounted, so a
    // slow earlier request can't overwrite a newer embed.
    resolveEmbed().then(state => {
      if (!cancelled) setEmbed(state)
    })

    return () => {
      cancelled = true
    }
  }, [bandcampAlbumUrl, bandcampProfileUrl, spotifyUrl, artistName])

  if (embed.type === 'none') {
    return null
  }

  if (embed.type === 'loading') {
    return (
      <section className={compact ? 'mb-2' : 'mb-8'}>
        {!compact && (
          <h2 className="text-lg font-display font-semibold mb-4 flex items-center gap-2">
            <Music className="h-5 w-5" />
            Music
          </h2>
        )}
        <div className={`flex items-center justify-center ${compact ? 'py-4' : 'py-8'} bg-muted/30 rounded-md`}>
          <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
        </div>
      </section>
    )
  }

  return (
    <section className={compact ? 'mb-2' : 'mb-8'}>
      {!compact && (
        <h2 className="text-lg font-display font-semibold mb-4 flex items-center gap-2">
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
            src={bandcampEmbedSrc({ kind: embed.embedKind, id: embed.embedId })}
            seamless
          />
        </div>
      ) : (
        <div className="music-embed-container">
          <iframe
            title={`${artistName} on Spotify`}
            style={{ borderRadius: '12px', width: '100%', height: compact ? '152px' : '352px' }}
            src={`https://open.spotify.com/embed/${embed.spotifyKind}/${embed.spotifyId}?utm_source=generator&theme=0`}
            frameBorder="0"
            allow="autoplay; clipboard-write; encrypted-media; fullscreen; picture-in-picture"
            loading="lazy"
          />
        </div>
      )}
    </section>
  )
}
