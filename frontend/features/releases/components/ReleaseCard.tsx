'use client'

import Link from 'next/link'
import { Disc3 } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { getReleaseTypeLabel } from '../types'
import type { ReleaseListItem } from '../types'

export type ReleaseCardDensity = 'compact' | 'comfortable' | 'expanded'

interface ReleaseCardProps {
  release: ReleaseListItem
  density?: ReleaseCardDensity
}

/**
 * Format artist names for display.
 * - 0 artists: returns null
 * - 1-3 artists: comma-separated names
 * - 4+ artists: "Various Artists"
 */
function formatArtistNames(release: ReleaseListItem): string | null {
  const artists = release.artists
  if (!artists || artists.length === 0) return null
  if (artists.length > 3) return 'Various Artists'
  return artists.map((a) => a.name).join(', ')
}

/**
 * Render artist names as linked spans (for comfortable/expanded modes)
 */
function ArtistLinks({ release }: { release: ReleaseListItem }) {
  const artists = release.artists
  if (!artists || artists.length === 0) return null

  if (artists.length > 3) {
    return <span className="text-muted-foreground">Various Artists</span>
  }

  return (
    <>
      {artists.map((artist, i) => (
        <span key={artist.id}>
          <Link
            href={`/artists/${artist.slug}`}
            className="text-muted-foreground hover:text-primary transition-colors"
            onClick={(e) => e.stopPropagation()}
          >
            {artist.name}
          </Link>
          {i < artists.length - 1 && (
            <span className="text-muted-foreground">, </span>
          )}
        </span>
      ))}
    </>
  )
}

export function ReleaseCard({
  release,
  density = 'comfortable',
}: ReleaseCardProps) {
  const releaseUrl = `/releases/${release.slug}`
  const typeLabel = getReleaseTypeLabel(release.release_type)
  const artistDisplay = formatArtistNames(release)

  if (density === 'compact') {
    return (
      <article className="flex items-center gap-3 px-3 py-1.5 hover:bg-muted/50 rounded-md transition-colors">
        {release.cover_art_url ? (
          <img
            src={release.cover_art_url}
            alt={`${release.title} cover art`}
            className="h-6 w-6 rounded object-cover shrink-0"
          />
        ) : (
          <Disc3 className="h-4 w-4 text-muted-foreground shrink-0" />
        )}
        <Link
          href={releaseUrl}
          className="font-medium text-sm truncate flex-1 hover:text-primary transition-colors"
        >
          {artistDisplay ? `${artistDisplay} — ${release.title}` : release.title}
        </Link>
        <Badge variant="secondary" className="text-[10px] shrink-0">
          {typeLabel}
        </Badge>
        {release.release_year && (
          <span className="text-xs text-muted-foreground shrink-0 tabular-nums">
            {release.release_year}
          </span>
        )}
      </article>
    )
  }

  if (density === 'expanded') {
    return (
      <article className="rounded-lg border border-border/50 bg-card p-6 transition-shadow hover:shadow-sm">
        <div className="flex gap-5">
          {/* Larger cover art */}
          <div className="shrink-0 rounded-md bg-muted/50 flex items-center justify-center overflow-hidden h-24 w-24">
            {release.cover_art_url ? (
              <img
                src={release.cover_art_url}
                alt={`${release.title} cover art`}
                className="h-full w-full object-cover"
              />
            ) : (
              <Disc3 className="h-12 w-12 text-muted-foreground/40" />
            )}
          </div>

          {/* Text Content */}
          <div className="flex-1 min-w-0">
            <Link href={releaseUrl} className="block group">
              <h3 className="font-bold text-xl text-foreground group-hover:text-primary transition-colors truncate">
                {release.title}
              </h3>
            </Link>

            {artistDisplay && (
              <div className="mt-1 text-sm truncate">
                <ArtistLinks release={release} />
              </div>
            )}

            <div className="flex items-center gap-3 mt-2">
              <Badge variant="secondary" className="text-xs px-2 py-0.5">
                {typeLabel}
              </Badge>
              {release.release_year && (
                <span className="text-base font-medium text-muted-foreground tabular-nums">
                  {release.release_year}
                </span>
              )}
            </div>

            {release.label_name && (
              <div className="mt-2 text-sm text-muted-foreground">
                {release.label_slug ? (
                  <Link
                    href={`/labels/${release.label_slug}`}
                    className="hover:text-primary transition-colors"
                  >
                    {release.label_name}
                  </Link>
                ) : (
                  release.label_name
                )}
              </div>
            )}
          </div>
        </div>
      </article>
    )
  }

  // Comfortable (default) — current card layout with fixed p-4 padding
  return (
    <article className="rounded-lg border border-border/50 bg-card p-4 transition-shadow hover:shadow-sm">
      <div className="flex gap-3">
        {/* Cover Art / Placeholder */}
        <div className="shrink-0 rounded-md bg-muted/50 flex items-center justify-center overflow-hidden h-16 w-16">
          {release.cover_art_url ? (
            <img
              src={release.cover_art_url}
              alt={`${release.title} cover art`}
              className="h-full w-full object-cover"
            />
          ) : (
            <Disc3 className="h-8 w-8 text-muted-foreground/40" />
          )}
        </div>

        {/* Text Content */}
        <div className="flex-1 min-w-0">
          <Link href={releaseUrl} className="block group">
            <h3 className="font-bold text-base text-foreground group-hover:text-primary transition-colors truncate">
              {release.title}
            </h3>
          </Link>

          {artistDisplay && (
            <div className="text-sm truncate mt-0.5">
              <ArtistLinks release={release} />
            </div>
          )}

          <div className="flex items-center gap-2 flex-wrap mt-1">
            <Badge variant="secondary" className="text-[10px] px-1.5 py-0">
              {typeLabel}
            </Badge>
            {release.release_year && (
              <span className="text-sm text-muted-foreground">
                {release.release_year}
              </span>
            )}
            {release.label_name && (
              <>
                <span className="text-muted-foreground/50">·</span>
                <span className="text-sm text-muted-foreground truncate">
                  {release.label_slug ? (
                    <Link
                      href={`/labels/${release.label_slug}`}
                      className="hover:text-primary transition-colors"
                      onClick={(e) => e.stopPropagation()}
                    >
                      {release.label_name}
                    </Link>
                  ) : (
                    release.label_name
                  )}
                </span>
              </>
            )}
          </div>
        </div>
      </div>
    </article>
  )
}
