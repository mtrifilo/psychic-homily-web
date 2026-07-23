'use client'

import { useQuery } from '@tanstack/react-query'
import * as Sentry from '@sentry/nextjs'
import { ExternalLink, Loader2, Music } from 'lucide-react'
import { parseSpotifyEmbed, type SpotifyEmbedKind } from '@/lib/spotify'
import { bandcampEmbedSrc, type BandcampEmbedResponse } from '@/lib/bandcamp'
import { queryKeys } from '@/lib/queryClient'

interface MusicEmbedProps {
  bandcampAlbumUrl?: string | null
  bandcampProfileUrl?: string | null
  spotifyUrl?: string | null
  artistName: string
  compact?: boolean
}

type BandcampEmbed = Pick<BandcampEmbedResponse, 'kind' | 'id'> & {
  kind: 'album' | 'track'
  id: string
}

// Resolve a Bandcamp album/track URL to its embeddable { kind, id } descriptor
// via the `/api/bandcamp/album-id` Next.js route handler (a relative path, NOT
// the Go backend `apiRequest` stack).
//
// Return-vs-throw is deliberate, because TanStack Query caches a returned value
// as a durable success (global staleTime 15min) but does NOT cache a thrown
// error. The route scrapes a third-party site, so a transient upstream outage
// surfaces as a 5xx. If we returned null for that, the query would cache the
// null success for 15min and every same-URL instance + remount would render the
// plain fallback link until staleTime expires — even after Bandcamp recovers
// seconds later (PSY-1102 adversarial review). So:
//   - 5xx / network throw  → THROW → query errors → no durable cache, a later
//     mount retries → embed self-heals once the outage clears.
//   - 4xx (incl. the route's 404 "no embed found") → return null → a genuine
//     "this URL has no embeddable player" answer that IS safe to cache.
//   - 2xx with no usable id/kind → return null (same: a real negative answer).
async function resolveBandcampEmbed(albumUrl: string): Promise<BandcampEmbed | null> {
  const response = await fetch(
    `/api/bandcamp/album-id?url=${encodeURIComponent(albumUrl)}`
  )
  if (!response.ok) {
    if (response.status >= 500) {
      throw new Error(`Bandcamp embed resolve failed: ${response.status}`)
    }
    return null
  }

  const data = (await response.json()) as BandcampEmbedResponse
  if (data.id && (data.kind === 'album' || data.kind === 'track')) {
    return { kind: data.kind, id: data.id }
  }
  return null
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
  // The bandcamp resolve is the only async dependency. Keying on the album URL
  // dedups the `/api/bandcamp/album-id` request across the many MusicEmbed
  // instances a list page mounts and caches the result across nav/remount.
  // Disabled when there's no album URL so Spotify-only / fallback-only embeds
  // resolve synchronously without a wasted request. The empty-string fallback
  // key is never fetched (the query is disabled in that case).
  const bandcampQuery = useQuery({
    queryKey: queryKeys.bandcamp.embed(bandcampAlbumUrl ?? ''),
    queryFn: async () => {
      try {
        return await resolveBandcampEmbed(bandcampAlbumUrl as string)
      } catch (error) {
        Sentry.captureException(error, {
          level: 'error',
          tags: { service: 'music-embed' },
          extra: { bandcampAlbumUrl },
        })
        // Re-throw so the query is marked errored; the embed-state derivation
        // treats an errored bandcamp resolve the same as "no embed" and falls
        // through to Spotify / fallback links.
        throw error
      }
    },
    enabled: Boolean(bandcampAlbumUrl),
    // The embed is a best-effort enhancement: a failed resolve should fall
    // through to the Spotify / fallback link immediately. Without this, a
    // network-level fetch rejection would inherit the global 3x retry-with-
    // backoff and keep the embed on a spinner for several seconds before
    // falling through — the pre-TanStack code fell through on the first error.
    retry: false,
  })

  const embed = deriveEmbedState({
    bandcampAlbumUrl,
    bandcampProfileUrl,
    spotifyUrl,
    artistName,
    bandcampIsPending: bandcampQuery.isPending,
    bandcampEmbed: bandcampQuery.data ?? null,
  })

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

// Map the resolved inputs onto the rendered embed, preserving the
// bandcamp(album/track) → spotify → bandcamp-fallback → profile-fallback
// priority. Kept pure (no hooks, no fetch) so the priority logic is testable
// and obvious at a single level of abstraction.
function deriveEmbedState({
  bandcampAlbumUrl,
  bandcampProfileUrl,
  spotifyUrl,
  artistName,
  bandcampIsPending,
  bandcampEmbed,
}: {
  bandcampAlbumUrl?: string | null
  bandcampProfileUrl?: string | null
  spotifyUrl?: string | null
  artistName: string
  // Raw `useQuery().isPending`. A DISABLED query (no album URL) also reports
  // `isPending: true`, so this is only a real "still resolving" signal when
  // there is an album URL — the guard below enforces that here, in one place,
  // rather than relying on every caller to remember it.
  bandcampIsPending: boolean
  bandcampEmbed: BandcampEmbed | null
}): EmbedState {
  // Priority 1: Bandcamp album/track embed (resolved kind + id).
  if (bandcampAlbumUrl) {
    if (bandcampIsPending) {
      return { type: 'loading' }
    }
    if (bandcampEmbed) {
      return { type: 'bandcamp', embedKind: bandcampEmbed.kind, embedId: bandcampEmbed.id }
    }
    // Resolve finished with no usable embed (error or empty) → fall through.
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

  // Priority 3: Bandcamp fallback links.
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
