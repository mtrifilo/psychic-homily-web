'use client'

import Link from 'next/link'
import { MapPin } from 'lucide-react'
import { MusicEmbed } from '@/components/shared/MusicEmbed'
import { SocialLinks } from '@/components/shared/SocialLinks'
import { hasRenderableMusic } from '@/lib/musicAvailability'
import { basedInPhrase, billHometown } from '../utils'
import type { ArtistResponse } from '../types'

/**
 * Whether an artist's music block will render anything. A stored Bandcamp URL
 * no longer implies that on its own, so this asks the shared predicate rather
 * than testing the column, or the expand control would open onto nothing.
 */
export function artistHasMusic(artist: ArtistResponse): boolean {
  return hasRenderableMusic({
    bandcampAlbumUrl: artist.bandcamp_embed_url,
    bandcampProfileUrl: artist.socials?.bandcamp,
    spotifyUrl: artist.socials?.spotify,
  })
}

/** Whether any act on the bill has music to open. */
export function showHasArtistMusic(artists: ArtistResponse[]): boolean {
  return artists.some(artistHasMusic)
}

/**
 * Where an act is based: `based in Tempe, AZ`, or nothing when unplaceable.
 *
 * {@link billHometown} counts COUNTRY as placeable, so an act carrying only a
 * country states it, and its country is included unless the state is set and
 * the country is USA/US.
 */
export function ArtistBase({ artist }: { artist: ArtistResponse }) {
  const base = basedInPhrase(billHometown(artist))
  if (!base) return null
  return (
    <div className="mt-0.5 flex items-center gap-1 text-xs text-muted-foreground">
      <MapPin className="h-3 w-3" />
      <span>{base}</span>
    </div>
  )
}

/**
 * The music a show's bill opens onto: a player per act that has one.
 *
 * Players render OPEN, never as a click-to-load facade (locked decision): the
 * discovery loop is a reader scanning tonight's shows for bands they have never
 * heard, and any second click kills it.
 *
 * Outer spacing is the caller's, through `className`; everything inside the
 * panel is the panel's, so no surface forks its own copy of the per-act line.
 */
export function ShowArtistMusicPanel({
  artists,
  className,
}: {
  artists: ArtistResponse[]
  className?: string
}) {
  const withMusic = artists.filter(artistHasMusic)
  if (withMusic.length === 0) return null

  return (
    <div className={className}>
      <div className="space-y-6">
        {withMusic.map(artist => (
          <div key={artist.id} className="space-y-2">
            <div className="flex items-start justify-between gap-2">
              <div>
                {artist.slug ? (
                  <Link
                    href={`/artists/${artist.slug}`}
                    className="font-medium transition-colors hover:text-primary"
                  >
                    {artist.name}
                  </Link>
                ) : (
                  <span className="font-medium">{artist.name}</span>
                )}
                <ArtistBase artist={artist} />
              </div>
              <SocialLinks social={artist.socials} className="shrink-0" />
            </div>
            <MusicEmbed
              bandcampAlbumUrl={artist.bandcamp_embed_url}
              bandcampProfileUrl={artist.socials?.bandcamp}
              spotifyUrl={artist.socials?.spotify}
              artistName={artist.name}
              compact
            />
          </div>
        ))}
      </div>
    </div>
  )
}
