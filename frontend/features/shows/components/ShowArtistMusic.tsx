'use client'

import Link from 'next/link'
import { MapPin } from 'lucide-react'
import {
  BANDCAMP_EMBED_MAX_WIDTH_PX,
  MusicEmbed,
} from '@/components/shared/MusicEmbed'
import { SocialLinks } from '@/components/shared/SocialLinks'
import { hasRenderableMusic } from '@/lib/musicAvailability'
import { basedInPhrase, billHometown } from '../utils'
import type { ArtistResponse } from '../types'

/**
 * An act's music sources, in the shape both `hasRenderableMusic` and
 * `MusicEmbed` take: the one mapping the predicate and every player on a bill
 * share, so the predicate cannot say yes to sources a player never receives.
 */
function artistMusicSources(artist: ArtistResponse) {
  return {
    bandcampAlbumUrl: artist.bandcamp_embed_url,
    bandcampProfileUrl: artist.socials?.bandcamp,
    spotifyUrl: artist.socials?.spotify,
  }
}

/**
 * Whether an artist's music block will render anything. A stored Bandcamp URL
 * no longer implies that on its own, so this asks the shared predicate rather
 * than testing the column, or the expand control would open onto nothing.
 */
export function artistHasMusic(artist: ArtistResponse): boolean {
  return hasRenderableMusic(artistMusicSources(artist))
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
 * The music a `ShowCard`'s bill opens onto: per act that has music, its
 * name, where it is based, its social links and a player. The `/shows` list
 * rows open {@link ShowArtistPlayerStack} instead.
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
              {...artistMusicSources(artist)}
              artistName={artist.name}
              compact
            />
          </div>
        ))}
      </div>
    </div>
  )
}

/**
 * The music a `/shows` list row opens onto: one slim OPEN player per act that
 * has music, in bill order, stacked 6px apart and no wider than a Bandcamp
 * player gets.
 *
 * Players only. The row above already names every act, and each player names
 * its own act again, so the stack prints no act name, no hometown and no
 * social links; those live on the show page's listen cards and the artist
 * page. Players render open for the same reason {@link ShowArtistMusicPanel}'s
 * do.
 *
 * Outer spacing and indent are the caller's, through `className`.
 */
export function ShowArtistPlayerStack({
  artists,
  className,
}: {
  artists: ArtistResponse[]
  className?: string
}) {
  const withMusic = artists.filter(artistHasMusic)
  if (withMusic.length === 0) return null

  return (
    <div className={className} data-testid="artist-player-stack">
      {/* The cap sits on its own box so the caller's padding does not eat
          into the players' 700px. */}
      <div
        className="space-y-1.5"
        style={{ maxWidth: BANDCAMP_EMBED_MAX_WIDTH_PX }}
      >
        {withMusic.map(artist => (
          <MusicEmbed
            key={artist.id}
            {...artistMusicSources(artist)}
            artistName={artist.name}
            size="slim"
          />
        ))}
      </div>
    </div>
  )
}
